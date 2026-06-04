package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/srex-dev/are-foundation/s0/agent-registry-service/internal/domain"
)

func TestRegisterAgentWithOutboxDuplicateReturnsAlreadyExists(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pool mock: %v", err)
	}
	defer mock.Close()

	repo := New(mock)
	a := domain.Agent{
		AgentID:        "a1",
		AgentType:      "AUTONOMOUS",
		OwnerID:        "00000000-0000-0000-0000-000000000001",
		Status:         domain.StatusPending,
		Metadata:       map[string]string{},
		RegistrationTS: time.Now().UTC(),
		UpdatedTS:      time.Now().UTC(),
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").WillReturnError(&pgconn.PgError{Code: "23505"})
	mock.ExpectRollback()

	_, err = repo.RegisterAgentWithOutbox(context.Background(), a, nil, "AGENT_REGISTERED", []byte("{}"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetAgentNotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pool mock: %v", err)
	}
	defer mock.Close()

	repo := New(mock)
	mock.ExpectQuery("SELECT agent_id, agent_type, owner_id").
		WithArgs("missing").
		WillReturnError(pgx.ErrNoRows)

	_, err = repo.GetAgent(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestGetAgentSuccess(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pool mock: %v", err)
	}
	defer mock.Close()
	repo := New(mock)
	now := time.Now().UTC()

	admission := []byte(`{}`)
	var nilPID, nilPver *string
	mock.ExpectQuery("SELECT agent_id, agent_type, owner_id").
		WithArgs("a1").
		WillReturnRows(pgxmock.NewRows([]string{"agent_id", "agent_type", "owner_id", "external_id", "status", "passport_id", "metadata", "registration_ts", "updated_ts", "admission_constraints", "admitted_policy_id", "admitted_policy_ver", "admission_ts"}).
			AddRow("a1", "AUTONOMOUS", "00000000-0000-0000-0000-000000000001", "ext", "ACTIVE", "", []byte(`{"k":"v"}`), now, now, admission, nilPID, nilPver, now))

	agent, err := repo.GetAgent(context.Background(), "a1")
	if err != nil {
		t.Fatalf("unexpected get error: %v", err)
	}
	if agent.AgentID != "a1" || agent.Metadata["k"] != "v" {
		t.Fatalf("unexpected agent: %+v", agent)
	}
}

func TestRegisterAgentWithOutboxSuccess(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pool mock: %v", err)
	}
	defer mock.Close()

	repo := New(mock)
	now := time.Now().UTC()
	a := domain.Agent{
		AgentID:              "a1",
		AgentType:            "AUTONOMOUS",
		OwnerID:              "00000000-0000-0000-0000-000000000001",
		Status:               domain.StatusPending,
		Metadata:             map[string]string{"k": "v"},
		RegistrationTS:       now,
		UpdatedTS:            now,
		AdmissionConstraints: map[string]any{},
		AdmissionTS:          now,
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO agent_status_history").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO agent_lifecycle_outbox").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	out, err := repo.RegisterAgentWithOutbox(context.Background(), a, nil, "AGENT_REGISTERED", []byte("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.AgentID != "a1" {
		t.Fatalf("unexpected agent result: %+v", out)
	}
}

func TestRegisterAgentWithOutboxWithEnvelope(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pool mock: %v", err)
	}
	defer mock.Close()

	repo := New(mock)
	now := time.Now().UTC()
	a := domain.Agent{
		AgentID:              "a1",
		AgentType:            "AUTONOMOUS",
		OwnerID:              "00000000-0000-0000-0000-000000000001",
		Status:               domain.StatusPending,
		Metadata:             map[string]string{},
		RegistrationTS:       now,
		UpdatedTS:            now,
		AdmissionConstraints: map[string]any{"max_latency_ms": 100.0},
		AdmissionTS:          now,
	}
	env := &domain.AdmissionEnvelope{
		EnvelopeID:             "env-1",
		AgentID:                "a1",
		PolicyID:               "pol-1",
		PolicyVer:              "v1",
		AdmittedScopes:         []string{"read:*"},
		AdmittedBehavioralCaps: map[string]float64{"max_latency_ms": 50},
		AdmittedTS:             now,
		IssuingAuthority:       "agent-registry",
		Signature:              []byte{0xab},
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO admission_envelopes").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO agent_status_history").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO agent_lifecycle_outbox").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	_, err = repo.RegisterAgentWithOutbox(context.Background(), a, env, "AGENT_REGISTERED", []byte("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAdmissionEnvelopeNotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pool mock: %v", err)
	}
	defer mock.Close()
	repo := New(mock)
	mock.ExpectQuery("SELECT envelope_id, agent_id, policy_id").
		WithArgs("missing").
		WillReturnError(pgx.ErrNoRows)
	_, err = repo.GetAdmissionEnvelope(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetAdmissionEnvelopeSuccess(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pool mock: %v", err)
	}
	defer mock.Close()
	repo := New(mock)
	now := time.Now().UTC()
	scopes := []byte(`["s1"]`)
	caps := []byte(`{"c":1.5}`)
	pid := "pol-1"
	pver := "v2"
	mock.ExpectQuery("SELECT envelope_id, agent_id, policy_id").
		WithArgs("a1").
		WillReturnRows(pgxmock.NewRows([]string{
			"envelope_id", "agent_id", "policy_id", "policy_ver",
			"admitted_scopes", "admitted_behavioral_caps", "admitted_ts", "issuing_authority", "signature",
		}).AddRow("e1", "a1", &pid, &pver, scopes, caps, now, "issuer", []byte{1}))

	env, err := repo.GetAdmissionEnvelope(context.Background(), "a1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if env.EnvelopeID != "e1" || env.PolicyID != "pol-1" || len(env.AdmittedScopes) != 1 || env.AdmittedScopes[0] != "s1" {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	if env.AdmittedBehavioralCaps["c"] != 1.5 {
		t.Fatalf("unexpected caps: %+v", env.AdmittedBehavioralCaps)
	}
}

func TestListAgentsDefaultFilterExcludesDeregisteredQueryPath(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pool mock: %v", err)
	}
	defer mock.Close()

	repo := New(mock)
	now := time.Now().UTC()
	admission := []byte(`{}`)
	var nilPID, nilPver *string
	rows := pgxmock.NewRows([]string{"agent_id", "agent_type", "owner_id", "external_id", "status", "passport_id", "metadata", "registration_ts", "updated_ts", "admission_constraints", "admitted_policy_id", "admitted_policy_ver", "admission_ts"}).
		AddRow("a1", "AUTONOMOUS", "00000000-0000-0000-0000-000000000001", "", "ACTIVE", "", []byte("{}"), now, now, admission, nilPID, nilPver, now)
	mock.ExpectQuery("SELECT agent_id, agent_type, owner_id").
		WithArgs(51).
		WillReturnRows(rows)

	items, _, count, err := repo.ListAgents(context.Background(), "", "", "", 50, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || count != 1 {
		t.Fatalf("unexpected list result")
	}
}

func TestUpdateAgentStatusInvalidTransition(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pool mock: %v", err)
	}
	defer mock.Close()
	repo := New(mock)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status FROM agents").WithArgs("a1").
		WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	mock.ExpectRollback()

	_, _, err = repo.UpdateAgentStatus(context.Background(), "a1", "PENDING", "reason", "tester")
	if err == nil {
		t.Fatal("expected failed precondition")
	}
}

func TestUpdateAgentStatusNoopIdempotent(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pool mock: %v", err)
	}
	defer mock.Close()
	repo := New(mock)
	now := time.Now().UTC()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status FROM agents").WithArgs("a1").
		WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	mock.ExpectCommit()
	admission := []byte(`{}`)
	var nilPID, nilPver *string
	mock.ExpectQuery("SELECT agent_id, agent_type, owner_id").WithArgs("a1").
		WillReturnRows(pgxmock.NewRows([]string{"agent_id", "agent_type", "owner_id", "external_id", "status", "passport_id", "metadata", "registration_ts", "updated_ts", "admission_constraints", "admitted_policy_id", "admitted_policy_ver", "admission_ts"}).
			AddRow("a1", "AUTONOMOUS", "00000000-0000-0000-0000-000000000001", "", "ACTIVE", "", []byte("{}"), now, now, admission, nilPID, nilPver, now))

	agent, prev, err := repo.UpdateAgentStatus(context.Background(), "a1", "ACTIVE", "same", "tester")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if prev != "ACTIVE" || agent.AgentID != "a1" {
		t.Fatalf("unexpected update result: prev=%s agent=%+v", prev, agent)
	}
}

func TestUpdateAgentStatusSuccessWritesHistoryAndOutbox(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pool mock: %v", err)
	}
	defer mock.Close()
	repo := New(mock)
	now := time.Now().UTC()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status FROM agents").WithArgs("a1").
		WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	mock.ExpectExec("UPDATE agents SET status=").WithArgs("SUSPENDED", pgxmock.AnyArg(), "a1").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("INSERT INTO agent_status_history").
		WithArgs("a1", "ACTIVE", "SUSPENDED", "reason", "tester").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO agent_lifecycle_outbox").
		WithArgs("a1", "AGENT_STATUS_CHANGED", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	admission2 := []byte(`{}`)
	var nilPID2, nilPver2 *string
	mock.ExpectQuery("SELECT agent_id, agent_type, owner_id").WithArgs("a1").
		WillReturnRows(pgxmock.NewRows([]string{"agent_id", "agent_type", "owner_id", "external_id", "status", "passport_id", "metadata", "registration_ts", "updated_ts", "admission_constraints", "admitted_policy_id", "admitted_policy_ver", "admission_ts"}).
			AddRow("a1", "AUTONOMOUS", "00000000-0000-0000-0000-000000000001", "", "SUSPENDED", "", []byte("{}"), now, now, admission2, nilPID2, nilPver2, now))

	agent, prev, err := repo.UpdateAgentStatus(context.Background(), "a1", "SUSPENDED", "reason", "tester")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if prev != "ACTIVE" || agent.Status != "SUSPENDED" {
		t.Fatalf("unexpected update result: prev=%s agent=%+v", prev, agent)
	}
}
