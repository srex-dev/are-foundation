package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type allowToken struct{}

func (allowToken) Validate(context.Context, string) (string, error) {
	return "test-subject", nil
}

func foundationRequest(method, path, body, idem string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("X-Request-ID", "req-1")
	req.Header.Set("X-ARE-Agent-ID", "demo-operator")
	req.Header.Set("Content-Type", "application/json")
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	return req
}

func TestFoundationRegisterIssueScopePolicyFlow(t *testing.T) {
	up := &StaticUpstreamClient{}
	gw := NewGateway(allowToken{}, up, nil)

	rec := httptest.NewRecorder()
	gw.Handler().ServeHTTP(rec, foundationRequest(http.MethodPost, "/v1/identity/agents", `{"agent_type":"demo.service","owner_id":"owner-1"}`, "agent-1"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", rec.Code, rec.Body.String())
	}

	passBody := `{"agent_id":"agt-agent-1","passport_type":"standard","requested_scopes":[{"action_class":"model.promote_to_production","resource_pattern":"model/*"}],"ttl_seconds":3600,"issued_by":"owner-1","reason":"demo"}`
	rec = httptest.NewRecorder()
	gw.Handler().ServeHTTP(rec, foundationRequest(http.MethodPost, "/v1/passports", passBody, "passport-1"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("passport status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	gw.Handler().ServeHTTP(rec, foundationRequest(http.MethodPost, "/v1/enforcement/scope:evaluate", `{"agent_id":"agt-agent-1","passport_id":"ppt-passport-1","action_class":"model.promote_to_production","resource":"model/champion"}`, "scope-1"))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"effect":"ALLOW"`) {
		t.Fatalf("scope status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	gw.Handler().ServeHTTP(rec, foundationRequest(http.MethodPost, "/v1/policy/evaluations", `{"decision_id":"dec-1","agent_id":"agt-agent-1","action_class":"model.promote_to_production","resource":"model/champion"}`, "policy-1"))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"effect":"ALLOW"`) {
		t.Fatalf("policy status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFoundationUnknownRouteFailsClosed(t *testing.T) {
	gw := NewGateway(allowToken{}, &StaticUpstreamClient{}, nil)
	rec := httptest.NewRecorder()
	gw.Handler().ServeHTTP(rec, foundationRequest(http.MethodPost, "/v1/execution/actions", `{}`, "exec-1"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected foundation route guard, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFoundationRouteContract(t *testing.T) {
	if !IsS0S1RESTProxyRoute(http.MethodPost, "/v1/passports:verify") {
		t.Fatal("expected passport verify in S0/S1 surface")
	}
	if IsS0S1RESTProxyRoute(http.MethodPost, "/v1/command-center/chat") {
		t.Fatal("command center must not be part of the foundation surface")
	}
}
