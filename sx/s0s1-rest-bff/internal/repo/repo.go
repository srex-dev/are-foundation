package repo

import (
	"context"
	"errors"
)

// AgentRec is persisted agent identity (metadata is JSON object).
type AgentRec struct {
	AgentType string
	OwnerID   string
	Metadata  map[string]any
	Status    string
	// AdmissionEnvelope optional immutable contract (demo BFF / REST parity with registry).
	AdmissionEnvelope map[string]any
}

var ErrAgentNotFound = errors.New("agent not found")

// Repository backs RegisterAgent / GetAgent / IssuePassport idempotency.
type Repository interface {
	GetRegisterIdem(ctx context.Context, idemKey string) ([]byte, bool, error)
	// FinishRegister stores agent + idempotent response; returns body to return (cached on races).
	FinishRegister(ctx context.Context, idemKey, agentID string, responseBody []byte, rec AgentRec) ([]byte, error)

	GetAgent(ctx context.Context, agentID string) (AgentRec, bool, error)

	// GetAdmissionEnvelope returns stored envelope when present.
	GetAdmissionEnvelope(ctx context.Context, agentID string) (map[string]any, bool, error)

	GetPassportIdem(ctx context.Context, idemKey string) ([]byte, bool, error)
	// FinishPassport stores passport idempotency; ErrAgentNotFound if agent_id is unknown.
	FinishPassport(ctx context.Context, idemKey, agentID string, responseBody []byte) ([]byte, error)
	ListPassportBodiesByAgent(ctx context.Context, agentID string) ([][]byte, error)
	GetPassportBody(ctx context.Context, passportID string) ([]byte, bool, error)
}
