package gateway

import (
	"context"
	"testing"
	"time"
)

func TestOutboxProcessorStopsOnContextCancel(t *testing.T) {
	proc := NewOutboxProcessor(10 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		proc.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("outbox processor did not stop")
	}
}
