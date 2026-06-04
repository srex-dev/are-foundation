package gateway

import (
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Minimal perimeter RED-ish signals (gateway HTTP only). Labels are intentionally low-cardinality.
//
//   - Rate: derivatives of are_gateway_http_requests_total (by method/route/status_family)
//   - Errors: filters on status_family 4xx/5xx vs 2xx
//   - Duration: are_gateway_http_request_duration_seconds (by route)
var (
	redHTTPRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "are_gateway",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "HTTP requests observed at the gateway perimeter (method, normalized route, status family).",
		},
		[]string{"method", "route", "status_family"},
	)
	redHTTPDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "are_gateway",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "Wall time for handling one HTTP request at the gateway (handler work only).",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
		},
		[]string{"route"},
	)
	// Retained for backward compatibility with older scrape scripts that grep this name.
	syntheticProbeGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "are_gateway",
			Name:      "synthetic_probe",
			Help:      "Always 1 when the process is running and /metrics is enabled (legacy compatibility).",
		},
	)
)

func init() {
	syntheticProbeGauge.Set(1)
}

func observeGatewayHTTP(method, path string, status int, dur time.Duration) {
	route := normalizedRoute(path)
	m := strings.ToUpper(strings.TrimSpace(method))
	if m == "" {
		m = "UNKNOWN"
	}
	redHTTPRequests.WithLabelValues(m, route, statusFamily(status)).Inc()
	redHTTPDuration.WithLabelValues(route).Observe(dur.Seconds())
}

func statusFamily(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500:
		return "5xx"
	default:
		return strconv.Itoa(code)
	}
}

// normalizedRoute buckets paths to safe labels (no IDs in path).
func normalizedRoute(path string) string {
	p := strings.TrimSuffix(strings.TrimSpace(path), "/")
	switch p {
	case "":
		return "/"
	case "/metrics":
		return "/metrics"
	case "/v1/platform/health":
		return "/v1/platform/health"
	case "/v1/identity/agents":
		return "/v1/identity/agents"
	case "/v1/passports":
		return "/v1/passports"
	case "/v1/passports:verify":
		return "/v1/passports:verify"
	case "/v1/enforcement/scope:evaluate":
		return "/v1/enforcement/scope:evaluate"
	case "/v1/policy/evaluations":
		return "/v1/policy/evaluations"
	default:
		if strings.HasPrefix(p, "/v1/identity/agents/") {
			return "/v1/identity/agents/{agent_id}"
		}
		if strings.HasPrefix(p, "/v1/passports/by-agent/") {
			return "/v1/passports/by-agent/{agent_id}"
		}
		if strings.HasPrefix(p, "/v1/") {
			sub := strings.TrimPrefix(p, "/v1/")
			if idx := strings.Index(sub, "/"); idx >= 0 {
				sub = sub[:idx]
			}
			if sub != "" {
				return "/v1/" + sub
			}
			return "/v1/other"
		}
		return "other"
	}
}
