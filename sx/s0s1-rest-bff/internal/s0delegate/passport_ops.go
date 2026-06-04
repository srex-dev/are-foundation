package s0delegate

import (
	"context"
	"encoding/json"
	"fmt"

	passportv1 "github.com/srex-dev/are-foundation/s0/passport-issuance-engine/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IssuePassportJSON calls S0 IssuePassport and returns JSON (201 body shape for BFF).
func IssuePassportJSON(ctx context.Context, c passportv1.PassportIssuanceServiceClient, req *passportv1.IssuePassportRequest) ([]byte, error) {
	if req == nil {
		return nil, fmt.Errorf("s0delegate: nil IssuePassportRequest")
	}
	resp, err := c.IssuePassport(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.GetPassport() == nil {
		return nil, fmt.Errorf("s0delegate: empty passport in IssuePassportResponse")
	}
	return json.Marshal(map[string]any{
		"passport": passportToMap(resp.GetPassport()),
	})
}

func passportToMap(p *passportv1.Passport) map[string]any {
	if p == nil {
		return nil
	}
	out := map[string]any{
		"passport_id":            p.GetPassportId(),
		"agent_id":               p.GetAgentId(),
		"passport_type":          p.GetPassportType(),
		"status":                 p.GetStatus(),
		"issued_by":              p.GetIssuedBy(),
		"issued_ts":              p.GetIssuedTs(),
		"expires_ts":             p.GetExpiresTs(),
		"credential_id":          p.GetCredentialId(),
		"public_key_pem":         p.GetPublicKeyPem(),
		"policy_id_at_issuance":  p.GetPolicyIdAtIssuance(),
		"policy_ver_at_issuance": p.GetPolicyVerAtIssuance(),
	}
	if len(p.GetSignature()) > 0 {
		out["signature"] = string(p.GetSignature())
	}
	if rs := p.GetRevocationReason(); rs != "" {
		out["revocation_reason"] = rs
	}
	if sb := p.GetSupersededBy(); sb != "" {
		out["superseded_by"] = sb
	}
	if p.GetRevokedTs() != 0 {
		out["revoked_ts"] = p.GetRevokedTs()
	}
	scopes := p.GetScopeSet()
	if len(scopes) == 0 {
		out["scope_set"] = []any{}
	} else {
		arr := make([]any, 0, len(scopes))
		for _, s := range scopes {
			if s == nil {
				continue
			}
			row := map[string]any{
				"scope_id":         s.GetScopeId(),
				"action_class":     s.GetActionClass(),
				"resource_pattern": s.GetResourcePattern(),
			}
			if cc := s.GetContextConstraints(); len(cc) > 0 {
				row["context_constraints"] = cc
			}
			if s.GetGrantedTs() != 0 {
				row["granted_ts"] = s.GetGrantedTs()
			}
			if s.GetExpiresTs() != 0 {
				row["expires_ts"] = s.GetExpiresTs()
			}
			if s.GetIsEscalation() {
				row["is_escalation"] = true
			}
			arr = append(arr, row)
		}
		out["scope_set"] = arr
	}
	return out
}

// BuildIssueRequest maps BFF JSON payload to proto (after validation in handler).
func BuildIssueRequest(
	agentID, passportType, issuedBy, reason string,
	ttl int64,
	scopes []map[string]any,
	forceReissue bool,
) *passportv1.IssuePassportRequest {
	req := &passportv1.IssuePassportRequest{
		AgentId:         agentID,
		PassportType:    passportType,
		TtlSeconds:      ttl,
		IssuedBy:        issuedBy,
		Reason:          reason,
		ForceReissue:    forceReissue,
		RequestedScopes: make([]*passportv1.ScopeRequest, 0, len(scopes)),
	}
	for _, row := range scopes {
		if row == nil {
			continue
		}
		ac, _ := row["action_class"].(string)
		rp, _ := row["resource_pattern"].(string)
		sr := &passportv1.ScopeRequest{
			ActionClass:     ac,
			ResourcePattern: rp,
		}
		// optional scope_ttl_seconds
		if n, ok := asInt64Any(row["scope_ttl_seconds"]); ok {
			sr.ScopeTtlSeconds = n
		}
		// context_constraints map[string]any -> map[string]string
		if raw, ok := row["context_constraints"].(map[string]any); ok && len(raw) > 0 {
			cc := make(map[string]string, len(raw))
			for k, v := range raw {
				cc[k] = fmt.Sprint(v)
			}
			sr.ContextConstraints = cc
		}
		req.RequestedScopes = append(req.RequestedScopes, sr)
	}
	return req
}

func asInt64Any(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case int64:
		return x, true
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	default:
		return 0, false
	}
}

// IsPassportNotFound is true for missing passport (if server uses NotFound for issue edge cases).
func IsPassportNotFound(err error) bool {
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return true
	}
	return false
}
