package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// IssuedTotal counts IssuePassport gRPC completions by public-safe result label.
	IssuedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "are_passport_issued_total",
		Help: "Total passport issuance attempts by result.",
	}, []string{"result"})
	// RevokedTotal counts RevokePassport gRPC completions by public-safe result label.
	RevokedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "are_passport_revoked_total",
		Help: "Total passport revocation attempts by result.",
	}, []string{"result"})
	// VerifyTotal counts VerifyPassport outcomes by public-safe result and reason labels.
	VerifyTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "are_passport_verify_total",
		Help: "Total passport verification attempts by result and reason.",
	}, []string{"result", "reason"})
	// RequestDurationSeconds captures passport operation latency without recording payloads or identifiers.
	RequestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "are_passport_request_duration_seconds",
		Help:    "Passport operation latency by operation and result.",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation", "result"})
	// LedgerWriteFailureTotal counts best-effort ledger write failures from the domain service (out-of-band).
	LedgerWriteFailureTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "are_passport_ledger_write_failure_total",
		Help: "Total passport ledger write failures (non-blocking path)",
	})
)

// ObserveDuration records an operation duration with bounded public labels.
func ObserveDuration(operation string, start time.Time, result string) {
	RequestDurationSeconds.WithLabelValues(operation, result).Observe(time.Since(start).Seconds())
}
