-- Immutable admission envelope (1:1 with agent) for invariant reintroduction loop (ADR-012).
CREATE TABLE IF NOT EXISTS admission_envelopes (
    envelope_id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL UNIQUE REFERENCES agents (agent_id) ON DELETE CASCADE,
    policy_id TEXT,
    policy_ver TEXT,
    admitted_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    admitted_behavioral_caps JSONB NOT NULL DEFAULT '{}'::jsonb,
    admitted_ts TIMESTAMPTZ NOT NULL,
    issuing_authority TEXT NOT NULL DEFAULT 'agent-registry',
    signature BYTEA NOT NULL
);

CREATE INDEX IF NOT EXISTS admission_envelopes_agent_id_idx ON admission_envelopes (agent_id);
