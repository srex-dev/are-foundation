CREATE SCHEMA IF NOT EXISTS are_ledger;

CREATE TABLE IF NOT EXISTS are_ledger.ledger_entries (
  entry_id UUID PRIMARY KEY,
  entry_type VARCHAR(50) NOT NULL,
  agent_id VARCHAR(100) NOT NULL,
  content BYTEA NOT NULL,
  content_type VARCHAR(100) NOT NULL,
  source_id VARCHAR(100) NOT NULL,
  correlation_id UUID NULL,
  idempotency_key VARCHAR(255) NULL,
  entry_hash VARCHAR(64) NOT NULL,
  previous_hash VARCHAR(64) NOT NULL,
  chain_position BIGINT NOT NULL,
  written_ts TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ledger_type_position
  ON are_ledger.ledger_entries(entry_type, chain_position);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ledger_idempotency
  ON are_ledger.ledger_entries(entry_type, idempotency_key)
  WHERE idempotency_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ledger_agent_id
  ON are_ledger.ledger_entries(agent_id);
CREATE INDEX IF NOT EXISTS idx_ledger_entry_type
  ON are_ledger.ledger_entries(entry_type);
CREATE INDEX IF NOT EXISTS idx_ledger_written_ts
  ON are_ledger.ledger_entries(written_ts);
CREATE INDEX IF NOT EXISTS idx_ledger_source_id
  ON are_ledger.ledger_entries(source_id);
CREATE INDEX IF NOT EXISTS idx_ledger_correlation_id
  ON are_ledger.ledger_entries(correlation_id)
  WHERE correlation_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS are_ledger.ledger_chain_tips (
  entry_type VARCHAR(50) PRIMARY KEY,
  last_entry_id UUID NOT NULL,
  last_hash VARCHAR(64) NOT NULL,
  last_position BIGINT NOT NULL,
  updated_ts TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS are_ledger.ledger_write_outbox (
  outbox_id UUID PRIMARY KEY,
  entry_id UUID NOT NULL,
  entry_type VARCHAR(50) NOT NULL,
  payload JSONB NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
  created_ts TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_attempt_ts TIMESTAMPTZ NULL,
  attempt_count INT NOT NULL DEFAULT 0,
  CONSTRAINT ledger_outbox_status_check CHECK (status IN ('PENDING', 'DELIVERED', 'FAILED'))
);

CREATE INDEX IF NOT EXISTS idx_ledger_outbox_status
  ON are_ledger.ledger_write_outbox(status, created_ts)
  WHERE status = 'PENDING';

-- Runtime grant example (adjust role name): the application role needs INSERT+SELECT on
-- are_ledger.ledger_entries (no UPDATE/DELETE) plus INSERT+SELECT+UPDATE on
-- are_ledger.ledger_write_outbox for outbox delivery status.

