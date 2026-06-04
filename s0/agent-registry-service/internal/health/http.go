package health

import (
	"encoding/json"
	"net/http"
)

// Handler serves liveness and readiness probes.
type Handler struct {
	Ready func() bool
}

// Register mounts /healthz and /readyz handlers.
func (h Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if h.Ready != nil && !h.Ready() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "not_ready"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})
}
