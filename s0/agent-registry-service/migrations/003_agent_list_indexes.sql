-- Support owner-scoped listing with stable cursor pagination (agent_id order within owner).
CREATE INDEX IF NOT EXISTS idx_agents_owner_agent ON agents(owner_id, agent_id);
