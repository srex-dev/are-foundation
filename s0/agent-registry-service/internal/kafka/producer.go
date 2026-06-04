package kafka

import (
	"context"
	"time"

	kgo "github.com/segmentio/kafka-go"
)

// Producer wraps kafka-go writer.
type Producer struct {
	writer *kgo.Writer
}

// NewProducer builds a lifecycle event producer.
func NewProducer(brokers []string, topic string) *Producer {
	return &Producer{
		writer: &kgo.Writer{
			Addr:         kgo.TCP(brokers...),
			Topic:        topic,
			RequiredAcks: kgo.RequireAll,
			BatchTimeout: 10 * time.Millisecond,
		},
	}
}

// Write sends one event keyed by agent id.
func (p *Producer) Write(ctx context.Context, key string, value []byte) error {
	return p.writer.WriteMessages(ctx, kgo.Message{
		Key:   []byte(key),
		Value: value,
		Time:  time.Now().UTC(),
	})
}

// Close closes the writer.
func (p *Producer) Close() error {
	return p.writer.Close()
}
