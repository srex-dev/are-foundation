package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/domain"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/registryerr"
)

type fakeRepo struct {
	registerFn func(ctx context.Context, a domain.Agent, envelope *domain.AdmissionEnvelope, eventType string, payload []byte) (domain.Agent, error)
	getFn      func(ctx context.Context, agentID string) (domain.Agent, error)
	getEnvFn   func(ctx context.Context, agentID string) (*domain.AdmissionEnvelope, error)
	listFn     func(ctx context.Context, status, agentType, ownerID string, limit int, token string) ([]domain.Agent, string, int, error)
	updateFn   func(ctx context.Context, agentID, newStatus, reason, changedBy string) (domain.Agent, string, error)
}

func (f *fakeRepo) RegisterAgentWithOutbox(_ context.Context, a domain.Agent, envelope *domain.AdmissionEnvelope, _ string, _ []byte) (domain.Agent, error) {
	if f.registerFn != nil {
		return f.registerFn(context.Background(), a, envelope, "AGENT_REGISTERED", nil)
	}
	return a, nil
}
func (f *fakeRepo) GetAdmissionEnvelope(_ context.Context, agentID string) (*domain.AdmissionEnvelope, error) {
	if f.getEnvFn != nil {
		return f.getEnvFn(context.Background(), agentID)
	}
	return nil, nil
}
func (f *fakeRepo) GetAgent(_ context.Context, _ string) (domain.Agent, error) {
	if f.getFn != nil {
		return f.getFn(context.Background(), "")
	}
	return domain.Agent{}, nil
}
func (f *fakeRepo) ListAgents(_ context.Context, _, _, _ string, _ int, _ string) ([]domain.Agent, string, int, error) {
	if f.listFn != nil {
		return f.listFn(context.Background(), "", "", "", 0, "")
	}
	return nil, "", 0, nil
}
func (f *fakeRepo) UpdateAgentStatus(_ context.Context, _ string, newStatus, _, _ string) (domain.Agent, string, error) {
	if f.updateFn != nil {
		return f.updateFn(context.Background(), "", newStatus, "", "")
	}
	return domain.Agent{Status: newStatus}, "ACTIVE", nil
}

func TestRegisterRequiresOwnerID(t *testing.T) {
	svc := New(&fakeRepo{})
	_, err := svc.Register(context.Background(), "AUTONOMOUS", "", "", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, registryerr.ErrInvalidArgument) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegisterInvalidType(t *testing.T) {
	svc := New(&fakeRepo{})
	_, err := svc.Register(context.Background(), "BADTYPE", "owner-1", "", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, registryerr.ErrInvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT error, got: %v", err)
	}
}

func TestRegisterSuccessSetsPendingAndTimestamps(t *testing.T) {
	svc := New(&fakeRepo{})
	agent, err := svc.Register(context.Background(), "autonomous", "owner-1", "ext-1", map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.AgentID == "" {
		t.Fatal("expected generated agent id")
	}
	if agent.Status != domain.StatusPending {
		t.Fatalf("expected PENDING status, got %s", agent.Status)
	}
	if agent.RegistrationTS.IsZero() || agent.UpdatedTS.IsZero() {
		t.Fatal("expected non-zero timestamps")
	}
	if !agent.RegistrationTS.Equal(agent.UpdatedTS) {
		t.Fatal("expected registration and updated timestamp equal on create")
	}
}

func TestRegisterWithAdmissionPersistsSnapshot(t *testing.T) {
	var got domain.Agent
	var gotEnv *domain.AdmissionEnvelope
	repo := &fakeRepo{
		registerFn: func(_ context.Context, a domain.Agent, env *domain.AdmissionEnvelope, _ string, _ []byte) (domain.Agent, error) {
			got = a
			gotEnv = env
			return a, nil
		},
	}
	svc := New(repo)
	admission := &domain.AdmissionSnapshot{
		Constraints: map[string]any{"max_scope_depth": 2},
		PolicyID:    "pol-1",
		PolicyVer:   "v9",
	}
	agent, err := svc.RegisterWithAdmission(context.Background(), "AUTONOMOUS", "00000000-0000-0000-0000-000000000099", "ext-adm", map[string]string{}, admission)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AdmittedPolicyID != "pol-1" || got.AdmittedPolicyVer != "v9" {
		t.Fatalf("expected policy id/ver on agent, got %+v", got)
	}
	if got.AdmissionConstraints["max_scope_depth"] != 2 {
		t.Fatalf("expected admission constraints, got %+v", got.AdmissionConstraints)
	}
	if agent.AgentID == "" {
		t.Fatal("expected agent id")
	}
	if gotEnv == nil || gotEnv.AgentID != agent.AgentID || gotEnv.PolicyID != "pol-1" {
		t.Fatalf("expected admission envelope persisted, got %+v", gotEnv)
	}
}

func TestGetAdmissionEnvelopeRequiresAgentID(t *testing.T) {
	svc := New(&fakeRepo{})
	_, err := svc.GetAdmissionEnvelope(context.Background(), "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, registryerr.ErrInvalidArgument) {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestGetAndListPassthrough(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepo{
		getFn: func(_ context.Context, _ string) (domain.Agent, error) {
			return domain.Agent{AgentID: "a1", RegistrationTS: now}, nil
		},
		listFn: func(_ context.Context, _, _, _ string, _ int, _ string) ([]domain.Agent, string, int, error) {
			return []domain.Agent{{AgentID: "a1"}}, "next", 1, nil
		},
	}
	svc := New(repo)
	agent, err := svc.Get(context.Background(), "a1")
	if err != nil || agent.AgentID != "a1" {
		t.Fatalf("unexpected get result: %+v err=%v", agent, err)
	}
	items, token, count, err := svc.List(context.Background(), "", "", "", 10, "")
	if err != nil {
		t.Fatalf("unexpected list error: %v", err)
	}
	if len(items) != 1 || token != "next" || count != 1 {
		t.Fatalf("unexpected list result len=%d token=%s count=%d", len(items), token, count)
	}
}

func TestUpdateStatusInvalidTransition(t *testing.T) {
	svc := New(&fakeRepo{
		updateFn: func(_ context.Context, _, _, _, _ string) (domain.Agent, string, error) {
			return domain.Agent{}, "ACTIVE", errors.New("FAILED_PRECONDITION: invalid transition from ACTIVE to PENDING")
		},
	})
	_, err := svc.UpdateStatus(context.Background(), "a", "PENDING", "reason", "user")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "FAILED_PRECONDITION") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateStatusRequiredArgs(t *testing.T) {
	svc := New(&fakeRepo{})
	_, err := svc.UpdateStatus(context.Background(), "", "ACTIVE", "", "u")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, registryerr.ErrInvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT error, got %v", err)
	}
}

func TestUpdateStatusUppercasesStatus(t *testing.T) {
	var got string
	repo := &fakeRepo{
		updateFn: func(_ context.Context, _, newStatus, _, _ string) (domain.Agent, string, error) {
			got = newStatus
			return domain.Agent{Status: newStatus}, domain.StatusPending, nil
		},
	}
	svc := New(repo)
	_, err := svc.UpdateStatus(context.Background(), "a1", "active", "reason", "tester")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ACTIVE" {
		t.Fatalf("expected uppercased status, got %s", got)
	}
}

func TestUpdateStatusRepoErrorPassThrough(t *testing.T) {
	repo := &fakeRepo{
		updateFn: func(_ context.Context, _, _, _, _ string) (domain.Agent, string, error) {
			return domain.Agent{}, "", errors.New("FAILED_PRECONDITION: invalid transition")
		},
	}
	svc := New(repo)
	_, err := svc.UpdateStatus(context.Background(), "a1", "ACTIVE", "reason", "tester")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "FAILED_PRECONDITION") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateStatusPostUpdateMismatch(t *testing.T) {
	repo := &fakeRepo{
		updateFn: func(_ context.Context, _, newStatus, _, _ string) (domain.Agent, string, error) {
			return domain.Agent{Status: "CORRUPT"}, "ACTIVE", nil
		},
	}
	svc := New(repo)
	_, err := svc.UpdateStatus(context.Background(), "a1", "ACTIVE", "reason", "tester")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, registryerr.ErrFailedPrecondition) {
		t.Fatalf("expected FAILED_PRECONDITION, got %v", err)
	}
	if !strings.Contains(err.Error(), "previous_status=") {
		t.Fatalf("expected previous status in error: %v", err)
	}
}
