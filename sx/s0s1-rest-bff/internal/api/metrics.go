package api

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// S0 delegate (iter-006) upstream gRPC errors, labeled by BFF error_code before HTTP mapping.
var s0DelegateUpstreamErrors = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "are_s0s1",
		Subsystem: "bff",
		Name:      "s0_delegate_upstream_errors_total",
		Help:      "gRPC errors from optional S0 registry/passport delegate (fol-002).",
	},
	[]string{"error_code"},
)

// IncS0DelegateError records one upstream S0 gRPC error (non-nil after status mapping).
func IncS0DelegateError(errorCode string) {
	ecode := strings.TrimSpace(errorCode)
	if ecode == "" {
		ecode = "UNKNOWN"
	}
	s0DelegateUpstreamErrors.WithLabelValues(ecode).Inc()
}
