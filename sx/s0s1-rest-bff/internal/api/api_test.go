package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"are-s0s1-rest-bff/internal/policy"
	"are-s0s1-rest-bff/internal/repo"
	"github.com/srex-dev/are-foundation/tools/testing/policyfixtures"
)

func TestRegisterIssueEvaluateFlow(t *testing.T) {
	mem := repo.NewMemory()
	h, err := Handler(Config{Repo: mem})
	if err != nil {
		t.Fatal(err)
	}

	idem := "test-idem-1"
	regBody := `{"agent_type":"demo.service","owner_id":"o1","metadata":{}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/identity/agents", strings.NewReader(regBody))
	req.Header.Set("X-Request-ID", "r1")
	req.Header.Set("Idempotency-Key", idem)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"agent_id":"agt-`+idem+`"`) {
		t.Fatalf("agent id: %s", rec.Body.String())
	}

	agentID := "agt-" + idem
	vread := policyfixtures.Must("demo_read_allow")
	passJSON := fmt.Sprintf(`{"agent_id":"%s","passport_type":"standard","requested_scopes":[{"action_class":"%s","resource_pattern":"urn:x/*"}],"ttl_seconds":3600,"issued_by":"op","reason":"r"}`, agentID, vread.ActionClass)
	req2 := httptest.NewRequest(http.MethodPost, "/v1/passports", strings.NewReader(passJSON))
	req2.Header.Set("X-Request-ID", "r2")
	req2.Header.Set("Idempotency-Key", "pass-1")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusCreated {
		t.Fatalf("passport: %d %s", rec2.Code, rec2.Body.String())
	}

	vdeny := policyfixtures.Must("demo_forbidden_deny")
	polBody, err := vdeny.PolicyEvaluationBody(agentID)
	if err != nil {
		t.Fatal(err)
	}
	polJSON := string(polBody)
	req3 := httptest.NewRequest(http.MethodPost, "/v1/policy/evaluations", strings.NewReader(polJSON))
	req3.Header.Set("X-Request-ID", "r3")
	req3.Header.Set("Idempotency-Key", "pol-1")
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK || !strings.Contains(rec3.Body.String(), `"DENY"`) {
		t.Fatalf("policy: %d %s", rec3.Code, rec3.Body.String())
	}
}

func TestPassportListVerifyAndScopeEvaluation(t *testing.T) {
	mem := repo.NewMemory()
	h, err := Handler(Config{Repo: mem})
	if err != nil {
		t.Fatal(err)
	}

	register := httptest.NewRequest(http.MethodPost, "/v1/identity/agents", strings.NewReader(`{"agent_type":"demo.service","owner_id":"o1","metadata":{}}`))
	register.Header.Set("X-Request-ID", "r1")
	register.Header.Set("Idempotency-Key", "agent-list")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, register)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status %d body %s", rec.Code, rec.Body.String())
	}

	passportBody := `{"agent_id":"agt-agent-list","passport_type":"standard","requested_scopes":[{"action_class":"model.promote_to_production","resource_pattern":"model/*"}],"ttl_seconds":3600,"issued_by":"op","reason":"demo"}`
	passport := httptest.NewRequest(http.MethodPost, "/v1/passports", strings.NewReader(passportBody))
	passport.Header.Set("X-Request-ID", "r2")
	passport.Header.Set("Idempotency-Key", "passport-list")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, passport)
	if rec.Code != http.StatusCreated {
		t.Fatalf("passport status %d body %s", rec.Code, rec.Body.String())
	}

	list := httptest.NewRequest(http.MethodGet, "/v1/passports/by-agent/agt-agent-list", nil)
	list.Header.Set("X-Request-ID", "r3")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, list)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ppt-passport-list"`) {
		t.Fatalf("list status %d body %s", rec.Code, rec.Body.String())
	}

	verify := httptest.NewRequest(http.MethodPost, "/v1/passports:verify", strings.NewReader(`{"agent_id":"agt-agent-list","passport_id":"ppt-passport-list"}`))
	verify.Header.Set("X-Request-ID", "r4")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, verify)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"verified":true`) {
		t.Fatalf("verify status %d body %s", rec.Code, rec.Body.String())
	}

	scope := httptest.NewRequest(http.MethodPost, "/v1/enforcement/scope:evaluate", strings.NewReader(`{"agent_id":"agt-agent-list","passport_id":"ppt-passport-list","action_class":"model.promote_to_production","resource":"model/champion"}`))
	scope.Header.Set("X-Request-ID", "r5")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, scope)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"effect":"ALLOW"`) {
		t.Fatalf("scope status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestGetAdmissionEnvelopeFlow(t *testing.T) {
	mem := repo.NewMemory()
	h, err := Handler(Config{Repo: mem})
	if err != nil {
		t.Fatal(err)
	}
	idem := "idem-env-1"
	regBody := `{"agent_type":"demo.service","owner_id":"o1","metadata":{},"admission_envelope":{"envelope_id":"e1","admitted_scopes":["READ"]}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/identity/agents", strings.NewReader(regBody))
	req.Header.Set("X-Request-ID", "r1")
	req.Header.Set("Idempotency-Key", idem)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	agentID := "agt-" + idem
	req2 := httptest.NewRequest(http.MethodGet, "/v1/identity/agents/"+agentID+"/admission-envelope", nil)
	req2.Header.Set("X-Request-ID", "r2")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), `"envelope_id":"e1"`) {
		t.Fatalf("envelope: %d %s", rec2.Code, rec2.Body.String())
	}
}

func TestGetAgentUnknown404(t *testing.T) {
	mem := repo.NewMemory()
	h, err := Handler(Config{Repo: mem})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/identity/agents/nope", nil)
	req.Header.Set("X-Request-ID", "r1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", rec.Code)
	}
}

func TestEvaluatePolicyOPAClientIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/data/are/evaluatepolicy/decision" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"result":{"effect":"DENY","reason":"forbidden action class for demo"}}`))
	}))
	defer srv.Close()
	c := policy.NewOPAClient(srv.URL, time.Second)
	c.HTTPClient = srv.Client()
	mem := repo.NewMemory()
	h, err := Handler(Config{Repo: mem, Policy: c})
	if err != nil {
		t.Fatal(err)
	}
	vdeny := policyfixtures.Must("demo_forbidden_deny")
	bodyB, err := vdeny.PolicyEvaluationBody("agt-x")
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyB)
	req := httptest.NewRequest(http.MethodPost, "/v1/policy/evaluations", strings.NewReader(body))
	req.Header.Set("X-Request-ID", "r1")
	req.Header.Set("Idempotency-Key", "k1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"DENY"`) {
		t.Fatalf("want 200 DENY got %d %s", rec.Code, rec.Body.String())
	}
}

func TestHandlerNilRepoReturnsError(t *testing.T) {
	_, err := Handler(Config{Repo: nil})
	if err == nil {
		t.Fatal("expected error for nil repo")
	}
	if err != ErrNilRepository {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestRegisterIdempotency(t *testing.T) {
	mem := repo.NewMemory()
	h, err := Handler(Config{Repo: mem})
	if err != nil {
		t.Fatal(err)
	}
	idem := "idem-z"
	body := []byte(`{"agent_type":"a","owner_id":"o"}`)
	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/identity/agents", bytes.NewReader(body))
		req.Header.Set("X-Request-ID", "r")
		req.Header.Set("Idempotency-Key", idem)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	r1 := do()
	r2 := do()
	if r1.Body.String() != r2.Body.String() {
		t.Fatalf("idem mismatch:\n%s\n%s", r1.Body, r2.Body)
	}
}

func TestRegisterIdempotencyBodyConflict(t *testing.T) {
	mem := repo.NewMemory()
	h, err := Handler(Config{Repo: mem})
	if err != nil {
		t.Fatal(err)
	}
	idem := "idem-conflict"
	req1 := httptest.NewRequest(http.MethodPost, "/v1/identity/agents",
		strings.NewReader(`{"agent_type":"typeA","owner_id":"owner1"}`))
	req1.Header.Set("X-Request-ID", "r1")
	req1.Header.Set("Idempotency-Key", idem)
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first register: want 201, got %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/v1/identity/agents",
		strings.NewReader(`{"agent_type":"typeB","owner_id":"owner2"}`))
	req2.Header.Set("X-Request-ID", "r2")
	req2.Header.Set("Idempotency-Key", idem)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("body conflict: want 409, got %d: %s", rec2.Code, rec2.Body.String())
	}
}
