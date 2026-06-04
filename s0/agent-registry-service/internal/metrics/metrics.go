package metrics

import "github.com/prometheus/client_golang/prometheus"

// RegistryMetrics contains component metric vectors.
type RegistryMetrics struct {
	Registrations *prometheus.CounterVec
	Lookups       *prometheus.CounterVec
	KafkaErrors   prometheus.Counter
}

// New registers and returns all metrics required by spec.
func New() *RegistryMetrics {
	m := &RegistryMetrics{
		Registrations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "are_agent_registry_registrations_total",
				Help: "Total registration attempts by result and reason.",
			},
			[]string{"result", "failure_reason"},
		),
		Lookups: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "are_agent_registry_lookups_total",
				Help: "Total lookups by method and cache behavior.",
			},
			[]string{"method", "cache"},
		),
		KafkaErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "are_agent_registry_kafka_emit_errors_total",
			Help: "Total failed outbox Kafka publish attempts.",
		}),
	}
	prometheus.MustRegister(m.Registrations, m.Lookups, m.KafkaErrors)
	return m
}
