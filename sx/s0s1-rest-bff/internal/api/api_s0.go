package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"are-s0s1-rest-bff/internal/repo"
	"are-s0s1-rest-bff/internal/s0delegate"
	"google.golang.org/grpc/status"
)

func agentRecFromRegisterResponseBody(body []byte, fallbackType, fallbackOwner string) (repo.AgentRec, error) {
	var w struct {
		Agent struct {
			AgentType string         `json:"agent_type"`
			OwnerID   string         `json:"owner_id"`
			Status    string         `json:"status"`
			Metadata  map[string]any `json:"metadata"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(body, &w); err != nil {
		return repo.AgentRec{}, err
	}
	a := w.Agent
	if a.AgentType == "" {
		a.AgentType = fallbackType
	}
	if a.OwnerID == "" {
		a.OwnerID = fallbackOwner
	}
	st := strings.TrimSpace(a.Status)
	if st == "" {
		st = "ACTIVE"
	}
	if a.Metadata == nil {
		a.Metadata = map[string]any{}
	}
	return repo.AgentRec{AgentType: a.AgentType, OwnerID: a.OwnerID, Metadata: a.Metadata, Status: st}, nil
}

func writeS0DelegateError(w http.ResponseWriter, rid string, err error) {
	IncS0DelegateError(s0ErrorMetricLabel(err))
	h := s0delegate.UpstreamHTTP(err)
	retryable := s0delegate.UpstreamIsRetryable(err)
	msg := "upstream error"
	if st, ok := status.FromError(err); ok {
		if m := strings.TrimSpace(st.Message()); m != "" {
			msg = m
		}
	}
	var code string
	switch h {
	case http.StatusBadRequest:
		code = "INVALID_ARGUMENT"
	case http.StatusNotFound:
		code = "NOT_FOUND"
	case http.StatusConflict:
		code = "IDEMPOTENCY_CONFLICT"
	case http.StatusGatewayTimeout:
		code = "DEPENDENCY_TIMEOUT"
	case http.StatusUnauthorized, http.StatusForbidden:
		code = "UPSTREAM_ERROR"
	default:
		if h >= 500 {
			code = "POLICY_ENGINE_UNAVAILABLE"
		} else {
			code = "UPSTREAM_ERROR"
		}
	}
	jsonError(w, rid, code, msg, retryable, h)
}

func s0ErrorMetricLabel(err error) string {
	st, ok := status.FromError(err)
	if !ok {
		return "NON_GRPC"
	}
	return st.Code().String()
}

func existsAgentForPassport(ctx context.Context, cfg *Config, rp repo.Repository, agentID string) (bool, error) {
	if cfg != nil && cfg.S0 != nil && cfg.S0.Reg != nil {
		return s0delegate.AgentExists(ctx, cfg.S0.Reg, agentID)
	}
	_, ok, err := rp.GetAgent(ctx, agentID)
	return ok, err
}
