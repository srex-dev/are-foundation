CREATE TABLE IF NOT EXISTS passports (
  passport_id UUID PRIMARY KEY,
  agent_id UUID NOT NULL,
  passport_type VARCHAR(20) NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
  issued_by VARCHAR(255) NOT NULL,
  credential_id UUID NOT NULL,
  public_key_pem TEXT NOT NULL,
  signature BYTEA NOT NULL,
  reason TEXT NOT NULL,
  issued_ts TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_ts TIMESTAMPTZ NOT NULL,
  revoked_ts TIMESTAMPTZ,
  revocation_reason TEXT,
  superseded_by UUID,
  updated_ts TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT passports_type_check CHECK (passport_type IN ('PROVISIONAL','STANDARD')),
  CONSTRAINT passports_status_check CHECK (status IN ('ACTIVE','EXPIRED','REVOKED','SUPERSEDED'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_passports_one_active_per_agent
  ON passports(agent_id) WHERE status = 'ACTIVE';

CREATE TABLE IF NOT EXISTS passport_scopes (
  scope_id UUID PRIMARY KEY,
  passport_id UUID NOT NULL REFERENCES passports(passport_id),
  action_class VARCHAR(50) NOT NULL,
  resource_pattern VARCHAR(500) NOT NULL,
  context_constraints JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_escalation BOOLEAN NOT NULL DEFAULT FALSE,
  granted_ts TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_ts TIMESTAMPTZ NOT NULL
);
