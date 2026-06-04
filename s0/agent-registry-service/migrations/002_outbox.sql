CREATE TABLE IF NOT EXISTS agent_lifecycle_outbox (
  outbox_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  agent_id         UUID NOT NULL,
  event_type       VARCHAR(50) NOT NULL,
  payload          JSONB NOT NULL,
  status           VARCHAR(20) NOT NULL DEFAULT 'PENDING',
  created_ts       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_attempt_ts  TIMESTAMPTZ,
  attempt_count    INT NOT NULL DEFAULT 0,
  CONSTRAINT outbox_status_check CHECK (status IN ('PENDING','DELIVERED','FAILED'))
);

CREATE INDEX IF NOT EXISTS idx_outbox_status
  ON agent_lifecycle_outbox(status, created_ts)
  WHERE status = 'PENDING';
