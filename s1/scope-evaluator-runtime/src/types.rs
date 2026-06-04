use std::collections::HashMap;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ScopeEntry {
    pub scope_id: String,
    pub action_class: String,
    pub resource_pattern: String,
    pub context_constraints: HashMap<String, String>,
    pub expires_ts: i64,
    pub is_escalation: bool,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Passport {
    pub passport_id: String,
    pub passport_type: String,
    pub expires_ts: i64,
    pub scopes: Vec<ScopeEntry>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct EvaluateScopeRequest {
    pub evaluation_id: String,
    pub agent_id: String,
    pub action_class: String,
    pub resource: String,
    pub context: HashMap<String, String>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ScopeCheck {
    pub evaluation_id: String,
    pub action_class: String,
    pub resource: String,
    pub context: HashMap<String, String>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ScopeMatch {
    pub scope_id: String,
    pub action_class: String,
    pub resource_pattern: String,
    pub is_escalation: bool,
    pub scope_expires_ts: i64,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ScopeMiss {
    pub scope_id: String,
    pub action_class: String,
    pub miss_reason: String,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct EvaluateScopeResponse {
    pub evaluation_id: String,
    pub agent_id: String,
    pub in_scope: bool,
    pub matches: Vec<ScopeMatch>,
    pub misses: Vec<ScopeMiss>,
    pub passport_id: String,
    pub passport_type: String,
    pub passport_expires_ts: i64,
    pub passport_expired: bool,
    pub cache_hit: bool,
    pub evaluated_ts: i64,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct GetAgentScopeSetResponse {
    pub agent_id: String,
    pub passport_id: String,
    pub passport_type: String,
    pub scopes: Vec<ScopeEntry>,
    pub cache_hit: bool,
    pub passport_expires_ts: i64,
}
