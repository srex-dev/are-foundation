package kafka

import (
	"context"
	"testing"
)

func TestNewProducerAndClose(t *testing.T) {
	p := NewProducer([]string{"127.0.0.1:9092"}, "are.agents.lifecycle")
	if p == nil {
		t.Fatal("expected producer")
	}
	if err := p.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
}

func TestWriteReturnsErrorWhenBrokerUnavailable(t *testing.T) {
	p := NewProducer([]string{"127.0.0.1:65530"}, "are.agents.lifecycle")
	defer p.Close()
	err := p.Write(context.Background(), "agent-1", []byte(`{"hello":"world"}`))
	if err == nil {
		t.Fatal("expected write error without reachable broker")
	}
}
