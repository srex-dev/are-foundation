package service

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/srex-dev/are-foundation/s0/passport-issuance-engine/internal/domain"
)

type registryMock struct {
	status AgentStatus
	err    error
}

func (m registryMock) GetAgentStatus(_ string) (AgentStatus, error) { return m.status, m.err }

type credentialMock struct {
	failSign bool
}

func (m credentialMock) Sign(payload []byte) (string, string, []byte, error) {
	if m.failSign {
		return "", "", nil, errors.New("down")
	}
	sig := append([]byte("sig:"), payload...)
	return "cred-1", "pub", sig, nil
}

func (m credentialMock) Verify(_ string, payload, signature []byte) (bool, error) {
	expected := append([]byte("sig:"), payload...)
	return bytes.Equal(expected, signature), nil
}

type outboxMock struct{}

func (outboxMock) Enqueue(_ string, _ string, _ string, _ string) error { return nil }

type ledgerMock struct{}

func (ledgerMock) WritePassportLifecycle(_ string, _ string, _ string, _ string) error { return nil }

type bundleMock struct {
	id, ver string
	err     error
}

func (m bundleMock) ActivePolicyBundle() (string, string, error) {
	return m.id, m.ver, m.err
}

func scope(action string) domain.Scope {
	return domain.Scope{
		ScopeID:         "scope-1",
		ActionClass:     action,
		ResourcePattern: "*",
		GrantedTS:       time.Now().UTC(),
		ExpiresTS:       time.Now().UTC().Add(time.Hour),
	}
}

func TestIssuePassportSetsPolicyVersionFromBundleClient(t *testing.T) {
	svc := New(registryMock{status: AgentActive}, credentialMock{}, ledgerMock{}, outboxMock{}, bundleMock{id: "bundle-a", ver: "1.2.3"})
	passport, err := svc.IssuePassport(IssueInput{
		AgentID:      "agent-1",
		PassportType: domain.TypeStandard,
		Scopes:       []domain.Scope{scope("READ")},
		TTLSeconds:   3600,
		IssuedBy:     "authority",
		Reason:       "initial",
	})
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	if passport.PolicyIDAtIssuance != "bundle-a" || passport.PolicyVerAtIssuance != "1.2.3" {
		t.Fatalf("expected policy at issuance, got id=%q ver=%q", passport.PolicyIDAtIssuance, passport.PolicyVerAtIssuance)
	}
}

func TestIssuePassportExplicitPolicyAtIssuanceOverridesBundle(t *testing.T) {
	svc := New(registryMock{status: AgentActive}, credentialMock{}, ledgerMock{}, outboxMock{}, bundleMock{id: "ignored", ver: "ignored"})
	passport, err := svc.IssuePassport(IssueInput{
		AgentID:             "agent-1",
		PassportType:        domain.TypeStandard,
		Scopes:              []domain.Scope{scope("READ")},
		TTLSeconds:          3600,
		IssuedBy:            "authority",
		Reason:              "initial",
		PolicyIDAtIssuance:  "explicit-id",
		PolicyVerAtIssuance: "explicit-ver",
	})
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	if passport.PolicyIDAtIssuance != "explicit-id" || passport.PolicyVerAtIssuance != "explicit-ver" {
		t.Fatalf("expected explicit policy, got id=%q ver=%q", passport.PolicyIDAtIssuance, passport.PolicyVerAtIssuance)
	}
}

func TestIssuePassportAndVerify(t *testing.T) {
	svc := New(registryMock{status: AgentActive}, credentialMock{}, ledgerMock{}, outboxMock{})
	passport, err := svc.IssuePassport(IssueInput{
		AgentID:      "agent-1",
		PassportType: domain.TypeStandard,
		Scopes:       []domain.Scope{scope("WRITE")},
		TTLSeconds:   3600,
		IssuedBy:     "authority",
		Reason:       "initial",
	})
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	ok, status, reason, err := svc.VerifyPassport(passport.PassportID, []byte("tampered"), passport.Signature)
	if err != nil {
		t.Fatalf("verify error: %v", err)
	}
	if ok {
		t.Fatalf("expected invalid signature")
	}
	if status != domain.StatusActive || reason != "signature_invalid" {
		t.Fatalf("unexpected verify result status=%s reason=%s", status, reason)
	}
}

func TestOneActivePassportPerAgent(t *testing.T) {
	svc := New(registryMock{status: AgentActive}, credentialMock{}, ledgerMock{}, outboxMock{})
	_, err := svc.IssuePassport(IssueInput{
		AgentID:      "agent-1",
		PassportType: domain.TypeStandard,
		Scopes:       []domain.Scope{scope("WRITE")},
		TTLSeconds:   3600,
		IssuedBy:     "authority",
		Reason:       "initial",
	})
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	_, err = svc.IssuePassport(IssueInput{
		AgentID:      "agent-1",
		PassportType: domain.TypeStandard,
		Scopes:       []domain.Scope{scope("WRITE")},
		TTLSeconds:   3600,
		IssuedBy:     "authority",
		Reason:       "duplicate",
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected already exists, got %v", err)
	}
}

func TestForceReissueRevokesOld(t *testing.T) {
	svc := New(registryMock{status: AgentActive}, credentialMock{}, ledgerMock{}, outboxMock{})
	first, _ := svc.IssuePassport(IssueInput{
		AgentID:      "agent-1",
		PassportType: domain.TypeStandard,
		Scopes:       []domain.Scope{scope("WRITE")},
		TTLSeconds:   3600,
		IssuedBy:     "authority",
		Reason:       "initial",
	})
	second, err := svc.IssuePassport(IssueInput{
		AgentID:      "agent-1",
		PassportType: domain.TypeStandard,
		Scopes:       []domain.Scope{scope("WRITE")},
		TTLSeconds:   3600,
		IssuedBy:     "authority",
		Reason:       "force",
		ForceReissue: true,
	})
	if err != nil {
		t.Fatalf("force reissue failed: %v", err)
	}
	old, _ := svc.GetPassport(first.PassportID)
	if old.Status != domain.StatusRevoked {
		t.Fatalf("expected old revoked, got %s", old.Status)
	}
	if second.PassportID == first.PassportID {
		t.Fatalf("expected new passport id")
	}
}

func TestProvisionalAllowlist(t *testing.T) {
	svc := New(registryMock{status: AgentPending}, credentialMock{}, ledgerMock{}, outboxMock{})
	_, err := svc.IssuePassport(IssueInput{
		AgentID:      "agent-1",
		PassportType: domain.TypeProvisional,
		Scopes:       []domain.Scope{scope("WRITE")},
		TTLSeconds:   3600,
		IssuedBy:     "authority",
		Reason:       "provisional",
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected invalid argument for provisional write scope, got %v", err)
	}
}

func TestRegistryStatusChecks(t *testing.T) {
	svc := New(registryMock{status: AgentDeregistered}, credentialMock{}, ledgerMock{}, outboxMock{})
	_, err := svc.IssuePassport(IssueInput{
		AgentID:      "agent-1",
		PassportType: domain.TypeStandard,
		Scopes:       []domain.Scope{scope("READ")},
		TTLSeconds:   3600,
		IssuedBy:     "authority",
		Reason:       "initial",
	})
	if !errors.Is(err, ErrFailedPrecondition) {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}

func TestSignFailurePreventsPersistence(t *testing.T) {
	svc := New(registryMock{status: AgentActive}, credentialMock{failSign: true}, ledgerMock{}, outboxMock{})
	_, err := svc.IssuePassport(IssueInput{
		AgentID:      "agent-1",
		PassportType: domain.TypeStandard,
		Scopes:       []domain.Scope{scope("READ")},
		TTLSeconds:   3600,
		IssuedBy:     "authority",
		Reason:       "initial",
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected unavailable, got %v", err)
	}
	if _, err := svc.GetAgentPassport("agent-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected no active passport persisted")
	}
}

func TestIssuePassportTTLLimits(t *testing.T) {
	svc := New(registryMock{status: AgentActive}, credentialMock{}, ledgerMock{}, outboxMock{})
	_, err := svc.IssuePassport(IssueInput{
		AgentID:      "agent-1",
		PassportType: domain.TypeStandard,
		Scopes:       []domain.Scope{scope("READ")},
		TTLSeconds:   100,
		IssuedBy:     "authority",
		Reason:       "too low",
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected invalid argument for low ttl, got %v", err)
	}
}

func TestIssuePassportRegistryUnavailable(t *testing.T) {
	svc := New(registryMock{err: errors.New("down")}, credentialMock{}, ledgerMock{}, outboxMock{})
	_, err := svc.IssuePassport(IssueInput{
		AgentID:      "agent-1",
		PassportType: domain.TypeStandard,
		Scopes:       []domain.Scope{scope("READ")},
		TTLSeconds:   3600,
		IssuedBy:     "authority",
		Reason:       "initial",
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected unavailable, got %v", err)
	}
}

func TestVerifyPassportRevokedAndExpired(t *testing.T) {
	svc := New(registryMock{status: AgentActive}, credentialMock{}, ledgerMock{}, outboxMock{})
	passport, _ := svc.IssuePassport(IssueInput{
		AgentID:      "agent-1",
		PassportType: domain.TypeStandard,
		Scopes:       []domain.Scope{scope("READ")},
		TTLSeconds:   3600,
		IssuedBy:     "authority",
		Reason:       "initial",
	})

	// Revoke via force reissue and verify first passport reports revoked.
	_, _ = svc.IssuePassport(IssueInput{
		AgentID:      "agent-1",
		PassportType: domain.TypeStandard,
		Scopes:       []domain.Scope{scope("READ")},
		TTLSeconds:   3600,
		IssuedBy:     "authority",
		Reason:       "force",
		ForceReissue: true,
	})
	ok, status, reason, err := svc.VerifyPassport(passport.PassportID, []byte("x"), []byte("y"))
	if err != nil {
		t.Fatalf("verify error: %v", err)
	}
	if ok || status != domain.StatusRevoked || reason != "revoked" {
		t.Fatalf("expected revoked result")
	}

	// Directly inject an expired passport to validate expired branch.
	expired := passport
	expired.PassportID = "expired-1"
	expired.Status = domain.StatusActive
	expired.ExpiresTS = time.Now().UTC().Add(-time.Minute)
	svc.mu.Lock()
	svc.passports[expired.PassportID] = expired
	svc.mu.Unlock()
	ok, status, reason, err = svc.VerifyPassport(expired.PassportID, []byte("x"), []byte("y"))
	if err != nil {
		t.Fatalf("verify error: %v", err)
	}
	if ok || status != domain.StatusExpired || reason != "expired" {
		t.Fatalf("expected expired result")
	}
}

func TestRevokePassportClearsActive(t *testing.T) {
	svc := New(registryMock{status: AgentActive}, credentialMock{}, ledgerMock{}, outboxMock{})
	p, err := svc.IssuePassport(IssueInput{
		AgentID:      "agent-1",
		PassportType: domain.TypeStandard,
		Scopes:       []domain.Scope{scope("READ")},
		TTLSeconds:   3600,
		IssuedBy:     "authority",
		Reason:       "initial",
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	got, err := svc.RevokePassport(p.PassportID, "ops", "admin")
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if got.Status != domain.StatusRevoked {
		t.Fatalf("expected revoked status")
	}
	_, err = svc.GetAgentPassport("agent-1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected no active passport, got %v", err)
	}
	_, err = svc.RevokePassport("", "r", "a")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected invalid argument")
	}
}
