package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/domain"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/registryerr"
)

// Repository defines required persistence behavior.
type Repository interface {
	RegisterAgentWithOutbox(ctx context.Context, a domain.Agent, envelope *domain.AdmissionEnvelope, eventType string, payload []byte) (domain.Agent, error)
	GetAgent(ctx context.Context, agentID string) (domain.Agent, error)
	GetAdmissionEnvelope(ctx context.Context, agentID string) (*domain.AdmissionEnvelope, error)
	ListAgents(ctx context.Context, status, agentType, ownerID string, limit int, token string) ([]domain.Agent, string, int, error)
	UpdateAgentStatus(ctx context.Context, agentID, newStatus, reason, changedBy string) (domain.Agent, string, error)
}

// Registry implements component business rules.
type Registry struct {
	repo Repository
}

// New returns a new registry service.
func New(repo Repository) *Registry {
	return &Registry{repo: repo}
}

// Register creates a new agent and writes an outbox event in one DB transaction.
func (r *Registry) Register(ctx context.Context, agentType, ownerID, externalID string, metadata map[string]string) (domain.Agent, error) {
	return r.RegisterWithAdmission(ctx, agentType, ownerID, externalID, metadata, nil)
}

// RegisterWithAdmission creates a new agent with an optional admission snapshot (immutable after write).
func (r *Registry) RegisterWithAdmission(ctx context.Context, agentType, ownerID, externalID string, metadata map[string]string, admission *domain.AdmissionSnapshot) (domain.Agent, error) {
	validType, err := domain.ValidateAgentType(agentType)
	if err != nil {
		return domain.Agent{}, fmt.Errorf("%w: %v", registryerr.ErrInvalidArgument, err)
	}
	if ownerID == "" {
		return domain.Agent{}, fmt.Errorf("%w: owner_id is required", registryerr.ErrInvalidArgument)
	}
	if err := domain.ValidateAgentMetadata(metadata); err != nil {
		return domain.Agent{}, fmt.Errorf("%w: %v", registryerr.ErrInvalidArgument, err)
	}

	now := time.Now().UTC()
	agent := domain.Agent{
		AgentID:        uuid.NewString(),
		AgentType:      validType,
		OwnerID:        ownerID,
		ExternalID:     externalID,
		Status:         domain.StatusPending,
		Metadata:       metadata,
		RegistrationTS: now,
		UpdatedTS:      now,
		AdmissionTS:    now,
	}
	if admission != nil {
		if admission.Constraints != nil {
			agent.AdmissionConstraints = admission.Constraints
		} else {
			agent.AdmissionConstraints = map[string]any{}
		}
		agent.AdmittedPolicyID = admission.PolicyID
		agent.AdmittedPolicyVer = admission.PolicyVer
	} else {
		agent.AdmissionConstraints = map[string]any{}
	}
	var env *domain.AdmissionEnvelope
	if admission != nil {
		eid := admission.EnvelopeID
		if eid == "" {
			eid = uuid.NewString()
		}
		sig := admission.Signature
		if sig == nil {
			sig = []byte{}
		}
		env = domain.NewAdmissionEnvelope(eid, agent.AgentID, now, admission, sig, admission.IssuingAuthority)
	}
	payload, err := json.Marshal(map[string]any{
		"event_id":        uuid.NewString(),
		"event_type":      "AGENT_REGISTERED",
		"agent_id":        agent.AgentID,
		"agent_type":      agent.AgentType,
		"owner_id":        agent.OwnerID,
		"previous_status": nil,
		"new_status":      agent.Status,
		"reason":          "initial registration",
		"emitted_ts":      now.UnixMilli(),
		"schema_version":  "1.0.0",
	})
	if err != nil {
		return domain.Agent{}, fmt.Errorf("marshal event payload: %w", err)
	}
	return r.repo.RegisterAgentWithOutbox(ctx, agent, env, "AGENT_REGISTERED", payload)
}

// GetAdmissionEnvelope returns the immutable admission envelope for an agent when present.
func (r *Registry) GetAdmissionEnvelope(ctx context.Context, agentID string) (*domain.AdmissionEnvelope, error) {
	if agentID == "" {
		return nil, fmt.Errorf("%w: agent_id is required", registryerr.ErrInvalidArgument)
	}
	return r.repo.GetAdmissionEnvelope(ctx, agentID)
}

// Get fetches one agent by id.
func (r *Registry) Get(ctx context.Context, agentID string) (domain.Agent, error) {
	return r.repo.GetAgent(ctx, agentID)
}

// List returns filtered paginated agents.
func (r *Registry) List(ctx context.Context, status, agentType, ownerID string, limit int, token string) ([]domain.Agent, string, int, error) {
	return r.repo.ListAgents(ctx, status, agentType, ownerID, limit, token)
}

// UpdateStatus applies lifecycle transitions and emits outbox status-change semantics in DB.
func (r *Registry) UpdateStatus(ctx context.Context, agentID, newStatus, reason, changedBy string) (domain.Agent, error) {
	newStatus = strings.ToUpper(strings.TrimSpace(newStatus))
	if agentID == "" || newStatus == "" || reason == "" || changedBy == "" {
		return domain.Agent{}, fmt.Errorf("%w: agent_id, new_status, reason, updated_by are required", registryerr.ErrInvalidArgument)
	}
	agent, prev, err := r.repo.UpdateAgentStatus(ctx, agentID, newStatus, reason, changedBy)
	if err != nil {
		return domain.Agent{}, err
	}
	slog.Info("agent_status_changed", "agent_id", agentID, "previous_status", prev, "new_status", newStatus, "changed_by", changedBy)
	if agent.Status != newStatus {
		return domain.Agent{}, fmt.Errorf("%w: agent status after update is %q, expected %q (previous_status=%q)",
			registryerr.ErrFailedPrecondition, agent.Status, newStatus, prev)
	}
	return agent, nil
}
