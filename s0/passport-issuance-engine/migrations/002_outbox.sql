CREATE TABLE IF NOT EXISTS passport_lifecycle_outbox (
  outbox_id UUID PRIMARY KEY,
  passport_id UUID NOT NULL,
  agent_id UUID NOT NULL,
  event_type VARCHAR(50) NOT NULL,
  payload JSONB NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
  created_ts TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_attempt_ts TIMESTAMPTZ,
  attempt_count INT NOT NULL DEFAULT 0,
  CONSTRAINT passport_outbox_status CHECK (status IN ('PENDING','DELIVERED','FAILED'))
);

CREATE INDEX IF NOT EXISTS idx_passport_outbox_pending
  ON passport_lifecycle_outbox(status, created_ts) WHERE status = 'PENDING';
