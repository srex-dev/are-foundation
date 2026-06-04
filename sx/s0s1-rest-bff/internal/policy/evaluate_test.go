package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/srex-dev/are-foundation/tools/testing/policyfixtures"
)

func TestStubDemoForbidden(t *testing.T) {
	v := policyfixtures.Must("demo_forbidden_deny")
	e, r, err := (Stub{}).Evaluate(context.Background(), v.DecisionID, v.AgentID, v.ActionClass, v.Resource)
	if err != nil {
		t.Fatal(err)
	}
	if e != v.WantEffect || !strings.Contains(strings.ToLower(r), strings.ToLower(v.WantReasonSubstring)) {
		t.Fatalf("got %s %s", e, r)
	}
}

func TestStubDemoReadAllow(t *testing.T) {
	v := policyfixtures.Must("demo_read_allow")
	e, r, err := (Stub{}).Evaluate(context.Background(), v.DecisionID, v.AgentID, v.ActionClass, v.Resource)
	if err != nil {
		t.Fatal(err)
	}
	if e != v.WantEffect || r != v.WantReason {
		t.Fatalf("got %s %s", e, r)
	}
}

func TestStubTimeoutResource(t *testing.T) {
	v := policyfixtures.Must("dependency_timeout")
	_, _, err := (Stub{}).Evaluate(context.Background(), v.DecisionID, v.AgentID, v.ActionClass, v.Resource)
	if err != ErrDependencyTimeout {
		t.Fatalf("want timeout err got %v", err)
	}
}

func TestStubContractShapeGenericRead(t *testing.T) {
	v := policyfixtures.Must("contract_shape_generic_read")
	e, r, err := (Stub{}).Evaluate(context.Background(), v.DecisionID, v.AgentID, v.ActionClass, v.Resource)
	if err != nil {
		t.Fatal(err)
	}
	if e != v.WantEffect {
		t.Fatalf("effect: want %q got %q", v.WantEffect, e)
	}
	if e == "ALLOW" && r != "policy-evaluated" {
		t.Fatalf("reason: want default stub reason got %q", r)
	}
}

func TestOPAClientRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/data/are/evaluatepolicy/decision" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"effect":"ALLOW","reason":"opa"}}`))
	}))
	defer srv.Close()

	c := &OPAClient{BaseURL: srv.URL, HTTPClient: srv.Client()}
	e, reason, err := c.Evaluate(context.Background(), "1", "agt", "OTHER", "urn:x")
	if err != nil {
		t.Fatal(err)
	}
	if e != "ALLOW" || reason != "opa" {
		t.Fatalf("got %s %s", e, reason)
	}
}

func TestOPAClientDependencyTimeoutBeforeHTTP(t *testing.T) {
	c := NewOPAClient("http://127.0.0.1:9", time.Second)
	_, _, err := c.Evaluate(context.Background(), "1", "a", "READ", "timeout://x")
	if err != ErrDependencyTimeout {
		t.Fatalf("want timeout err got %v", err)
	}
}
