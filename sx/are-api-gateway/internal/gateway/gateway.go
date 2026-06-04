package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// UpstreamClient forwards requests to internal services.
type UpstreamClient interface {
	Call(ctx context.Context, req UpstreamRequest) (UpstreamResponse, error)
}

// UpstreamRequest carries pass-through request state.
type UpstreamRequest struct {
	Method         string
	Path           string
	RawQuery       string
	Body           []byte
	AgentID        string
	RequestID      string
	Identity       string
	IdempotencyKey string
	Authorization  string // full Authorization header value, e.g. "Bearer …", for upstream forwarding
	ContentType    string
	Traceparent    string
}

// UpstreamResponse holds routed response.
type UpstreamResponse struct {
	StatusCode int
	Body       []byte
}

// Gateway handles external API traffic.
type Gateway struct {
	auth     TokenValidator
	upstream UpstreamClient
	logger   io.Writer
	timeout  time.Duration
	now      func() time.Time
}

const defaultAuthTimeout = 500 * time.Millisecond

func metricsScrapeRequiresAuth() bool {
	// Anonymous /metrics allowed only in dev-unsafe stacks, explicit opt-out, or legacy REQUIRE_AUTH=0.
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ARE_GATEWAY_METRICS_ALLOW_ANONYMOUS")), "true") ||
		strings.TrimSpace(os.Getenv("ARE_GATEWAY_METRICS_ALLOW_ANONYMOUS")) == "1" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ARE_GW_DEV_UNSAFE_MODE")), "true") {
		return false
	}
	v := strings.TrimSpace(strings.ToLower(os.Getenv("ARE_GATEWAY_METRICS_REQUIRE_AUTH")))
	if v == "0" || v == "false" || v == "no" {
		return false
	}
	return true
}

func (g *Gateway) authorizeMetricsScrape(r *http.Request) error {
	if !metricsScrapeRequiresAuth() {
		return nil
	}
	authz := r.Header.Get("Authorization")
	if authz == "" || !strings.HasPrefix(authz, "Bearer ") {
		return errors.New("missing bearer token")
	}
	rawToken := strings.TrimPrefix(authz, "Bearer ")
	ctx, cancel := context.WithTimeout(r.Context(), g.timeout)
	defer cancel()
	_, err := g.auth.Validate(ctx, rawToken)
	if err != nil || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("unauthorized")
	}
	return nil
}

// MetricsPrometheusHandler serves the same registry as GET /metrics on the API listener (decoupled scrape port ARE_GW_METRICS_PORT).
func (g *Gateway) MetricsPrometheusHandler() http.Handler {
	inner := promhttp.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := g.now()
		status := http.StatusOK
		defer func() {
			observeGatewayHTTP(r.Method, "/metrics", status, time.Since(start))
		}()
		if err := g.authorizeMetricsScrape(r); err != nil {
			status = http.StatusUnauthorized
			http.Error(w, err.Error(), status)
			return
		}
		inner.ServeHTTP(w, r)
	})
}

// stripUntrustedStageSpoofHeaders removes client headers that must never affect authorization or routing.
func stripUntrustedStageSpoofHeaders(r *http.Request) {
	for _, h := range []string{
		"X-Are-Ui-Stage",
		"X-Are-Session-Stage",
		"X-Are-Client-Stage",
		"X-Are-Deployment-Stage",
	} {
		r.Header.Del(h)
	}
}

// NewGateway constructs an API gateway handler.
func NewGateway(auth TokenValidator, upstream UpstreamClient, logger io.Writer) *Gateway {
	return &Gateway{
		auth:     auth,
		upstream: upstream,
		logger:   logger,
		timeout:  defaultAuthTimeout,
		now:      time.Now,
	}
}

// SetAuthTimeout sets auth validation deadline.
func (g *Gateway) SetAuthTimeout(timeout time.Duration) {
	if timeout > 0 {
		g.timeout = timeout
	}
}

// Handler returns an HTTP handler implementing the gateway behavior.
func (g *Gateway) Handler() http.Handler {
	return http.HandlerFunc(g.handle)
}

func (g *Gateway) handle(w http.ResponseWriter, r *http.Request) {
	start := g.now()
	trace := startServerTrace(r, "are-api-gateway", "are-api-gateway/http-server")
	requestID := r.Header.Get("X-Request-ID")
	authz := r.Header.Get("Authorization")
	agentID := r.Header.Get("X-ARE-Agent-ID")
	idempotencyKey := r.Header.Get("Idempotency-Key")
	status := http.StatusOK
	identity := ""
	defer func() {
		route := normalizedRoute(r.URL.Path)
		observeGatewayHTTP(r.Method, r.URL.Path, status, time.Since(start))
		trace.finish(r.Method, route, r.URL.Path, status, start, g.now(), "are-api-gateway")
		g.logRequest(logEntry{
			RequestID: requestID,
			Identity:  identity,
			Method:    r.Method,
			Path:      r.URL.Path,
			Status:    status,
			LatencyMS: int(time.Since(start).Milliseconds()),
		})
	}()

	if r.URL.Path == "/v1/platform/health" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
		return
	}
	if r.URL.Path == "/metrics" {
		if err := g.authorizeMetricsScrape(r); err != nil {
			status = http.StatusUnauthorized
			http.Error(w, err.Error(), status)
			return
		}
		promhttp.Handler().ServeHTTP(w, r)
		return
	}

	if requestID == "" {
		status = http.StatusBadRequest
		http.Error(w, "missing request id", status)
		return
	}
	if requiresIdempotencyKey(r.Method, r.URL.Path) && idempotencyKey == "" {
		status = http.StatusBadRequest
		http.Error(w, "missing idempotency key", status)
		return
	}
	if authz == "" || !strings.HasPrefix(authz, "Bearer ") {
		status = http.StatusUnauthorized
		http.Error(w, "missing bearer token", status)
		return
	}
	if agentID == "" {
		status = http.StatusBadRequest
		http.Error(w, "missing agent id", status)
		return
	}
	rawToken := strings.TrimPrefix(authz, "Bearer ")
	ctx, cancel := context.WithTimeout(r.Context(), g.timeout)
	defer cancel()
	id, err := g.auth.Validate(ctx, rawToken)
	if err != nil || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		status = http.StatusUnauthorized
		http.Error(w, "unauthorized", status)
		return
	}
	identity = id
	stripUntrustedStageSpoofHeaders(r)
	if !isFoundationAPIRoute(r.Method, r.URL.Path) {
		status = http.StatusNotFound
		http.Error(w, "route is not part of the ARE Foundation S0/S1 surface", status)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		status = http.StatusBadRequest
		http.Error(w, "bad body", status)
		return
	}
	resp, err := g.upstream.Call(r.Context(), UpstreamRequest{
		Method:         r.Method,
		Path:           r.URL.Path,
		RawQuery:       r.URL.RawQuery,
		Body:           body,
		AgentID:        agentID,
		RequestID:      requestID,
		Identity:       identity,
		IdempotencyKey: idempotencyKey,
		Authorization:  authz,
		ContentType:    r.Header.Get("Content-Type"),
		Traceparent:    trace.traceparent(),
	})
	if err != nil {
		status = http.StatusServiceUnavailable
		g.logUpstreamError(requestID, r.Method, r.URL.Path, err)
		http.Error(w, "upstream unavailable", status)
		return
	}
	status = resp.StatusCode
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)
}

type logEntry struct {
	RequestID string `json:"request_id"`
	Identity  string `json:"caller_identity"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"response_code"`
	LatencyMS int    `json:"latency_ms"`
}

func (g *Gateway) logRequest(entry logEntry) {
	if g.logger == nil {
		return
	}
	bytes, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = g.logger.Write(append(bytes, '\n'))
}

func (g *Gateway) logUpstreamError(requestID, method, path string, err error) {
	if g.logger == nil {
		return
	}
	entry := map[string]string{
		"event":      "upstream_error",
		"request_id": requestID,
		"method":     method,
		"path":       path,
		"error":      err.Error(),
	}
	bytes, marshalErr := json.Marshal(entry)
	if marshalErr != nil {
		return
	}
	_, _ = g.logger.Write(append(bytes, '\n'))
}
