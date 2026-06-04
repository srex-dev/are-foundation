package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/srex-dev/are-foundation/s0/passport-issuance-engine/internal/domain"
)

var (
	// ErrNotFound indicates a missing record.
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists indicates an active passport already exists.
	ErrAlreadyExists = errors.New("already exists")
	// ErrInvalidArgument indicates input validation failed.
	ErrInvalidArgument = errors.New("invalid argument")
	// ErrFailedPrecondition indicates state transition is not allowed.
	ErrFailedPrecondition = errors.New("failed precondition")
	// ErrUnavailable indicates a required dependency is unavailable.
	ErrUnavailable = errors.New("unavailable")
)

// AgentStatus is the status returned by Agent Registry dependency.
type AgentStatus string

const (
	AgentPending      AgentStatus = "PENDING"
	AgentActive       AgentStatus = "ACTIVE"
	AgentSuspended    AgentStatus = "SUSPENDED"
	AgentDeregistered AgentStatus = "DEREGISTERED"
)

// AgentRegistryClient represents required registry calls.
type AgentRegistryClient interface {
	GetAgentStatus(agentID string) (AgentStatus, error)
}

// PolicyBundleRegistryClient returns the active policy bundle identity at issuance time.
type PolicyBundleRegistryClient interface {
	ActivePolicyBundle() (policyID, policyVer string, err error)
}

// CredentialStoreClient represents required signing calls.
type CredentialStoreClient interface {
	Sign(payload []byte) (credentialID, publicKey string, signature []byte, err error)
	Verify(publicKey string, payload, signature []byte) (bool, error)
}

// LedgerClient represents non-blocking write calls.
type LedgerClient interface {
	WritePassportLifecycle(agentID, passportID, eventType, reason string) error
}

// OutboxWriter records non-blocking lifecycle events.
type OutboxWriter interface {
	Enqueue(eventType, agentID, passportID, reason string) error
}

// IssueInput carries IssuePassport request parameters.
type IssueInput struct {
	AgentID             string
	PassportType        domain.PassportType
	Scopes              []domain.Scope
	TTLSeconds          int64
	IssuedBy            string
	ForceReissue        bool
	Reason              string
	PolicyIDAtIssuance  string
	PolicyVerAtIssuance string
}

// Service performs passport issuance operations.
type Service struct {
	mu                           sync.RWMutex
	passports                    map[string]domain.Passport
	activeByAgent                map[string]string
	registry                     AgentRegistryClient
	policyBundles                PolicyBundleRegistryClient
	credentials                  CredentialStoreClient
	ledger                       LedgerClient
	outbox                       OutboxWriter
	provisionalAllowedActionType map[string]bool
}

// New returns a new Service. Optional policyBundles may be nil.
func New(registry AgentRegistryClient, credentials CredentialStoreClient, ledger LedgerClient, outbox OutboxWriter, policyBundles ...PolicyBundleRegistryClient) *Service {
	var bundles PolicyBundleRegistryClient
	if len(policyBundles) > 0 {
		bundles = policyBundles[0]
	}
	return &Service{
		passports:     make(map[string]domain.Passport),
		activeByAgent: make(map[string]string),
		registry:      registry,
		policyBundles: bundles,
		credentials:   credentials,
		ledger:        ledger,
		outbox:        outbox,
		provisionalAllowedActionType: map[string]bool{
			"READ":   true,
			"NOTIFY": true,
			"QUERY":  true,
		},
	}
}

// IssuePassport issues a signed passport with enforced invariants.
func (s *Service) IssuePassport(input IssueInput) (domain.Passport, error) {
	if input.AgentID == "" || input.IssuedBy == "" || input.Reason == "" || len(input.Scopes) == 0 {
		return domain.Passport{}, ErrInvalidArgument
	}
	if input.TTLSeconds < 300 {
		return domain.Passport{}, fmt.Errorf("%w: ttl too low", ErrInvalidArgument)
	}
	if input.PassportType == domain.TypeProvisional && input.TTLSeconds > 86400 {
		return domain.Passport{}, fmt.Errorf("%w: ttl too high for provisional", ErrInvalidArgument)
	}
	if input.PassportType == domain.TypeStandard && input.TTLSeconds > 2592000 {
		return domain.Passport{}, fmt.Errorf("%w: ttl too high for standard", ErrInvalidArgument)
	}

	status, err := s.registry.GetAgentStatus(input.AgentID)
	if err != nil {
		return domain.Passport{}, ErrUnavailable
	}
	if status == AgentDeregistered || status == AgentSuspended {
		return domain.Passport{}, ErrFailedPrecondition
	}
	if status != AgentActive && status != AgentPending {
		return domain.Passport{}, ErrFailedPrecondition
	}

	if input.PassportType == domain.TypeProvisional {
		for _, scope := range input.Scopes {
			if !s.provisionalAllowedActionType[scope.ActionClass] {
				return domain.Passport{}, fmt.Errorf("%w: action not allowed for provisional", ErrInvalidArgument)
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if currentID, ok := s.activeByAgent[input.AgentID]; ok {
		if !input.ForceReissue {
			return domain.Passport{}, fmt.Errorf("%w: active passport %s", ErrAlreadyExists, currentID)
		}
		p := s.passports[currentID]
		now := time.Now().UTC()
		p.Status = domain.StatusRevoked
		p.RevokedTS = &now
		p.RevocationReason = "superseded_by_force_reissue"
		s.passports[currentID] = p
		_ = s.outbox.Enqueue("PASSPORT_REVOKED", p.AgentID, p.PassportID, p.RevocationReason)
	}

	passportID := uuid.NewString()
	now := time.Now().UTC()
	expires := now.Add(time.Duration(input.TTLSeconds) * time.Second)
	payload := canonicalPayload(input.AgentID, input.PassportType, input.Scopes, now, expires)
	credentialID, publicKey, signature, err := s.credentials.Sign(payload)
	if err != nil {
		return domain.Passport{}, ErrUnavailable
	}

	policyID, policyVer := input.PolicyIDAtIssuance, input.PolicyVerAtIssuance
	if policyID == "" && policyVer == "" && s.policyBundles != nil {
		var errBundle error
		policyID, policyVer, errBundle = s.policyBundles.ActivePolicyBundle()
		if errBundle != nil {
			return domain.Passport{}, ErrUnavailable
		}
	}

	passport := domain.Passport{
		PassportID:          passportID,
		AgentID:             input.AgentID,
		PassportType:        input.PassportType,
		ScopeSet:            input.Scopes,
		Status:              domain.StatusActive,
		IssuedBy:            input.IssuedBy,
		CredentialID:        credentialID,
		PublicKeyPEM:        publicKey,
		Signature:           signature,
		Reason:              input.Reason,
		IssuedTS:            now,
		ExpiresTS:           expires,
		PolicyIDAtIssuance:  policyID,
		PolicyVerAtIssuance: policyVer,
	}
	s.passports[passportID] = passport
	s.activeByAgent[input.AgentID] = passportID
	_ = s.outbox.Enqueue("PASSPORT_ISSUED", input.AgentID, passportID, input.Reason)
	if s.ledger != nil {
		_ = s.ledger.WritePassportLifecycle(input.AgentID, passportID, "PASSPORT_ISSUED", input.Reason)
	}
	return passport, nil
}

// GetPassport returns a passport by id.
func (s *Service) GetPassport(passportID string) (domain.Passport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.passports[passportID]
	if !ok {
		return domain.Passport{}, ErrNotFound
	}
	return p, nil
}

// GetAgentPassport returns active passport for an agent.
func (s *Service) GetAgentPassport(agentID string) (domain.Passport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.activeByAgent[agentID]
	if !ok {
		return domain.Passport{}, ErrNotFound
	}
	return s.passports[id], nil
}

// VerifyPassport verifies signature and status.
func (s *Service) VerifyPassport(passportID string, payload, signature []byte) (bool, domain.PassportStatus, string, error) {
	p, err := s.GetPassport(passportID)
	if err != nil {
		return false, "", "", err
	}
	if p.Status == domain.StatusRevoked {
		return false, p.Status, "revoked", nil
	}
	if p.Status == domain.StatusExpired || time.Now().UTC().After(p.ExpiresTS) {
		return false, domain.StatusExpired, "expired", nil
	}
	ok, err := s.credentials.Verify(p.PublicKeyPEM, payload, signature)
	if err != nil {
		return false, p.Status, "verify_error", ErrUnavailable
	}
	if !ok {
		return false, p.Status, "signature_invalid", nil
	}
	return true, p.Status, "", nil
}

// RevokePassport marks a passport revoked and clears it as the agent's active passport when applicable.
func (s *Service) RevokePassport(passportID, reason, revokedBy string) (domain.Passport, error) {
	if passportID == "" || reason == "" || revokedBy == "" {
		return domain.Passport{}, ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.passports[passportID]
	if !ok {
		return domain.Passport{}, ErrNotFound
	}
	if p.Status == domain.StatusRevoked {
		return p, nil
	}
	now := time.Now().UTC()
	p.Status = domain.StatusRevoked
	p.RevokedTS = &now
	p.RevocationReason = reason
	s.passports[passportID] = p
	if activeID, ok := s.activeByAgent[p.AgentID]; ok && activeID == passportID {
		delete(s.activeByAgent, p.AgentID)
	}
	_ = s.outbox.Enqueue("PASSPORT_REVOKED", p.AgentID, passportID, reason)
	if s.ledger != nil {
		_ = s.ledger.WritePassportLifecycle(p.AgentID, passportID, "PASSPORT_REVOKED", reason)
	}
	return p, nil
}

func canonicalPayload(agentID string, pType domain.PassportType, scopes []domain.Scope, issued, expires time.Time) []byte {
	h := sha256.New()
	h.Write([]byte(agentID))
	h.Write([]byte(string(pType)))
	for _, s := range scopes {
		h.Write([]byte(s.ScopeID))
		h.Write([]byte(s.ActionClass))
		h.Write([]byte(s.ResourcePattern))
	}
	h.Write([]byte(issued.Format(time.RFC3339Nano)))
	h.Write([]byte(expires.Format(time.RFC3339Nano)))
	sum := h.Sum(nil)
	out := make([]byte, hex.EncodedLen(len(sum)))
	hex.Encode(out, sum)
	return out
}
