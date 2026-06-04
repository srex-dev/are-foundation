package s0delegate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	registryv1 "github.com/srex-dev/are-foundation/s0/agent-registry-service/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Register calls S0 RegisterAgent and returns JSON body (201 shape) and canonical agent_id.
// Optional admission is sent as admission_constraints_json when non-empty.
func Register(
	ctx context.Context,
	c registryv1.AgentRegistryServiceClient,
	agentType, ownerID string,
	metadata map[string]any,
	admission map[string]any,
) ([]byte, string, error) {
	meta := mapStringString(metadata)
	req := &registryv1.RegisterAgentRequest{
		AgentType: agentType,
		OwnerId:   ownerID,
		Metadata:  meta,
	}
	if len(admission) > 0 {
		b, err := json.Marshal(admission)
		if err != nil {
			return nil, "", err
		}
		req.AdmissionConstraintsJson = b
	}
	resp, err := c.RegisterAgent(ctx, req)
	if err != nil {
		return nil, "", err
	}
	if resp.GetAgent() == nil {
		return nil, "", fmt.Errorf("s0delegate: empty agent in RegisterAgentResponse")
	}
	a := resp.GetAgent()
	body, err := json.Marshal(map[string]any{
		"agent": agentToMap(a),
	})
	if err != nil {
		return nil, "", err
	}
	return body, strings.TrimSpace(a.GetAgentId()), nil
}

// GetAgentJSON returns agent JSON for GET /v1/identity/agents/{id}.
func GetAgentJSON(ctx context.Context, c registryv1.AgentRegistryServiceClient, agentID string) ([]byte, error) {
	resp, err := c.GetAgent(ctx, &registryv1.GetAgentRequest{AgentId: agentID})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			return nil, errNotFound
		}
		return nil, err
	}
	if resp.GetAgent() == nil {
		return nil, errNotFound
	}
	return json.Marshal(map[string]any{
		"agent": agentToMap(resp.GetAgent()),
	})
}

// AgentExists returns whether the agent is known to the registry.
func AgentExists(ctx context.Context, c registryv1.AgentRegistryServiceClient, agentID string) (bool, error) {
	resp, err := c.GetAgent(ctx, &registryv1.GetAgentRequest{AgentId: agentID})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			return false, nil
		}
		return false, err
	}
	return resp.GetAgent() != nil, nil
}

func agentToMap(a *registryv1.Agent) map[string]any {
	if a == nil {
		return nil
	}
	m := map[string]any{
		"agent_id":   a.GetAgentId(),
		"agent_type": a.GetAgentType(),
		"owner_id":   a.GetOwnerId(),
		"status":     a.GetStatus(),
	}
	if a.GetMetadata() != nil {
		m["metadata"] = a.GetMetadata()
	}
	if ext := a.GetExternalId(); ext != "" {
		m["external_id"] = ext
	}
	if a.GetPassportId() != "" {
		m["passport_id"] = a.GetPassportId()
	}
	return m
}

// GetAdmissionEnvelopeJSON returns JSON for GET .../admission-envelope (200 shape).
func GetAdmissionEnvelopeJSON(ctx context.Context, c registryv1.AgentRegistryServiceClient, agentID string) ([]byte, error) {
	resp, err := c.GetAdmissionEnvelope(ctx, &registryv1.GetAdmissionEnvelopeRequest{AgentId: agentID})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			return nil, errNotFound
		}
		return nil, err
	}
	env := resp.GetEnvelope()
	if env == nil {
		return nil, errNotFound
	}
	body, err := json.Marshal(map[string]any{
		"agent_id":           agentID,
		"admission_envelope": admissionEnvelopeToMap(env),
	})
	if err != nil {
		return nil, err
	}
	return body, nil
}

func admissionEnvelopeToMap(e *registryv1.AdmissionEnvelope) map[string]any {
	if e == nil {
		return nil
	}
	m := map[string]any{
		"envelope_id":       e.GetEnvelopeId(),
		"agent_id":          e.GetAgentId(),
		"policy_id":         e.GetPolicyId(),
		"policy_ver":        e.GetPolicyVer(),
		"admitted_scopes":   e.GetAdmittedScopes(),
		"admitted_ts_ms":    e.GetAdmittedTsMs(),
		"issuing_authority": e.GetIssuingAuthority(),
	}
	if caps := e.GetAdmittedBehavioralCaps(); len(caps) > 0 {
		m["admitted_behavioral_caps"] = caps
	}
	if sig := e.GetSignature(); len(sig) > 0 {
		m["signature"] = string(sig)
	}
	return m
}

func mapStringString(in map[string]any) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		switch t := v.(type) {
		case string:
			out[k] = t
		case float64, bool, int, int64:
			out[k] = fmt.Sprint(t)
		case json.Number:
			out[k] = t.String()
		default:
			b, _ := json.Marshal(t)
			out[k] = string(b)
		}
	}
	return out
}

// errNotFound is returned from Get when S0 has no such agent.
var errNotFound = notFoundError{}

type notFoundError struct{}

func (notFoundError) Error() string { return "not found" }

// IsNotFound reports whether err is a delegate not-found.
func IsNotFound(err error) bool {
	_, ok := err.(notFoundError)
	if ok {
		return true
	}
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return true
	}
	return false
}
