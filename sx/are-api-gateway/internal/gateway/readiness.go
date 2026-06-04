package gateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

// ReadinessConfig drives GET /readyz dependency checks.
type ReadinessConfig struct {
	JWKSURL      string
	KafkaBrokers []string // optional; if non-empty, at least one broker must accept a TCP dial
	ProbeTimeout time.Duration
	HTTPClient   *http.Client // if nil, a client with ProbeTimeout is used per request
}

// ParseKafkaBootstrap splits comma-separated broker addresses (e.g. "kafka:9092,kafka-2:9092").
func ParseKafkaBootstrap(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ReadinessFailures returns human-readable probe failures for logging or response bodies.
func ReadinessFailures(ctx context.Context, cfg ReadinessConfig) []string {
	timeout := cfg.ProbeTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	var failures []string
	if err := probeJWKS(ctx, client, cfg.JWKSURL); err != nil {
		failures = append(failures, fmt.Sprintf("jwks: %v", err))
	}
	if len(cfg.KafkaBrokers) > 0 {
		if err := probeKafka(ctx, cfg.KafkaBrokers); err != nil {
			failures = append(failures, fmt.Sprintf("kafka: %v", err))
		}
	}
	return failures
}

func probeJWKS(ctx context.Context, client *http.Client, jwksURL string) error {
	if strings.TrimSpace(jwksURL) == "" {
		return fmt.Errorf("jwks url is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if _, err := io.CopyN(io.Discard, resp.Body, 65536); err != nil && err != io.EOF {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	return nil
}

func probeKafka(ctx context.Context, brokers []string) error {
	var lastErr error
	for _, addr := range brokers {
		if addr == "" {
			continue
		}
		conn, err := kafka.DialContext(ctx, "tcp", addr)
		if err != nil {
			lastErr = err
			continue
		}
		_ = conn.Close()
		return nil
	}
	if lastErr == nil {
		return fmt.Errorf("no kafka brokers configured for probe")
	}
	return lastErr
}

// ServeReadiness returns an HTTP handler for GET /readyz (503 when any probe fails).
func ServeReadiness(cfg ReadinessConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		failures := ReadinessFailures(r.Context(), cfg)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if len(failures) > 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(strings.Join(failures, "\n")))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}
