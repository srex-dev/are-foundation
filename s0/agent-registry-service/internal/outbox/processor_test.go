package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v3"
)

type emitterStub struct {
	err error
}

func (e emitterStub) Write(_ context.Context, _ string, _ []byte) error {
	return e.err
}

func TestBackoffCapsAtThirtySeconds(t *testing.T) {
	if Backoff(1) != 100*time.Millisecond {
		t.Fatalf("unexpected backoff for attempt 1: %v", Backoff(1))
	}
	if Backoff(20) != 30*time.Second {
		t.Fatalf("expected cap at 30s, got %v", Backoff(20))
	}
}

func TestProcessBatchMarksDeliveredOnSuccess(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock: %v", err)
	}
	defer mock.Close()

	rows := pgxmock.NewRows([]string{"outbox_id", "agent_id", "payload", "attempt_count"}).
		AddRow("o1", "a1", []byte(`{"hello":"world"}`), 0)
	mock.ExpectQuery("SELECT outbox_id, agent_id::text, payload, attempt_count").
		WillReturnRows(rows)
	mock.ExpectExec("UPDATE agent_lifecycle_outbox").
		WithArgs("o1").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	p := New(mock, emitterStub{}, 5*time.Millisecond, 10)
	if err := p.processBatch(context.Background()); err != nil {
		t.Fatalf("unexpected process error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessBatchMarksFailedAtMaxAttempts(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock: %v", err)
	}
	defer mock.Close()

	rows := pgxmock.NewRows([]string{"outbox_id", "agent_id", "payload", "attempt_count"}).
		AddRow("o1", "a1", []byte(`{}`), 9)
	mock.ExpectQuery("SELECT outbox_id, agent_id::text, payload, attempt_count").
		WillReturnRows(rows)
	mock.ExpectExec("UPDATE agent_lifecycle_outbox").
		WithArgs("o1", "FAILED", 10).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	p := New(mock, emitterStub{err: errors.New("kafka unavailable")}, 5*time.Millisecond, 10)
	if err := p.processBatch(context.Background()); err != nil {
		t.Fatalf("unexpected process error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock: %v", err)
	}
	defer mock.Close()

	p := New(mock, emitterStub{}, 50*time.Millisecond, 10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Run(ctx); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestProcessBatchHandlesQueryError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock: %v", err)
	}
	defer mock.Close()
	mock.ExpectQuery("SELECT outbox_id, agent_id::text, payload, attempt_count").
		WillReturnError(errors.New("db unavailable"))

	p := New(mock, emitterStub{}, 5*time.Millisecond, 10)
	if err := p.processBatch(context.Background()); err == nil {
		t.Fatal("expected query error")
	}
}
