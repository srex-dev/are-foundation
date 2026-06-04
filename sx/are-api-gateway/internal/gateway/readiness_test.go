package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadinessJWKSOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{}})
	}))
	t.Cleanup(srv.Close)

	failures := ReadinessFailures(context.Background(), ReadinessConfig{
		JWKSURL:      srv.URL,
		ProbeTimeout: 2 * time.Second,
	})
	if len(failures) != 0 {
		t.Fatalf("expected no failures, got %v", failures)
	}
}

func TestReadinessJWKSBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	failures := ReadinessFailures(context.Background(), ReadinessConfig{
		JWKSURL:      srv.URL,
		ProbeTimeout: 2 * time.Second,
	})
	if len(failures) != 1 || !strings.Contains(failures[0], "jwks:") {
		t.Fatalf("expected jwks failure, got %v", failures)
	}
}

func TestReadinessKafkaUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(srv.Close)

	failures := ReadinessFailures(context.Background(), ReadinessConfig{
		JWKSURL:      srv.URL,
		KafkaBrokers: []string{"127.0.0.1:1"},
		ProbeTimeout: 2 * time.Second,
	})
	if len(failures) != 1 || !strings.Contains(failures[0], "kafka:") {
		t.Fatalf("expected kafka failure, got %v", failures)
	}
}

func TestParseKafkaBootstrap(t *testing.T) {
	got := ParseKafkaBootstrap(" a:9092 , b:9093 ")
	if len(got) != 2 || got[0] != "a:9092" || got[1] != "b:9093" {
		t.Fatalf("unexpected %q", got)
	}
	if ParseKafkaBootstrap("") != nil {
		t.Fatal("expected nil for empty")
	}
}

func TestServeReadiness503(t *testing.T) {
	h := ServeReadiness(ReadinessConfig{
		JWKSURL:      "http://127.0.0.1:1/no",
		ProbeTimeout: 500 * time.Millisecond,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	h(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}
