package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/srex-dev/are-foundation/s0/passport-issuance-engine/internal/config"
	grpcapi "github.com/srex-dev/are-foundation/s0/passport-issuance-engine/internal/grpc"
	"github.com/srex-dev/are-foundation/s0/passport-issuance-engine/internal/local"
	"github.com/srex-dev/are-foundation/s0/passport-issuance-engine/internal/metrics"
	passportv1 "github.com/srex-dev/are-foundation/s0/passport-issuance-engine/proto"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("passport issuance: %v", err)
	}
}

func run() error {
	_ = metrics.IssuedTotal // register default metrics
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	svc := local.NewInMemoryService()
	grpcSrv := grpc.NewServer()
	passportv1.RegisterPassportIssuanceServiceServer(grpcSrv, grpcapi.New(svc))

	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcSrv, hs)

	grpcLis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		return fmt.Errorf("grpc listen: %w", err)
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsSrv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.MetricsPort), Handler: metricsMux}

	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
	healthSrv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.HealthPort), Handler: healthMux}

	g, _ := errgroup.WithContext(ctx)
	g.Go(func() error {
		log.Printf("passport gRPC listening on :%d", cfg.GRPCPort)
		if serveErr := grpcSrv.Serve(grpcLis); serveErr != nil && !errors.Is(serveErr, grpc.ErrServerStopped) {
			return fmt.Errorf("grpc serve: %w", serveErr)
		}
		return nil
	})
	g.Go(func() error {
		log.Printf("passport metrics listening on :%d", cfg.MetricsPort)
		if serveErr := metricsSrv.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("metrics: %w", serveErr)
		}
		return nil
	})
	g.Go(func() error {
		log.Printf("passport health listening on :%d", cfg.HealthPort)
		if serveErr := healthSrv.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("health: %w", serveErr)
		}
		return nil
	})

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		grpcSrv.GracefulStop()
		_ = metricsSrv.Shutdown(shutdownCtx)
		_ = healthSrv.Shutdown(shutdownCtx)
	}()

	return g.Wait()
}
