package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWrapCORSIfConfigured_NoOrigins(t *testing.T) {
	called := false
	h := WrapCORSIfConfigured(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte("ok"))
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatal("expected inner handler")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestWrapCORSIfConfigured_Preflight(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner should not run for OPTIONS")
	})
	h := WrapCORSIfConfigured([]string{"http://localhost:5173"}, inner)
	req := httptest.NewRequest(http.MethodOptions, "/v1/identity/agents", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("allow-origin: %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestParseCORSAllowedOrigins(t *testing.T) {
	got := ParseCORSAllowedOrigins(" http://a , ,http://b ")
	if len(got) != 2 || got[0] != "http://a" || got[1] != "http://b" {
		t.Fatalf("%#v", got)
	}
}
