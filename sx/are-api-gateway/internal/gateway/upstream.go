package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type agentRecord struct {
	AgentType string
	OwnerID   string
}

type passportRecord struct {
	PassportID   string
	AgentID      string
	PassportType string
	ScopeSet     []map[string]any
	Status       string
	IssuedBy     string
	ExpiresAt    time.Time
}

// StaticUpstreamClient is a public-safe local foundation simulator.
type StaticUpstreamClient struct {
	mu                 sync.Mutex
	idempotentResp     map[string]UpstreamResponse
	idempotentBodyHash map[string]string
	registeredAgents   map[string]agentRecord
	passports          map[string]passportRecord
	passportsByAgent   map[string][]string
	opaBaseURL         string
	opaHTTP            *http.Client
}

func (s *StaticUpstreamClient) ensure() {
	if s.idempotentResp == nil {
		s.idempotentResp = map[string]UpstreamResponse{}
	}
	if s.idempotentBodyHash == nil {
		s.idempotentBodyHash = map[string]string{}
	}
	if s.registeredAgents == nil {
		s.registeredAgents = map[string]agentRecord{}
	}
	if s.passports == nil {
		s.passports = map[string]passportRecord{}
	}
	if s.passportsByAgent == nil {
		s.passportsByAgent = map[string][]string{}
	}
}

func (s *StaticUpstreamClient) Call(ctx context.Context, req UpstreamRequest) (UpstreamResponse, error) {
	route, ok := matchPhase1Route(req.Method, req.Path)
	if !ok {
		return jsonError(req.RequestID, "NOT_FOUND", "route is not part of the ARE Foundation S0/S1 surface", false, http.StatusNotFound)
	}
	switch route.Method {
	case "RegisterAgent":
		return s.handleRegisterAgent(req)
	case "GetAgent":
		return s.handleGetAgent(req)
	case "IssuePassport":
		return s.handleIssuePassport(req)
	case "ListPassportsByAgent":
		return s.handleListPassportsByAgent(req)
	case "VerifyPassport":
		return s.handleVerifyPassport(req)
	case "EvaluateScope":
		return s.handleEvaluateScope(req)
	case "EvaluatePolicy":
		return s.handleEvaluatePolicy(ctx, req)
	case "GetDeploymentMeta":
		return s.handleGetDeploymentMeta(req)
	default:
		return jsonError(req.RequestID, "NOT_FOUND", "foundation route has no handler", false, http.StatusNotFound)
	}
}

func (s *StaticUpstreamClient) handleGetDeploymentMeta(req UpstreamRequest) (UpstreamResponse, error) {
	stage := strings.TrimSpace(strings.ToLower(os.Getenv("ARE_DEPLOYMENT_STAGE")))
	if stage == "" {
		stage = "stage-s0s1"
	}
	body, err := json.Marshal(map[string]any{
		"deployment_stage": stage,
		"request_id":       req.RequestID,
		"foundation":       "stage-s0s1",
	})
	if err != nil {
		return UpstreamResponse{}, fmt.Errorf("marshal deployment meta: %w", err)
	}
	return UpstreamResponse{StatusCode: http.StatusOK, Body: body}, nil
}

func registerAgentBodyFingerprint(body []byte) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	agentType, _ := payload["agent_type"].(string)
	ownerID, _ := payload["owner_id"].(string)
	return strings.TrimSpace(agentType) + "\x00" + strings.TrimSpace(ownerID), nil
}

func (s *StaticUpstreamClient) handleRegisterAgent(req UpstreamRequest) (UpstreamResponse, error) {
	if req.IdempotencyKey == "" {
		return jsonError(req.RequestID, "MISSING_IDEMPOTENCY_KEY", "idempotency key required", false, http.StatusBadRequest)
	}
	idempotencyKey := "RegisterAgent:" + req.IdempotencyKey
	fp, fpErr := registerAgentBodyFingerprint(req.Body)

	s.mu.Lock()
	s.ensure()
	existing, ok := s.idempotentResp[idempotencyKey]
	if ok {
		prevFP := s.idempotentBodyHash[idempotencyKey]
		s.mu.Unlock()
		if fpErr != nil {
			return jsonError(req.RequestID, "INVALID_ARGUMENT", "invalid json body", false, http.StatusBadRequest)
		}
		if prevFP != "" && prevFP != fp {
			return jsonError(req.RequestID, "IDEMPOTENCY_CONFLICT", "idempotency key replay with conflicting body", false, http.StatusConflict)
		}
		return existing, nil
	}
	s.mu.Unlock()

	var payload map[string]any
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		return jsonError(req.RequestID, "INVALID_ARGUMENT", "invalid json body", false, http.StatusBadRequest)
	}
	agentType, _ := payload["agent_type"].(string)
	ownerID, _ := payload["owner_id"].(string)
	if strings.TrimSpace(agentType) == "" || strings.TrimSpace(ownerID) == "" {
		return jsonError(req.RequestID, "INVALID_ARGUMENT", "agent_type and owner_id are required", false, http.StatusBadRequest)
	}
	agentID := "agt-" + req.IdempotencyKey
	body, err := json.Marshal(map[string]any{
		"agent": map[string]any{
			"agent_id":   agentID,
			"agent_type": agentType,
			"owner_id":   ownerID,
			"status":     "ACTIVE",
		},
	})
	if err != nil {
		return UpstreamResponse{}, fmt.Errorf("marshal register agent response: %w", err)
	}
	upstreamResp := UpstreamResponse{StatusCode: http.StatusCreated, Body: body}
	s.mu.Lock()
	s.ensure()
	s.idempotentResp[idempotencyKey] = upstreamResp
	if fpErr == nil {
		s.idempotentBodyHash[idempotencyKey] = fp
	}
	s.registeredAgents[agentID] = agentRecord{AgentType: agentType, OwnerID: ownerID}
	s.mu.Unlock()
	return upstreamResp, nil
}

func (s *StaticUpstreamClient) handleGetAgent(req UpstreamRequest) (UpstreamResponse, error) {
	const prefix = "/v1/identity/agents/"
	agentID := strings.TrimSpace(strings.TrimPrefix(req.Path, prefix))
	if agentID == "" || strings.Contains(agentID, "/") {
		return jsonError(req.RequestID, "INVALID_ARGUMENT", "agent id required", false, http.StatusBadRequest)
	}
	s.mu.Lock()
	s.ensure()
	rec, ok := s.registeredAgents[agentID]
	s.mu.Unlock()
	if !ok {
		return jsonError(req.RequestID, "NOT_FOUND", "unknown agent_id", false, http.StatusNotFound)
	}
	body, err := json.Marshal(map[string]any{
		"agent": map[string]any{
			"agent_id":   agentID,
			"agent_type": rec.AgentType,
			"owner_id":   rec.OwnerID,
			"status":     "ACTIVE",
		},
	})
	if err != nil {
		return UpstreamResponse{}, fmt.Errorf("marshal get agent response: %w", err)
	}
	return UpstreamResponse{StatusCode: http.StatusOK, Body: body}, nil
}

func (s *StaticUpstreamClient) handleIssuePassport(req UpstreamRequest) (UpstreamResponse, error) {
	if req.IdempotencyKey == "" {
		return jsonError(req.RequestID, "MISSING_IDEMPOTENCY_KEY", "idempotency key required", false, http.StatusBadRequest)
	}
	idempotencyKey := "IssuePassport:" + req.IdempotencyKey
	s.mu.Lock()
	s.ensure()
	existing, ok := s.idempotentResp[idempotencyKey]
	if ok {
		s.mu.Unlock()
		return existing, nil
	}
	s.mu.Unlock()

	var payload map[string]any
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		return jsonError(req.RequestID, "INVALID_ARGUMENT", "invalid json body", false, http.StatusBadRequest)
	}
	agentID, _ := payload["agent_id"].(string)
	passportType, _ := payload["passport_type"].(string)
	issuedBy, _ := payload["issued_by"].(string)
	reason, _ := payload["reason"].(string)
	scopesRaw, scopesOK := payload["requested_scopes"].([]any)
	ttl, ttlOK := asInt64(payload["ttl_seconds"])
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(passportType) == "" || !scopesOK || len(scopesRaw) == 0 ||
		!ttlOK || ttl <= 0 || strings.TrimSpace(issuedBy) == "" || strings.TrimSpace(reason) == "" {
		return jsonError(req.RequestID, "INVALID_ARGUMENT", "agent_id, passport_type, requested_scopes, ttl_seconds, issued_by, and reason are required", false, http.StatusBadRequest)
	}

	s.mu.Lock()
	s.ensure()
	_, registered := s.registeredAgents[agentID]
	s.mu.Unlock()
	if !registered {
		return jsonError(req.RequestID, "NOT_FOUND", "unknown agent_id", false, http.StatusNotFound)
	}

	scopeSet := make([]map[string]any, 0, len(scopesRaw))
	for i, item := range scopesRaw {
		row, ok := item.(map[string]any)
		if !ok {
			return jsonError(req.RequestID, "INVALID_ARGUMENT", "invalid requested_scopes entry", false, http.StatusBadRequest)
		}
		actionClass, _ := row["action_class"].(string)
		resourcePattern, _ := row["resource_pattern"].(string)
		if strings.TrimSpace(actionClass) == "" || strings.TrimSpace(resourcePattern) == "" {
			return jsonError(req.RequestID, "INVALID_ARGUMENT", "requested_scopes require action_class and resource_pattern", false, http.StatusBadRequest)
		}
		scopeSet = append(scopeSet, map[string]any{
			"scope_id":         fmt.Sprintf("scp-%s-%d", req.IdempotencyKey, i),
			"action_class":     actionClass,
			"resource_pattern": resourcePattern,
		})
	}

	passportID := "ppt-" + req.IdempotencyKey
	rec := passportRecord{
		PassportID:   passportID,
		AgentID:      agentID,
		PassportType: passportType,
		ScopeSet:     scopeSet,
		Status:       "ACTIVE",
		IssuedBy:     issuedBy,
		ExpiresAt:    time.Now().UTC().Add(time.Duration(ttl) * time.Second),
	}
	body, err := json.Marshal(map[string]any{
		"passport": map[string]any{
			"passport_id":   rec.PassportID,
			"agent_id":      rec.AgentID,
			"passport_type": rec.PassportType,
			"scope_set":     rec.ScopeSet,
			"status":        rec.Status,
			"issued_by":     rec.IssuedBy,
			"expires_at":    rec.ExpiresAt.Format(time.RFC3339),
		},
	})
	if err != nil {
		return UpstreamResponse{}, fmt.Errorf("marshal issue passport response: %w", err)
	}
	upstreamResp := UpstreamResponse{StatusCode: http.StatusCreated, Body: body}
	s.mu.Lock()
	s.ensure()
	s.idempotentResp[idempotencyKey] = upstreamResp
	s.passports[passportID] = rec
	s.passportsByAgent[agentID] = append(s.passportsByAgent[agentID], passportID)
	s.mu.Unlock()
	return upstreamResp, nil
}

func (s *StaticUpstreamClient) handleListPassportsByAgent(req UpstreamRequest) (UpstreamResponse, error) {
	const prefix = "/v1/passports/by-agent/"
	agentID := strings.TrimSpace(strings.TrimPrefix(req.Path, prefix))
	if agentID == "" || strings.Contains(agentID, "/") {
		return jsonError(req.RequestID, "INVALID_ARGUMENT", "agent id required", false, http.StatusBadRequest)
	}
	s.mu.Lock()
	s.ensure()
	ids := append([]string(nil), s.passportsByAgent[agentID]...)
	rows := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		rec := s.passports[id]
		rows = append(rows, publicPassport(rec))
	}
	s.mu.Unlock()
	body, err := json.Marshal(map[string]any{
		"agent_id":   agentID,
		"passports":  rows,
		"request_id": req.RequestID,
	})
	if err != nil {
		return UpstreamResponse{}, fmt.Errorf("marshal list passports response: %w", err)
	}
	return UpstreamResponse{StatusCode: http.StatusOK, Body: body}, nil
}

func (s *StaticUpstreamClient) handleVerifyPassport(req UpstreamRequest) (UpstreamResponse, error) {
	var payload map[string]any
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		return jsonError(req.RequestID, "INVALID_ARGUMENT", "invalid json body", false, http.StatusBadRequest)
	}
	passportID, _ := payload["passport_id"].(string)
	agentID, _ := payload["agent_id"].(string)
	if strings.TrimSpace(passportID) == "" || strings.TrimSpace(agentID) == "" {
		return jsonError(req.RequestID, "INVALID_ARGUMENT", "passport_id and agent_id are required", false, http.StatusBadRequest)
	}
	s.mu.Lock()
	s.ensure()
	rec, ok := s.passports[passportID]
	s.mu.Unlock()
	verified := ok && rec.AgentID == agentID && rec.Status == "ACTIVE" && time.Now().UTC().Before(rec.ExpiresAt)
	reason := "passport verified"
	if !verified {
		reason = "passport missing, expired, revoked, or not bound to agent"
	}
	body, err := json.Marshal(map[string]any{
		"verified":     verified,
		"reason":       reason,
		"passport_id":  passportID,
		"agent_id":     agentID,
		"request_id":   req.RequestID,
		"executed":     false,
		"proof_status": "reference_only",
	})
	if err != nil {
		return UpstreamResponse{}, fmt.Errorf("marshal verify passport response: %w", err)
	}
	return UpstreamResponse{StatusCode: http.StatusOK, Body: body}, nil
}

func (s *StaticUpstreamClient) handleEvaluateScope(req UpstreamRequest) (UpstreamResponse, error) {
	var payload map[string]any
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		return jsonError(req.RequestID, "INVALID_ARGUMENT", "invalid json body", false, http.StatusBadRequest)
	}
	agentID, _ := payload["agent_id"].(string)
	actionClass, _ := payload["action_class"].(string)
	resource, _ := payload["resource"].(string)
	passportID, _ := payload["passport_id"].(string)
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(actionClass) == "" || strings.TrimSpace(resource) == "" {
		return jsonError(req.RequestID, "INVALID_ARGUMENT", "agent_id, action_class, and resource are required", false, http.StatusBadRequest)
	}
	allowed, reason := s.scopeAllows(agentID, passportID, actionClass, resource)
	effect := "DENY"
	if allowed {
		effect = "ALLOW"
	}
	body, err := json.Marshal(map[string]any{
		"decision": map[string]any{
			"effect":       effect,
			"reason":       reason,
			"agent_id":     agentID,
			"action_class": actionClass,
			"resource":     resource,
		},
		"executed": false,
	})
	if err != nil {
		return UpstreamResponse{}, fmt.Errorf("marshal scope evaluation response: %w", err)
	}
	return UpstreamResponse{StatusCode: http.StatusOK, Body: body}, nil
}

func (s *StaticUpstreamClient) scopeAllows(agentID, passportID, actionClass, resource string) (bool, string) {
	s.mu.Lock()
	s.ensure()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	check := func(rec passportRecord) bool {
		if rec.AgentID != agentID || rec.Status != "ACTIVE" || !now.Before(rec.ExpiresAt) {
			return false
		}
		for _, scope := range rec.ScopeSet {
			ac, _ := scope["action_class"].(string)
			rp, _ := scope["resource_pattern"].(string)
			if ac == actionClass && resourceMatches(rp, resource) {
				return true
			}
		}
		return false
	}
	if passportID != "" {
		rec, ok := s.passports[passportID]
		if !ok {
			return false, "passport not found"
		}
		if check(rec) {
			return true, "scope matched requested passport"
		}
		return false, "scope missing, expired, or revoked"
	}
	for _, id := range s.passportsByAgent[agentID] {
		if check(s.passports[id]) {
			return true, "scope matched active passport"
		}
	}
	return false, "no active passport scope matched"
}

func resourceMatches(pattern, resource string) bool {
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

func (s *StaticUpstreamClient) handleEvaluatePolicy(ctx context.Context, req UpstreamRequest) (UpstreamResponse, error) {
	var payload map[string]any
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		return jsonError(req.RequestID, "INVALID_ARGUMENT", "invalid json body", false, http.StatusBadRequest)
	}
	decisionID, _ := payload["decision_id"].(string)
	agentID, _ := payload["agent_id"].(string)
	actionClass, _ := payload["action_class"].(string)
	resource, _ := payload["resource"].(string)
	if strings.TrimSpace(decisionID) == "" || strings.TrimSpace(agentID) == "" || strings.TrimSpace(actionClass) == "" {
		return jsonError(req.RequestID, "INVALID_ARGUMENT", "decision_id, agent_id, and action_class are required", false, http.StatusBadRequest)
	}
	if strings.Contains(strings.ToLower(resource), "timeout") {
		return jsonError(req.RequestID, "DEPENDENCY_TIMEOUT", "policy engine timeout", true, http.StatusGatewayTimeout)
	}
	if s != nil && s.opaBaseURL != "" {
		effect, reason, err := evaluatePolicyOPA(ctx, s.opaBaseURL, s.opaHTTPClient(), decisionID, agentID, actionClass, resource)
		if err != nil {
			return jsonError(req.RequestID, "POLICY_ENGINE_UNAVAILABLE", err.Error(), true, http.StatusBadGateway)
		}
		return policyDecision(req.RequestID, decisionID, effect, reason)
	}
	effect := "ALLOW"
	reason := "policy-evaluated"
	if strings.Contains(strings.ToUpper(actionClass), "DENY") || actionClass == "demo.forbidden_action" {
		effect = "DENY"
		reason = "action class denied by foundation demo policy"
	}
	return policyDecision(req.RequestID, decisionID, effect, reason)
}

func policyDecision(requestID, decisionID, effect, reason string) (UpstreamResponse, error) {
	body, err := json.Marshal(map[string]any{
		"decision": map[string]any{
			"decision_id": decisionID,
			"effect":      strings.ToUpper(effect),
			"reason":      reason,
		},
		"request_id": requestID,
		"executed":   false,
	})
	if err != nil {
		return UpstreamResponse{}, fmt.Errorf("marshal evaluate policy response: %w", err)
	}
	return UpstreamResponse{StatusCode: http.StatusOK, Body: body}, nil
}

func publicPassport(rec passportRecord) map[string]any {
	return map[string]any{
		"passport_id":   rec.PassportID,
		"agent_id":      rec.AgentID,
		"passport_type": rec.PassportType,
		"scope_set":     rec.ScopeSet,
		"status":        rec.Status,
		"issued_by":     rec.IssuedBy,
		"expires_at":    rec.ExpiresAt.Format(time.RFC3339),
	}
}

func asInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	default:
		return 0, false
	}
}

func jsonError(requestID string, code string, message string, retryable bool, status int) (UpstreamResponse, error) {
	response := map[string]any{
		"error": map[string]any{
			"code":       code,
			"message":    message,
			"request_id": requestID,
			"retryable":  retryable,
		},
	}
	body, err := json.Marshal(response)
	if err != nil {
		return UpstreamResponse{}, fmt.Errorf("marshal error response: %w", err)
	}
	return UpstreamResponse{StatusCode: status, Body: body}, nil
}
