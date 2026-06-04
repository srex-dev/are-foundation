package outbox

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Emitter emits a single outbox payload to Kafka.
type Emitter interface {
	Write(ctx context.Context, key string, value []byte) error
}

// Processor polls outbox and forwards pending messages.
type Processor struct {
	pool        poolQueries
	emitter     Emitter
	pollEvery   time.Duration
	maxAttempts int
}

type poolQueries interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// New creates an outbox processor.
func New(pool poolQueries, emitter Emitter, pollEvery time.Duration, maxAttempts int) *Processor {
	return &Processor{
		pool:        pool,
		emitter:     emitter,
		pollEvery:   pollEvery,
		maxAttempts: maxAttempts,
	}
}

// Run starts the polling loop until context cancellation.
func (p *Processor) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_ = p.processBatch(ctx)
		}
	}
}

func (p *Processor) processBatch(ctx context.Context) error {
	rows, err := p.pool.Query(ctx, `
		SELECT outbox_id, agent_id::text, payload, attempt_count
		FROM agent_lifecycle_outbox
		WHERE status='PENDING'
		ORDER BY created_ts
		LIMIT 100
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var outboxID string
		var agentID string
		var payload []byte
		var attempts int
		if err := rows.Scan(&outboxID, &agentID, &payload, &attempts); err != nil {
			return err
		}

		err := p.emitter.Write(ctx, agentID, payload)
		if err == nil {
			_, _ = p.pool.Exec(ctx, `
				UPDATE agent_lifecycle_outbox
				SET status='DELIVERED', last_attempt_ts=NOW(), attempt_count=attempt_count+1
				WHERE outbox_id=$1
			`, outboxID)
			continue
		}

		log.Printf(
			"agent-registry FM-001: outbox kafka emit failed outbox_id=%s agent_id=%s attempt=%d err=%v",
			outboxID,
			agentID,
			attempts+1,
			err,
		)

		newAttempts := attempts + 1
		newStatus := "PENDING"
		if newAttempts >= p.maxAttempts {
			newStatus = "FAILED"
		}
		_, _ = p.pool.Exec(ctx, `
			UPDATE agent_lifecycle_outbox
			SET status=$2, last_attempt_ts=NOW(), attempt_count=$3
			WHERE outbox_id=$1
		`, outboxID, newStatus, newAttempts)
	}
	return rows.Err()
}

// Backoff computes exponential retry wait for attempt count.
func Backoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	d := 100 * time.Millisecond
	for i := 1; i < attempt; i++ {
		d *= 2
		if d > 30*time.Second {
			return 30 * time.Second
		}
	}
	return d
}

// IsNotFound checks expected not-found reads.
func IsNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
