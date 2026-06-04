package gateway

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type serverTrace struct {
	endpoint     string
	traceID      string
	spanID       string
	parentSpanID string
	sampled      bool
	serviceName  string
	scopeName    string
}

func startServerTrace(r *http.Request, serviceName, scopeName string) *serverTrace {
	if r == nil || r.URL == nil || r.URL.Path == "/metrics" {
		return nil
	}
	incoming := parseTraceparent(r.Header.Get("traceparent"))
	endpoint := otlpEndpoint()
	traceID := ""
	parentSpanID := ""
	sampled := false
	if incoming != nil {
		traceID = incoming.traceID
		parentSpanID = incoming.parentSpanID
		sampled = incoming.sampled
	} else if endpoint != "" {
		traceID = randomHex(16)
		sampled = shouldSampleTrace(traceID)
	}
	if traceID == "" {
		return nil
	}
	return &serverTrace{
		endpoint:     endpoint,
		traceID:      traceID,
		spanID:       randomHex(8),
		parentSpanID: parentSpanID,
		sampled:      sampled,
		serviceName:  envDefault("OTEL_SERVICE_NAME", serviceName),
		scopeName:    scopeName,
	}
}

func (t *serverTrace) traceparent() string {
	if t == nil {
		return ""
	}
	flags := "00"
	spanID := t.parentSpanID
	if t.sampled {
		flags = "01"
		spanID = t.spanID
	}
	if spanID == "" {
		spanID = t.spanID
	}
	if spanID == "" {
		return ""
	}
	return "00-" + t.traceID + "-" + spanID + "-" + flags
}

func (t *serverTrace) shouldRecord() bool {
	return t != nil && t.sampled && t.endpoint != ""
}

func (t *serverTrace) recordedTraceparent() string {
	if !t.shouldRecord() {
		return ""
	}
	return "00-" + t.traceID + "-" + t.spanID + "-01"
}

func (t *serverTrace) finish(method, route, path string, status int, started time.Time, ended time.Time, component string) {
	if !t.shouldRecord() {
		return
	}
	if route == "" {
		route = normalizedRoute(path)
	}
	if path == "" {
		path = route
	}
	statusCode := 1
	statusMessage := ""
	if status >= 500 {
		statusCode = 2
		statusMessage = "HTTP " + strconv.Itoa(status)
	}
	span := map[string]any{
		"traceId":           t.traceID,
		"spanId":            t.spanID,
		"name":              strings.ToUpper(method) + " " + route,
		"kind":              2,
		"startTimeUnixNano": strconv.FormatInt(started.UnixNano(), 10),
		"endTimeUnixNano":   strconv.FormatInt(ended.UnixNano(), 10),
		"attributes": []map[string]any{
			otlpAttr("http.request.method", strings.ToUpper(method)),
			otlpAttr("url.path", path),
			otlpAttr("http.route", route),
			otlpAttr("http.response.status_code", status),
			otlpAttr("are.component", component),
			otlpAttr("error", status >= 500),
		},
		"status": map[string]any{"code": statusCode},
	}
	if t.parentSpanID != "" {
		span["parentSpanId"] = t.parentSpanID
	}
	if statusMessage != "" {
		span["status"] = map[string]any{"code": statusCode, "message": statusMessage}
	}
	payload := map[string]any{
		"resourceSpans": []map[string]any{
			{
				"resource": map[string]any{
					"attributes": []map[string]any{
						otlpAttr("service.name", t.serviceName),
						otlpAttr("telemetry.sdk.name", "manual-otlp-json"),
						otlpAttr("telemetry.sdk.language", "go"),
					},
				},
				"scopeSpans": []map[string]any{
					{
						"scope": map[string]any{"name": t.scopeName},
						"spans": []map[string]any{span},
					},
				},
			},
		},
	}
	go postOTLP(t.endpoint, payload)
}

type traceparentParts struct {
	traceID      string
	parentSpanID string
	sampled      bool
}

func parseTraceparent(value string) *traceparentParts {
	parts := strings.Split(strings.TrimSpace(value), "-")
	if len(parts) != 4 || parts[0] != "00" || len(parts[1]) != 32 || len(parts[2]) != 16 {
		return nil
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return nil
	}
	if _, err := hex.DecodeString(parts[2]); err != nil {
		return nil
	}
	flags, err := strconv.ParseUint(parts[3], 16, 8)
	if err != nil {
		return nil
	}
	return &traceparentParts{traceID: parts[1], parentSpanID: parts[2], sampled: flags&1 == 1}
}

func otlpEndpoint() string {
	raw := strings.TrimSpace(os.Getenv("ARE_OTLP_HTTP_URL"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("STRATA_OTLP_HTTP_URL"))
	}
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"))
	}
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	}
	if raw == "" {
		return ""
	}
	if strings.HasSuffix(raw, "/v1/traces") {
		return raw
	}
	return strings.TrimRight(raw, "/") + "/v1/traces"
}

func shouldSampleTrace(traceID string) bool {
	raw := strings.TrimSpace(os.Getenv("ARE_OTLP_TRACE_SAMPLE_RATE"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("STRATA_OTLP_TRACE_SAMPLE_RATE"))
	}
	rate := 1.0
	if raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			rate = v
		}
	}
	if rate >= 1 {
		return true
	}
	if rate <= 0 {
		return false
	}
	n, err := strconv.ParseUint(traceID[:8], 16, 32)
	if err != nil {
		return false
	}
	return float64(n)/float64(0xffffffff) <= rate
}

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

func envDefault(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func otlpAttr(key string, value any) map[string]any {
	switch v := value.(type) {
	case bool:
		return map[string]any{"key": key, "value": map[string]any{"boolValue": v}}
	case int:
		return map[string]any{"key": key, "value": map[string]any{"intValue": strconv.Itoa(v)}}
	case int64:
		return map[string]any{"key": key, "value": map[string]any{"intValue": strconv.FormatInt(v, 10)}}
	case float64:
		return map[string]any{"key": key, "value": map[string]any{"doubleValue": v}}
	default:
		return map[string]any{"key": key, "value": map[string]any{"stringValue": strings.TrimSpace(toString(v))}}
	}
}

func toString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func postOTLP(endpoint string, payload map[string]any) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	client := &http.Client{Timeout: time.Second}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}
