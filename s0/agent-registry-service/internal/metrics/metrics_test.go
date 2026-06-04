package metrics

import "testing"

func TestNewRegistersMetrics(t *testing.T) {
	m := New()
	if m == nil || m.Registrations == nil || m.Lookups == nil || m.KafkaErrors == nil {
		t.Fatal("expected initialized metrics")
	}
}
