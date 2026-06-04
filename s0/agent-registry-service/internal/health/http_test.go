package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthAndReadyHandlers(t *testing.T) {
	mux := http.NewServeMux()
	Handler{Ready: func() bool { return true }}.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health status code = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ready status code = %d", rec.Code)
	}
}

func TestReadyHandlerNotReady(t *testing.T) {
	mux := http.NewServeMux()
	Handler{Ready: func() bool { return false }}.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}
