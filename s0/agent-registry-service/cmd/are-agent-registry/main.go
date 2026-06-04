package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/config"
	grpcserver "github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/grpc"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/health"
	ikafka "github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/kafka"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/metrics"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/outbox"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/repository/postgres"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/service"
	registryv1 "github.com/srex-dev/are-foundation/s0/agent-registry-service/proto"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	healthsrv "google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := connectWithRetry(ctx, cfg.DBConnectionString, 3, 2*time.Second)
	if err != nil {
		log.Fatalf("postgres unavailable after retries: %v", err)
	}
	defer pool.Close()

	if err := runMigrations(ctx, pool); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	_ = metrics.New()

	brokers := strings.Split(cfg.KafkaBootstrapServers, ",")
	producer := ikafka.NewProducer(brokers, cfg.KafkaTopicLifecycle)
	defer producer.Close()

	outboxProcessor := outbox.New(pool, producer, time.Duration(cfg.OutboxPollIntervalMS)*time.Millisecond, cfg.OutboxMaxAttempts)

	grpcServer := grpc.NewServer()
	healthServer := healthsrv.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	repo := postgres.New(pool)
	registryService := service.New(repo)
	registryv1.RegisterAgentRegistryServiceServer(grpcServer, grpcserver.New(registryService))

	grpcLis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		log.Fatalf("listen grpc: %v", err)
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsSrv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.MetricsPort), Handler: metricsMux}

	var ready atomic.Bool
	ready.Store(true)
	healthMux := http.NewServeMux()
	health.Handler{Ready: func() bool { return ready.Load() }}.Register(healthMux)
	healthSrv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.HealthPort), Handler: healthMux}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		err := outboxProcessor.Run(gctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("outbox processor: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		if serveErr := grpcServer.Serve(grpcLis); serveErr != nil && !errors.Is(serveErr, grpc.ErrServerStopped) {
			return fmt.Errorf("grpc serve: %w", serveErr)
		}
		return nil
	})
	g.Go(func() error {
		if serveErr := metricsSrv.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("metrics listen: %w", serveErr)
		}
		return nil
	})
	g.Go(func() error {
		if serveErr := healthSrv.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("health listen: %w", serveErr)
		}
		return nil
	})

	go func() {
		<-ctx.Done()
		ready.Store(false)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		grpcServer.GracefulStop()
		_ = metricsSrv.Shutdown(shutdownCtx)
		_ = healthSrv.Shutdown(shutdownCtx)
	}()

	if err := g.Wait(); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		log.Fatalf("fatal server error: %v", err)
	}
}

func connectWithRetry(ctx context.Context, connString string, attempts int, delay time.Duration) (*pgxpool.Pool, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		pool, err := pgxpool.New(ctx, connString)
		if err == nil {
			pingErr := pool.Ping(ctx)
			if pingErr == nil {
				return pool, nil
			}
			pool.Close()
			lastErr = fmt.Errorf("postgres ping failed: %w", pingErr)
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, "SELECT 1")
	return err
}
