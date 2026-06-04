package local

import (
	"bytes"

	"github.com/srex-dev/are-foundation/s0/passport-issuance-engine/internal/metrics"
	"github.com/srex-dev/are-foundation/s0/passport-issuance-engine/internal/service"
)

// RegistryStub reports all agents as ACTIVE for local / demo runs.
type RegistryStub struct{}

func (RegistryStub) GetAgentStatus(string) (service.AgentStatus, error) {
	return service.AgentActive, nil
}

// CredentialStub provides deterministic sign/verify (same pattern as unit tests).
type CredentialStub struct{}

func (CredentialStub) Sign(payload []byte) (string, string, []byte, error) {
	sig := append([]byte("sig:"), payload...)
	return "cred-local", "pub-local", sig, nil
}

func (CredentialStub) Verify(_ string, payload, signature []byte) (bool, error) {
	expected := append([]byte("sig:"), payload...)
	return bytes.Equal(expected, signature), nil
}

type outboxStub struct{}

func (outboxStub) Enqueue(_, _, _, _ string) error { return nil }

type ledgerStub struct{}

func (ledgerStub) WritePassportLifecycle(_, _, _, _ string) error { return nil }

type countingLedger struct {
	inner service.LedgerClient
}

func (c countingLedger) WritePassportLifecycle(agentID, passportID, eventType, reason string) error {
	err := c.inner.WritePassportLifecycle(agentID, passportID, eventType, reason)
	if err != nil {
		metrics.LedgerWriteFailureTotal.Inc()
	}
	return err
}

// BundleStub returns a fixed policy id/version when present.
type BundleStub struct {
	ID, Ver string
	Err     error
}

func (b BundleStub) ActivePolicyBundle() (string, string, error) {
	return b.ID, b.Ver, b.Err
}

// NewInMemoryService builds the passport service with local-friendly dependencies.
func NewInMemoryService() *service.Service {
	return service.New(
		RegistryStub{},
		CredentialStub{},
		countingLedger{inner: ledgerStub{}},
		outboxStub{},
		BundleStub{ID: "local-bundle", Ver: "1.0.0"},
	)
}
