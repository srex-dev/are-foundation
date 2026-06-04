//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	kgo "github.com/segmentio/kafka-go"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/domain"
	ikafka "github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/kafka"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/outbox"
	repopostgres "github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/repository/postgres"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/service"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const lifecycleTopic = "are.agents.lifecycle"

func TestIT001RegistrationFlowWithPostgresKafka(t *testing.T) {
	ctx := context.Background()
	pool, producer, brokers, cleanup := bootDeps(t, ctx)
	defer cleanup()

	repo := repopostgres.New(pool)
	svc := service.New(repo)
	proc := outbox.New(pool, producer, 100*time.Millisecond, 10)
	procCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = proc.Run(procCtx) }()

	agent, err := svc.Register(ctx, "AUTONOMOUS", "00000000-0000-0000-0000-000000000001", "ext-it001", map[string]string{"it": "001"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	got, err := svc.Get(ctx, agent.AgentID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AgentID != agent.AgentID || got.Status != domain.StatusPending {
		t.Fatalf("unexpected get result: %+v", got)
	}

	msg := readOneEvent(t, ctx, brokers, agent.AgentID)
	if msg == nil {
		t.Fatal("expected kafka lifecycle event")
	}
}

func TestIT002StatusHistoryAuditTrail(t *testing.T) {
	ctx := context.Background()
	pool, producer, _, cleanup := bootDeps(t, ctx)
	defer cleanup()

	repo := repopostgres.New(pool)
	svc := service.New(repo)
	proc := outbox.New(pool, producer, 100*time.Millisecond, 10)
	procCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = proc.Run(procCtx) }()

	agent, err := svc.Register(ctx, "ASSISTED", "00000000-0000-0000-0000-000000000002", "ext-it002", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err = svc.UpdateStatus(ctx, agent.AgentID, "ACTIVE", "approved", "integration-test")
	if err != nil {
		t.Fatalf("status update: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_status_history WHERE agent_id=$1`, agent.AgentID).Scan(&count); err != nil {
		t.Fatalf("status history count query: %v", err)
	}
	if count < 2 {
		t.Fatalf("expected >=2 history rows, got %d", count)
	}
}

func TestIT003CacheBehaviorWithRedisDisabled(t *testing.T) {
	ctx := context.Background()
	pool, producer, _, cleanup := bootDeps(t, ctx)
	defer cleanup()

	repo := repopostgres.New(pool)
	svc := service.New(repo)
	proc := outbox.New(pool, producer, 100*time.Millisecond, 10)
	procCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = proc.Run(procCtx) }()

	agent, err := svc.Register(ctx, "AUTONOMOUS", "00000000-0000-0000-0000-000000000020", "ext-it003", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	first, err := svc.Get(ctx, agent.AgentID)
	if err != nil {
		t.Fatalf("first get: %v", err)
	}
	second, err := svc.Get(ctx, agent.AgentID)
	if err != nil {
		t.Fatalf("second get: %v", err)
	}
	if first.AgentID != second.AgentID {
		t.Fatalf("lookup mismatch without cache: first=%s second=%s", first.AgentID, second.AgentID)
	}
}

func TestIT004ConcurrentRegistrationsUniqueIDs(t *testing.T) {
	ctx := context.Background()
	pool, producer, _, cleanup := bootDeps(t, ctx)
	defer cleanup()

	repo := repopostgres.New(pool)
	svc := service.New(repo)
	proc := outbox.New(pool, producer, 100*time.Millisecond, 10)
	procCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = proc.Run(procCtx) }()

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	errCh := make(chan error, n)
	ids := sync.Map{}
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			agent, err := svc.Register(
				ctx,
				"EPHEMERAL",
				"00000000-0000-0000-0000-000000000003",
				fmt.Sprintf("ext-concurrent-%03d", i),
				nil,
			)
			if err != nil {
				errCh <- err
				return
			}
			if _, loaded := ids.LoadOrStore(agent.AgentID, true); loaded {
				errCh <- fmt.Errorf("duplicate agent id generated: %s", agent.AgentID)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent registration error: %v", err)
	}
}

func TestIT005KafkaEventSchema(t *testing.T) {
	ctx := context.Background()
	pool, producer, brokers, cleanup := bootDeps(t, ctx)
	defer cleanup()

	repo := repopostgres.New(pool)
	svc := service.New(repo)
	proc := outbox.New(pool, producer, 100*time.Millisecond, 10)
	procCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = proc.Run(procCtx) }()

	agent, err := svc.Register(ctx, "SYSTEM", "00000000-0000-0000-0000-000000000004", "ext-it005", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	msg := readOneEvent(t, ctx, brokers, agent.AgentID)
	if msg == nil {
		t.Fatal("expected message")
	}

	var payload map[string]any
	if err := json.Unmarshal(msg.Value, &payload); err != nil {
		t.Fatalf("invalid event json: %v", err)
	}
	required := []string{"event_id", "event_type", "agent_id", "new_status", "schema_version"}
	for _, key := range required {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing required key %s in payload: %+v", key, payload)
		}
	}
	if payload["schema_version"] != "1.0.0" {
		t.Fatalf("unexpected schema version: %v", payload["schema_version"])
	}
}

func bootDeps(t *testing.T, ctx context.Context) (*pgxpool.Pool, *ikafka.Producer, []string, func()) {
	t.Helper()

	pg, err := tcpostgres.Run(ctx,
		"postgres:15-alpine",
		tcpostgres.WithDatabase("are"),
		tcpostgres.WithUsername("are"),
		tcpostgres.WithPassword("are"),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	kc, err := kafka.Run(ctx, "confluentinc/confluent-local:7.5.0")
	if err != nil {
		_ = pg.Terminate(ctx)
		t.Fatalf("start kafka container: %v", err)
	}

	pgConn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = kc.Terminate(ctx)
		_ = pg.Terminate(ctx)
		t.Fatalf("postgres connection string: %v", err)
	}
	pool, err := pgxpool.New(ctx, pgConn)
	if err != nil {
		_ = kc.Terminate(ctx)
		_ = pg.Terminate(ctx)
		t.Fatalf("pgx pool init: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		_ = kc.Terminate(ctx)
		_ = pg.Terminate(ctx)
		t.Fatalf("pg ping: %v", err)
	}
	applyMigrations(t, ctx, pool)

	broker, err := kc.Brokers(ctx)
	if err != nil || len(broker) == 0 {
		pool.Close()
		_ = kc.Terminate(ctx)
		_ = pg.Terminate(ctx)
		t.Fatalf("kafka broker address: %v", err)
	}

	// Ensure topic exists before producer/consumer usage.
	conn, err := kgo.Dial("tcp", broker[0])
	if err != nil {
		pool.Close()
		_ = kc.Terminate(ctx)
		_ = pg.Terminate(ctx)
		t.Fatalf("dial kafka: %v", err)
	}
	controller, err := conn.Controller()
	if err != nil {
		_ = conn.Close()
		pool.Close()
		_ = kc.Terminate(ctx)
		_ = pg.Terminate(ctx)
		t.Fatalf("kafka controller: %v", err)
	}
	_ = conn.Close()
	ctrlConn, err := kgo.Dial("tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		pool.Close()
		_ = kc.Terminate(ctx)
		_ = pg.Terminate(ctx)
		t.Fatalf("dial controller: %v", err)
	}
	_ = ctrlConn.CreateTopics(kgo.TopicConfig{
		Topic:             lifecycleTopic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
	_ = ctrlConn.Close()

	producer := ikafka.NewProducer(broker, lifecycleTopic)
	cleanup := func() {
		_ = producer.Close()
		pool.Close()
		_ = kc.Terminate(ctx)
		_ = pg.Terminate(ctx)
	}
	return pool, producer, broker, cleanup
}

func applyMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	paths := []string{
		filepath.FromSlash("../migrations/001_init.sql"),
		filepath.FromSlash("../migrations/002_outbox.sql"),
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read migration %s: %v", p, err)
		}
		if _, err := pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("apply migration %s: %v", p, err)
		}
	}
}

func readOneEvent(t *testing.T, ctx context.Context, brokers []string, key string) *kgo.Message {
	t.Helper()
	reader := kgo.NewReader(kgo.ReaderConfig{
		Brokers:     brokers,
		Topic:       lifecycleTopic,
		GroupID:     "are-it-group",
		StartOffset: kgo.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
	})
	defer reader.Close()

	timeoutCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		msg, err := reader.ReadMessage(timeoutCtx)
		if err != nil {
			return nil
		}
		if string(msg.Key) == key {
			return &msg
		}
	}
}
