package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"are-s0s1-rest-bff/internal/policy"
	"are-s0s1-rest-bff/internal/repo"
	"are-s0s1-rest-bff/internal/s0delegate"
)

func jsonError(w http.ResponseWriter, requestID string, code, message string, retryable bool, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":       code,
			"message":    message,
			"request_id": requestID,
			"retryable":  retryable,
		},
	})
}

func requestID(r *http.Request) string {
	return r.Header.Get("X-Request-ID")
}

// registerBodyConflict returns true when the cached idempotent response was
// produced for a different (agent_type, owner_id) pair than the current request.
func registerBodyConflict(cachedResp []byte, newAgentType, newOwnerID string) bool {
	var resp struct {
		Agent struct {
			AgentType string `json:"agent_type"`
			OwnerID   string `json:"owner_id"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(cachedResp, &resp); err != nil {
		return false
	}
	return strings.TrimSpace(resp.Agent.AgentType) != strings.TrimSpace(newAgentType) ||
		strings.TrimSpace(resp.Agent.OwnerID) != strings.TrimSpace(newOwnerID)
}

// Config wires the v1 API mux (TLS / logging stay outside).
type Config struct {
	Repo   repo.Repository
	Policy policy.Evaluator // nil => policy.Stub (same rules as gateway static upstream)
	// S0 optional: when dialled from main, Register/Get/Issue can delegate to ARE-A-S0-001/005.
	S0 *s0delegate.Backend
}

// ErrNilRepository indicates Handler was called without a repository.
var ErrNilRepository = errors.New("api.Handler: Repo is nil")

// Handler returns the v1 API mux.
func Handler(cfg Config) (http.Handler, error) {
	if cfg.Repo == nil {
		return nil, ErrNilRepository
	}
	eval := cfg.Policy
	if eval == nil {
		eval = policy.Stub{}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/identity/agents", func(w http.ResponseWriter, r *http.Request) {
		registerAgent(w, r, &cfg)
	})
	mux.HandleFunc("GET /v1/identity/agents/", func(w http.ResponseWriter, r *http.Request) {
		getAgent(w, r, &cfg)
	})
	mux.HandleFunc("POST /v1/passports", func(w http.ResponseWriter, r *http.Request) {
		issuePassport(w, r, &cfg)
	})
	mux.HandleFunc("GET /v1/passports/by-agent/", func(w http.ResponseWriter, r *http.Request) {
		listPassportsByAgent(w, r, &cfg)
	})
	mux.HandleFunc("POST /v1/passports:verify", func(w http.ResponseWriter, r *http.Request) {
		verifyPassport(w, r, &cfg)
	})
	mux.HandleFunc("POST /v1/enforcement/scope:evaluate", func(w http.ResponseWriter, r *http.Request) {
		evaluateScope(w, r, &cfg)
	})
	mux.HandleFunc("POST /v1/policy/evaluations", func(w http.ResponseWriter, r *http.Request) {
		evaluatePolicy(w, r, eval)
	})
	mux.HandleFunc("GET /v1/meta/deployment", func(w http.ResponseWriter, r *http.Request) {
		deploymentMeta(w, r)
	})
	return loggingMiddleware(mux), nil
}

// statusRecorder captures the HTTP status for structured logs (Gate 5 — correlate with gateway).
type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

// loggingMiddleware emits JSON structured logs (PA-3 pilot) aligned with gateway X-Request-ID.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rid := requestID(r)
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.InfoContext(r.Context(), "s0s1_bff_request",
			slog.String("request_id", rid),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("response_code", rec.code),
			slog.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
	})
}

func deploymentMeta(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r)
	stage := strings.TrimSpace(strings.ToLower(os.Getenv("ARE_DEPLOYMENT_STAGE")))
	if stage == "" {
		stage = strings.TrimSpace(strings.ToLower(os.Getenv("ARE_S0S1_DEPLOYMENT_STAGE")))
	}
	if stage == "" {
		stage = "s0s1"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"deployment_stage": stage,
		"request_id":       rid,
	})
}

func registerAgent(w http.ResponseWriter, r *http.Request, cfg *Config) {
	ctx := r.Context()
	rid := requestID(r)
	rp := cfg.Repo
	idem := r.Header.Get("Idempotency-Key")
	if idem == "" {
		jsonError(w, rid, "MISSING_IDEMPOTENCY_KEY", "idempotency key required", false, http.StatusBadRequest)
		return
	}

	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		jsonError(w, rid, "INVALID_ARGUMENT", "invalid json body", false, http.StatusBadRequest)
		return
	}
	agentType, _ := payload["agent_type"].(string)
	ownerID, _ := payload["owner_id"].(string)
	if strings.TrimSpace(agentType) == "" || strings.TrimSpace(ownerID) == "" {
		jsonError(w, rid, "INVALID_ARGUMENT", "agent_type and owner_id are required", false, http.StatusBadRequest)
		return
	}

	key := "RegisterAgent:" + idem
	prev, ok, err := rp.GetRegisterIdem(ctx, key)
	if err != nil {
		jsonError(w, rid, "INTERNAL", "storage error", false, http.StatusInternalServerError)
		return
	}
	if ok {
		if registerBodyConflict(prev, agentType, ownerID) {
			jsonError(w, rid, "IDEMPOTENCY_CONFLICT", "idempotency key reused with different body", false, http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(prev)
		return
	}
	meta, _ := payload["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
	}
	var admission map[string]any
	if raw, ok := payload["admission_envelope"].(map[string]any); ok && raw != nil {
		admission = raw
	}
	if cfg.S0 != nil && cfg.S0.Reg != nil {
		body, agentID, err := s0delegate.Register(ctx, cfg.S0.Reg, agentType, ownerID, meta, admission)
		if err != nil {
			writeS0DelegateError(w, rid, err)
			return
		}
		rec, err := agentRecFromRegisterResponseBody(body, agentType, ownerID)
		if err != nil {
			jsonError(w, rid, "INTERNAL", "marshal failed", false, http.StatusInternalServerError)
			return
		}
		if len(admission) > 0 {
			rec.AdmissionEnvelope = admission
		}
		out, err := rp.FinishRegister(ctx, key, agentID, body, rec)
		if err != nil {
			jsonError(w, rid, "INTERNAL", "storage error", false, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(out)
		return
	}
	agentID := "agt-" + idem
	rec := repo.AgentRec{AgentType: agentType, OwnerID: ownerID, Metadata: meta, Status: "ACTIVE", AdmissionEnvelope: admission}
	bodyMap := map[string]any{
		"agent": map[string]any{
			"agent_id":   agentID,
			"agent_type": agentType,
			"owner_id":   ownerID,
			"status":     "ACTIVE",
		},
	}
	body, err := json.Marshal(bodyMap)
	if err != nil {
		jsonError(w, rid, "INTERNAL", "marshal failed", false, http.StatusInternalServerError)
		return
	}
	out, err := rp.FinishRegister(ctx, key, agentID, body, rec)
	if err != nil {
		jsonError(w, rid, "INTERNAL", "storage error", false, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(out)
}

func agentPathSuffix(path string) (suffix string, ok bool) {
	const pfx = "/v1/identity/agents/"
	if !strings.HasPrefix(path, pfx) {
		return "", false
	}
	s := strings.TrimPrefix(path, pfx)
	if s == "" {
		return "", false
	}
	return s, true
}

func getAgent(w http.ResponseWriter, r *http.Request, cfg *Config) {
	ctx := r.Context()
	rid := requestID(r)
	rp := cfg.Repo
	suffix, ok := agentPathSuffix(r.URL.Path)
	if !ok {
		jsonError(w, rid, "INVALID_ARGUMENT", "agent id required", false, http.StatusBadRequest)
		return
	}
	if strings.HasSuffix(suffix, "/admission-envelope") {
		agentID := strings.TrimSpace(strings.TrimSuffix(suffix, "/admission-envelope"))
		if agentID == "" || strings.Contains(agentID, "/") {
			jsonError(w, rid, "INVALID_ARGUMENT", "agent id required", false, http.StatusBadRequest)
			return
		}
		getAdmissionEnvelope(w, r, cfg, agentID)
		return
	}
	agentID := strings.TrimSuffix(suffix, ":check")
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		jsonError(w, rid, "INVALID_ARGUMENT", "agent id required", false, http.StatusBadRequest)
		return
	}
	if cfg.S0 != nil && cfg.S0.Reg != nil {
		body, err := s0delegate.GetAgentJSON(ctx, cfg.S0.Reg, agentID)
		if err != nil {
			if s0delegate.IsNotFound(err) {
				jsonError(w, rid, "NOT_FOUND", "unknown agent_id", false, http.StatusNotFound)
				return
			}
			writeS0DelegateError(w, rid, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}
	rec, found, err := rp.GetAgent(ctx, agentID)
	if err != nil {
		jsonError(w, rid, "INTERNAL", "storage error", false, http.StatusInternalServerError)
		return
	}
	if !found {
		jsonError(w, rid, "NOT_FOUND", "unknown agent_id", false, http.StatusNotFound)
		return
	}
	body, err := json.Marshal(map[string]any{
		"agent": map[string]any{
			"agent_id":   agentID,
			"agent_type": rec.AgentType,
			"owner_id":   rec.OwnerID,
			"status":     rec.Status,
		},
	})
	if err != nil {
		jsonError(w, rid, "INTERNAL", "marshal failed", false, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func getAdmissionEnvelope(w http.ResponseWriter, r *http.Request, cfg *Config, agentID string) {
	ctx := r.Context()
	rid := requestID(r)
	rp := cfg.Repo
	if cfg.S0 != nil && cfg.S0.Reg != nil {
		body, err := s0delegate.GetAdmissionEnvelopeJSON(ctx, cfg.S0.Reg, agentID)
		if err != nil {
			if s0delegate.IsNotFound(err) {
				jsonError(w, rid, "NOT_FOUND", "admission envelope not found", false, http.StatusNotFound)
				return
			}
			writeS0DelegateError(w, rid, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}
	env, found, err := rp.GetAdmissionEnvelope(ctx, agentID)
	if err != nil {
		jsonError(w, rid, "INTERNAL", "storage error", false, http.StatusInternalServerError)
		return
	}
	if !found {
		jsonError(w, rid, "NOT_FOUND", "admission envelope not found", false, http.StatusNotFound)
		return
	}
	body, err := json.Marshal(map[string]any{
		"agent_id":           agentID,
		"admission_envelope": env,
	})
	if err != nil {
		jsonError(w, rid, "INTERNAL", "marshal failed", false, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func issuePassport(w http.ResponseWriter, r *http.Request, cfg *Config) {
	ctx := r.Context()
	rid := requestID(r)
	rp := cfg.Repo
	idem := r.Header.Get("Idempotency-Key")
	if idem == "" {
		jsonError(w, rid, "MISSING_IDEMPOTENCY_KEY", "idempotency key required", false, http.StatusBadRequest)
		return
	}
	key := "IssuePassport:" + idem

	prev, ok, err := rp.GetPassportIdem(ctx, key)
	if err != nil {
		jsonError(w, rid, "INTERNAL", "storage error", false, http.StatusInternalServerError)
		return
	}
	if ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(prev)
		return
	}

	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		jsonError(w, rid, "INVALID_ARGUMENT", "invalid json body", false, http.StatusBadRequest)
		return
	}
	agentID, _ := payload["agent_id"].(string)
	passportType, _ := payload["passport_type"].(string)
	issuedBy, _ := payload["issued_by"].(string)
	reason, _ := payload["reason"].(string)
	scopesRaw, scopesOK := payload["requested_scopes"].([]any)
	ttl, ttlOK := asInt64(payload["ttl_seconds"])
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(passportType) == "" || !scopesOK || len(scopesRaw) == 0 ||
		!ttlOK || strings.TrimSpace(issuedBy) == "" || strings.TrimSpace(reason) == "" {
		jsonError(w, rid, "INVALID_ARGUMENT", "agent_id, passport_type, requested_scopes, ttl_seconds, issued_by, and reason are required", false, http.StatusBadRequest)
		return
	}

	scopeRows := make([]map[string]any, 0, len(scopesRaw))
	for _, item := range scopesRaw {
		row, ok := item.(map[string]any)
		if !ok {
			jsonError(w, rid, "INVALID_ARGUMENT", "invalid requested_scopes entry", false, http.StatusBadRequest)
			return
		}
		ac, _ := row["action_class"].(string)
		resPat, _ := row["resource_pattern"].(string)
		if strings.TrimSpace(ac) == "" || strings.TrimSpace(resPat) == "" {
			jsonError(w, rid, "INVALID_ARGUMENT", "requested_scopes require action_class and resource_pattern", false, http.StatusBadRequest)
			return
		}
		scopeRows = append(scopeRows, row)
	}

	found, err := existsAgentForPassport(ctx, cfg, rp, agentID)
	if err != nil {
		jsonError(w, rid, "INTERNAL", "storage error", false, http.StatusInternalServerError)
		return
	}
	if !found {
		jsonError(w, rid, "NOT_FOUND", "unknown agent_id", false, http.StatusNotFound)
		return
	}

	if cfg.S0 != nil && cfg.S0.Pass != nil {
		forceReissue, _ := payload["force_reissue"].(bool)
		pReq := s0delegate.BuildIssueRequest(agentID, passportType, issuedBy, reason, ttl, scopeRows, forceReissue)
		body, err := s0delegate.IssuePassportJSON(ctx, cfg.S0.Pass, pReq)
		if err != nil {
			writeS0DelegateError(w, rid, err)
			return
		}
		out, err := rp.FinishPassport(ctx, key, agentID, body)
		if err != nil {
			if err == repo.ErrAgentNotFound {
				jsonError(w, rid, "NOT_FOUND", "unknown agent_id", false, http.StatusNotFound)
				return
			}
			jsonError(w, rid, "INTERNAL", "storage error", false, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(out)
		return
	}

	scopeSet := make([]map[string]any, 0, len(scopeRows))
	for i, row := range scopeRows {
		ac, _ := row["action_class"].(string)
		resPat, _ := row["resource_pattern"].(string)
		scopeSet = append(scopeSet, map[string]any{
			"scope_id":         fmt.Sprintf("scp-%s-%d", idem, i),
			"action_class":     ac,
			"resource_pattern": resPat,
		})
	}

	passportID := "ppt-" + idem
	bodyMap := map[string]any{
		"passport": map[string]any{
			"passport_id":   passportID,
			"agent_id":      agentID,
			"passport_type": passportType,
			"scope_set":     scopeSet,
			"status":        "ACTIVE",
			"issued_by":     issuedBy,
		},
	}
	body, err := json.Marshal(bodyMap)
	if err != nil {
		jsonError(w, rid, "INTERNAL", "marshal failed", false, http.StatusInternalServerError)
		return
	}
	out, err := rp.FinishPassport(ctx, key, agentID, body)
	if err != nil {
		if err == repo.ErrAgentNotFound {
			jsonError(w, rid, "NOT_FOUND", "unknown agent_id", false, http.StatusNotFound)
			return
		}
		jsonError(w, rid, "INTERNAL", "storage error", false, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(out)
}

func listPassportsByAgent(w http.ResponseWriter, r *http.Request, cfg *Config) {
	const pfx = "/v1/passports/by-agent/"
	rid := requestID(r)
	agentID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, pfx))
	if agentID == "" || strings.Contains(agentID, "/") {
		jsonError(w, rid, "INVALID_ARGUMENT", "agent id required", false, http.StatusBadRequest)
		return
	}
	bodies, err := cfg.Repo.ListPassportBodiesByAgent(r.Context(), agentID)
	if err != nil {
		jsonError(w, rid, "INTERNAL", "storage error", false, http.StatusInternalServerError)
		return
	}
	passports := make([]map[string]any, 0, len(bodies))
	for _, body := range bodies {
		if passport := publicPassportFromBody(body); passport != nil {
			passports = append(passports, passport)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"agent_id":   agentID,
		"passports":  passports,
		"request_id": rid,
	})
}

func verifyPassport(w http.ResponseWriter, r *http.Request, cfg *Config) {
	rid := requestID(r)
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		jsonError(w, rid, "INVALID_ARGUMENT", "invalid json body", false, http.StatusBadRequest)
		return
	}
	passportID, _ := payload["passport_id"].(string)
	agentID, _ := payload["agent_id"].(string)
	if strings.TrimSpace(passportID) == "" || strings.TrimSpace(agentID) == "" {
		jsonError(w, rid, "INVALID_ARGUMENT", "passport_id and agent_id are required", false, http.StatusBadRequest)
		return
	}
	body, found, err := cfg.Repo.GetPassportBody(r.Context(), passportID)
	if err != nil {
		jsonError(w, rid, "INTERNAL", "storage error", false, http.StatusInternalServerError)
		return
	}
	passport := publicPassportFromBody(body)
	verified := found && passport != nil && passport["agent_id"] == agentID && passport["status"] == "ACTIVE"
	reason := "passport verified"
	if !verified {
		reason = "passport missing, revoked, expired, or not bound to agent"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"verified":     verified,
		"reason":       reason,
		"passport_id":  passportID,
		"agent_id":     agentID,
		"request_id":   rid,
		"executed":     false,
		"proof_status": "reference_only",
	})
}

func evaluateScope(w http.ResponseWriter, r *http.Request, cfg *Config) {
	rid := requestID(r)
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		jsonError(w, rid, "INVALID_ARGUMENT", "invalid json body", false, http.StatusBadRequest)
		return
	}
	agentID, _ := payload["agent_id"].(string)
	passportID, _ := payload["passport_id"].(string)
	actionClass, _ := payload["action_class"].(string)
	resource, _ := payload["resource"].(string)
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(actionClass) == "" || strings.TrimSpace(resource) == "" {
		jsonError(w, rid, "INVALID_ARGUMENT", "agent_id, action_class, and resource are required", false, http.StatusBadRequest)
		return
	}
	allowed, reason, err := scopeAllows(r.Context(), cfg.Repo, agentID, passportID, actionClass, resource)
	if err != nil {
		jsonError(w, rid, "INTERNAL", "storage error", false, http.StatusInternalServerError)
		return
	}
	effect := "DENY"
	if allowed {
		effect = "ALLOW"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"decision": map[string]any{
			"effect":       effect,
			"reason":       reason,
			"agent_id":     agentID,
			"action_class": actionClass,
			"resource":     resource,
		},
		"request_id": rid,
		"executed":   false,
	})
}

func publicPassportFromBody(body []byte) map[string]any {
	var envelope struct {
		Passport map[string]any `json:"passport"`
	}
	if len(body) == 0 || json.Unmarshal(body, &envelope) != nil || envelope.Passport == nil {
		return nil
	}
	return envelope.Passport
}

func scopeAllows(ctx context.Context, rp repo.Repository, agentID, passportID, actionClass, resource string) (bool, string, error) {
	matches := func(passport map[string]any) bool {
		if passport == nil || passport["agent_id"] != agentID || passport["status"] != "ACTIVE" {
			return false
		}
		scopes, _ := passport["scope_set"].([]any)
		for _, item := range scopes {
			row, _ := item.(map[string]any)
			if row == nil {
				continue
			}
			ac, _ := row["action_class"].(string)
			rp, _ := row["resource_pattern"].(string)
			if ac == actionClass && resourcePatternMatches(rp, resource) {
				return true
			}
		}
		return false
	}
	if strings.TrimSpace(passportID) != "" {
		body, found, err := rp.GetPassportBody(ctx, passportID)
		if err != nil {
			return false, "", err
		}
		if !found {
			return false, "passport not found", nil
		}
		if matches(publicPassportFromBody(body)) {
			return true, "scope matched requested passport", nil
		}
		return false, "scope missing or passport inactive", nil
	}
	bodies, err := rp.ListPassportBodiesByAgent(ctx, agentID)
	if err != nil {
		return false, "", err
	}
	for _, body := range bodies {
		if matches(publicPassportFromBody(body)) {
			return true, "scope matched active passport", nil
		}
	}
	return false, "no active passport scope matched", nil
}

func resourcePatternMatches(pattern, resource string) bool {
	pattern = strings.TrimSpace(pattern)
	switch {
	case pattern == "*" || pattern == resource:
		return true
	case strings.HasSuffix(pattern, "*"):
		return strings.HasPrefix(resource, strings.TrimSuffix(pattern, "*"))
	default:
		return false
	}
}

func evaluatePolicy(w http.ResponseWriter, r *http.Request, eval policy.Evaluator) {
	rid := requestID(r)
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		jsonError(w, rid, "INVALID_ARGUMENT", "invalid json body", false, http.StatusBadRequest)
		return
	}
	decisionID, _ := payload["decision_id"].(string)
	agentID, _ := payload["agent_id"].(string)
	actionClass, _ := payload["action_class"].(string)
	resource, _ := payload["resource"].(string)
	if strings.TrimSpace(decisionID) == "" || strings.TrimSpace(agentID) == "" || strings.TrimSpace(actionClass) == "" {
		jsonError(w, rid, "INVALID_ARGUMENT", "decision_id, agent_id, and action_class are required", false, http.StatusBadRequest)
		return
	}
	effect, reason, err := eval.Evaluate(r.Context(), decisionID, agentID, actionClass, resource)
	if err != nil {
		if errors.Is(err, policy.ErrDependencyTimeout) {
			jsonError(w, rid, "DEPENDENCY_TIMEOUT", "policy engine timeout", true, http.StatusGatewayTimeout)
			return
		}
		jsonError(w, rid, "POLICY_ENGINE_UNAVAILABLE", err.Error(), true, http.StatusBadGateway)
		return
	}
	if effect != "ALLOW" && effect != "DENY" {
		jsonError(w, rid, "INTERNAL", "invalid policy effect", false, http.StatusInternalServerError)
		return
	}
	body, err := json.Marshal(map[string]any{
		"decision": map[string]any{
			"decision_id": decisionID,
			"effect":      effect,
			"reason":      reason,
		},
	})
	if err != nil {
		jsonError(w, rid, "INTERNAL", "marshal failed", false, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func asInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case int64:
		return x, true
	case int:
		return int64(x), true
	case json.Number:
		n, err := x.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}
