CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS agents (
  agent_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  agent_type       VARCHAR(50) NOT NULL,
  owner_id         UUID NOT NULL,
  external_id      VARCHAR(255),
  status           VARCHAR(50) NOT NULL DEFAULT 'PENDING',
  passport_id      UUID,
  metadata         JSONB DEFAULT '{}',
  registration_ts  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_ts       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT agents_external_id_owner_unique UNIQUE (external_id, owner_id),
  CONSTRAINT agents_status_check CHECK (status IN ('PENDING','ACTIVE','SUSPENDED','DEREGISTERED')),
  CONSTRAINT agents_type_check CHECK (agent_type IN ('AUTONOMOUS','ASSISTED','SYSTEM','EPHEMERAL'))
);

CREATE INDEX IF NOT EXISTS idx_agents_owner_id ON agents(owner_id);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
CREATE INDEX IF NOT EXISTS idx_agents_type ON agents(agent_type);
CREATE INDEX IF NOT EXISTS idx_agents_registration_ts ON agents(registration_ts);

CREATE TABLE IF NOT EXISTS agent_status_history (
  history_id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  agent_id         UUID NOT NULL REFERENCES agents(agent_id),
  previous_status  VARCHAR(50),
  new_status       VARCHAR(50) NOT NULL,
  reason           TEXT NOT NULL,
  changed_by       VARCHAR(255) NOT NULL,
  changed_ts       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_status_history_agent_id ON agent_status_history(agent_id);
