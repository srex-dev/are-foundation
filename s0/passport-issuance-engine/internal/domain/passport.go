package domain

import "time"

// PassportStatus describes passport lifecycle status.
type PassportStatus string

const (
	StatusActive     PassportStatus = "ACTIVE"
	StatusExpired    PassportStatus = "EXPIRED"
	StatusRevoked    PassportStatus = "REVOKED"
	StatusSuperseded PassportStatus = "SUPERSEDED"
)

// PassportType describes the type of passport.
type PassportType string

const (
	TypeProvisional PassportType = "PROVISIONAL"
	TypeStandard    PassportType = "STANDARD"
)

// Scope binds an action class and resource pattern.
type Scope struct {
	ScopeID            string
	ActionClass        string
	ResourcePattern    string
	ContextConstraints map[string]string
	IsEscalation       bool
	GrantedTS          time.Time
	ExpiresTS          time.Time
}

// Passport represents a signed authorization artifact.
type Passport struct {
	PassportID          string
	AgentID             string
	PassportType        PassportType
	ScopeSet            []Scope
	Status              PassportStatus
	IssuedBy            string
	CredentialID        string
	PublicKeyPEM        string
	Signature           []byte
	Reason              string
	IssuedTS            time.Time
	ExpiresTS           time.Time
	RevokedTS           *time.Time
	RevocationReason    string
	SupersededBy        string
	PolicyIDAtIssuance  string
	PolicyVerAtIssuance string
}
