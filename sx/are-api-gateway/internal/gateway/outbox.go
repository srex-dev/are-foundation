package gateway

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
)

// OutboxProcessor is a background goroutine for gateway lifecycle consistency.
type OutboxProcessor struct {
	pollInterval time.Duration
	writer       *kafka.Writer
}

// NewOutboxProcessor creates an outbox processor.
func NewOutboxProcessor(pollInterval time.Duration) *OutboxProcessor {
	return &OutboxProcessor{
		pollInterval: pollInterval,
		writer:       &kafka.Writer{},
	}
}

// Run runs until context cancellation.
func (o *OutboxProcessor) Run(ctx context.Context) {
	ticker := time.NewTicker(o.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// No owned entities for this component; keep loop as required processor.
			_ = o.writer.Stats()
		}
	}
}
