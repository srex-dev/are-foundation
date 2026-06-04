package domain

import "time"

const (
	StatusPending      = "PENDING"
	StatusActive       = "ACTIVE"
	StatusSuspended    = "SUSPENDED"
	StatusDeregistered = "DEREGISTERED"
)

var validTransitions = map[string]map[string]bool{
	StatusPending: {
		StatusActive: true,
	},
	StatusActive: {
		StatusSuspended:    true,
		StatusDeregistered: true,
	},
	StatusSuspended: {
		StatusActive:       true,
		StatusDeregistered: true,
	},
	StatusDeregistered: {},
}

// Agent is the core identity entity owned by this component.
type Agent struct {
	AgentID              string
	AgentType            string
	OwnerID              string
	ExternalID           string
	Status               string
	PassportID           string
	Metadata             map[string]string
	RegistrationTS       time.Time
	UpdatedTS            time.Time
	AdmissionConstraints map[string]any
	AdmittedPolicyID     string
	AdmittedPolicyVer    string
	AdmissionTS          time.Time
}

// AdmissionSnapshot is optional data captured once at registration (immutable after write).
type AdmissionSnapshot struct {
	Constraints map[string]any
	PolicyID    string
	PolicyVer   string
	// ScopePatterns are admitted scope identifiers/patterns (e.g. action class prefixes).
	ScopePatterns []string
	// BehavioralCaps optional explicit metric cap overrides (metric -> max).
	BehavioralCaps map[string]float64
	// EnvelopeID stable id; if empty the service assigns one before persistence.
	EnvelopeID string
	// IssuingAuthority signer label (empty → default in NewAdmissionEnvelope).
	IssuingAuthority string
	// Signature opaque bytes (may be empty when signing is not configured).
	Signature []byte
}

// AdmissionEnvelope is the immutable admission contract for one agent (ADR-012).
type AdmissionEnvelope struct {
	EnvelopeID             string
	AgentID                string
	PolicyID               string
	PolicyVer              string
	AdmittedScopes         []string
	AdmittedBehavioralCaps map[string]float64
	AdmittedTS             time.Time
	IssuingAuthority       string
	Signature              []byte
}

// NewAdmissionEnvelope builds an envelope from a registration snapshot (no crypto here).
func NewAdmissionEnvelope(
	envelopeID, agentID string,
	admittedTS time.Time,
	snap *AdmissionSnapshot,
	signature []byte,
	issuingAuthority string,
) *AdmissionEnvelope {
	if snap == nil {
		return nil
	}
	if issuingAuthority == "" {
		issuingAuthority = "agent-registry"
	}
	caps := mergeBehavioralCaps(snap)
	scopes := append([]string(nil), snap.ScopePatterns...)
	return &AdmissionEnvelope{
		EnvelopeID:             envelopeID,
		AgentID:                agentID,
		PolicyID:               snap.PolicyID,
		PolicyVer:              snap.PolicyVer,
		AdmittedScopes:         scopes,
		AdmittedBehavioralCaps: caps,
		AdmittedTS:             admittedTS,
		IssuingAuthority:       issuingAuthority,
		Signature:              append([]byte(nil), signature...),
	}
}

func mergeBehavioralCaps(snap *AdmissionSnapshot) map[string]float64 {
	out := make(map[string]float64)
	for k, v := range snap.Constraints {
		switch t := v.(type) {
		case float64:
			out[k] = t
		case int:
			out[k] = float64(t)
		case int64:
			out[k] = float64(t)
		default:
			// skip non-numeric constraint keys for behavioral caps
		}
	}
	for k, v := range snap.BehavioralCaps {
		out[k] = v
	}
	return out
}

// CanTransition checks whether status can move to newStatus.
func CanTransition(current, newStatus string) bool {
	if current == newStatus {
		return true
	}
	next, ok := validTransitions[current]
	if !ok {
		return false
	}
	return next[newStatus]
}
