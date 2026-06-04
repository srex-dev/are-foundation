use std::collections::HashMap;
use std::sync::Arc;

use async_trait::async_trait;
use chrono::Utc;
use tokio::sync::Mutex;

use crate::cache::PassportCache;
use crate::glob_match::{glob_match, GlobError};
use crate::types::{
    EvaluateScopeRequest, EvaluateScopeResponse, GetAgentScopeSetResponse, Passport, ScopeCheck,
    ScopeMatch, ScopeMiss,
};

#[derive(Debug, thiserror::Error, PartialEq, Eq, Clone)]
pub enum ScopeError {
    #[error("invalid argument")]
    InvalidArgument,
    #[error("not found")]
    NotFound,
    #[error("unavailable")]
    Unavailable,
}

#[async_trait]
pub trait PassportClient: Send + Sync {
    async fn get_agent_passport(&self, agent_id: &str) -> Result<Passport, ScopeError>;
}

#[derive(Default)]
struct CircuitBreaker {
    consecutive_failures: u32,
    is_open: bool,
}

pub struct ScopeEvaluatorService {
    cache: Arc<Mutex<PassportCache>>,
    passport_client: Arc<dyn PassportClient>,
    circuit: Arc<Mutex<CircuitBreaker>>,
    threshold: u32,
}

impl ScopeEvaluatorService {
    pub fn new(
        passport_client: Arc<dyn PassportClient>,
        cache_max_entries: usize,
        cache_ttl_seconds: u64,
        threshold: u32,
    ) -> Self {
        Self {
            cache: Arc::new(Mutex::new(PassportCache::new(
                cache_max_entries,
                cache_ttl_seconds,
            ))),
            passport_client,
            circuit: Arc::new(Mutex::new(CircuitBreaker::default())),
            threshold,
        }
    }

    pub async fn evaluate_scope(
        &self,
        request: EvaluateScopeRequest,
    ) -> Result<EvaluateScopeResponse, ScopeError> {
        if request.evaluation_id.is_empty()
            || request.agent_id.is_empty()
            || request.action_class.is_empty()
            || request.resource.is_empty()
        {
            return Err(ScopeError::InvalidArgument);
        }
        let now = Utc::now().timestamp_millis();
        let (passport, cache_hit) = self.get_or_fetch_passport(&request.agent_id).await?;
        let passport_expired = passport.expires_ts <= now;

        let mut matches = Vec::new();
        let mut misses = Vec::new();

        for scope in &passport.scopes {
            if scope.expires_ts <= now {
                misses.push(miss(scope, "SCOPE_EXPIRED"));
                continue;
            }
            if scope.action_class != request.action_class {
                misses.push(miss(scope, "ACTION_CLASS_MISMATCH"));
                continue;
            }
            match glob_match(&scope.resource_pattern, &request.resource) {
                Ok(true) => {}
                Ok(false) => {
                    misses.push(miss(scope, "RESOURCE_PATTERN_NO_MATCH"));
                    continue;
                }
                Err(GlobError::InvalidPattern | GlobError::PatternTooComplex) => {
                    misses.push(miss(scope, "INVALID_PATTERN"));
                    continue;
                }
            }
            if !context_matches(&scope.context_constraints, &request.context) {
                misses.push(miss(scope, "CONTEXT_CONSTRAINT_FAILED"));
                continue;
            }
            matches.push(ScopeMatch {
                scope_id: scope.scope_id.clone(),
                action_class: scope.action_class.clone(),
                resource_pattern: scope.resource_pattern.clone(),
                is_escalation: scope.is_escalation,
                scope_expires_ts: scope.expires_ts,
            });
        }

        Ok(EvaluateScopeResponse {
            evaluation_id: request.evaluation_id,
            agent_id: request.agent_id,
            in_scope: !passport_expired && !matches.is_empty(),
            matches,
            misses,
            passport_id: passport.passport_id,
            passport_type: passport.passport_type,
            passport_expires_ts: passport.expires_ts,
            passport_expired,
            cache_hit,
            evaluated_ts: now,
        })
    }

    pub async fn evaluate_scope_batch(
        &self,
        agent_id: &str,
        checks: Vec<ScopeCheck>,
    ) -> Result<Vec<EvaluateScopeResponse>, ScopeError> {
        if agent_id.is_empty() {
            return Err(ScopeError::InvalidArgument);
        }
        let mut out = Vec::with_capacity(checks.len());
        for check in checks {
            let req = EvaluateScopeRequest {
                evaluation_id: check.evaluation_id,
                agent_id: agent_id.to_string(),
                action_class: check.action_class,
                resource: check.resource,
                context: check.context,
            };
            out.push(self.evaluate_scope(req).await?);
        }
        Ok(out)
    }

    pub async fn get_agent_scope_set(
        &self,
        agent_id: &str,
    ) -> Result<GetAgentScopeSetResponse, ScopeError> {
        if agent_id.is_empty() {
            return Err(ScopeError::InvalidArgument);
        }
        let (passport, cache_hit) = self.get_or_fetch_passport(agent_id).await?;
        Ok(GetAgentScopeSetResponse {
            agent_id: agent_id.to_string(),
            passport_id: passport.passport_id,
            passport_type: passport.passport_type,
            scopes: passport.scopes,
            cache_hit,
            passport_expires_ts: passport.expires_ts,
        })
    }

    pub async fn invalidate_cache(&self, agent_id: &str) -> Result<bool, ScopeError> {
        if agent_id.is_empty() {
            return Err(ScopeError::InvalidArgument);
        }
        let mut cache = self.cache.lock().await;
        Ok(cache.invalidate(agent_id))
    }

    pub async fn cache_size(&self) -> usize {
        self.cache.lock().await.len()
    }

    async fn get_or_fetch_passport(&self, agent_id: &str) -> Result<(Passport, bool), ScopeError> {
        {
            let mut cache = self.cache.lock().await;
            if let Some(p) = cache.get(agent_id) {
                return Ok((p, true));
            }
        }

        {
            let circuit = self.circuit.lock().await;
            if circuit.is_open {
                return Err(ScopeError::Unavailable);
            }
        }

        match self.passport_client.get_agent_passport(agent_id).await {
            Ok(passport) => {
                let mut circuit = self.circuit.lock().await;
                circuit.consecutive_failures = 0;
                circuit.is_open = false;
                drop(circuit);

                let mut cache = self.cache.lock().await;
                cache.insert(agent_id.to_string(), passport.clone());
                Ok((passport, false))
            }
            Err(err) => {
                let mut circuit = self.circuit.lock().await;
                circuit.consecutive_failures += 1;
                if circuit.consecutive_failures >= self.threshold {
                    circuit.is_open = true;
                }
                if err == ScopeError::NotFound {
                    Err(ScopeError::NotFound)
                } else {
                    Err(ScopeError::Unavailable)
                }
            }
        }
    }
}

fn context_matches(
    constraints: &HashMap<String, String>,
    request: &HashMap<String, String>,
) -> bool {
    if constraints.is_empty() {
        return true;
    }
    if request.len() != constraints.len() {
        return false;
    }
    constraints
        .iter()
        .all(|(k, v)| request.get(k).map(|x| x == v).unwrap_or(false))
}

fn miss(scope: &crate::types::ScopeEntry, reason: &str) -> ScopeMiss {
    ScopeMiss {
        scope_id: scope.scope_id.clone(),
        action_class: scope.action_class.clone(),
        miss_reason: reason.to_string(),
    }
}

#[cfg(test)]
mod tests {
    use super::{PassportClient, ScopeError, ScopeEvaluatorService};
    use crate::types::{EvaluateScopeRequest, Passport, ScopeCheck, ScopeEntry};
    use async_trait::async_trait;
    use chrono::Utc;
    use std::collections::HashMap;
    use std::sync::Arc;
    use tokio::sync::Mutex;

    struct MockClient {
        passport: Option<Passport>,
        error: Option<ScopeError>,
        calls: Arc<Mutex<u32>>,
    }

    #[async_trait]
    impl PassportClient for MockClient {
        async fn get_agent_passport(&self, _agent_id: &str) -> Result<Passport, ScopeError> {
            let mut c = self.calls.lock().await;
            *c += 1;
            if let Some(e) = &self.error {
                return Err(e.clone());
            }
            self.passport.clone().ok_or(ScopeError::NotFound)
        }
    }

    fn req() -> EvaluateScopeRequest {
        EvaluateScopeRequest {
            evaluation_id: "e1".into(),
            agent_id: "a1".into(),
            action_class: "READ".into(),
            resource: "data/users".into(),
            context: HashMap::new(),
        }
    }

    fn passport(scopes: Vec<ScopeEntry>, expires_ts: i64) -> Passport {
        Passport {
            passport_id: "p1".into(),
            passport_type: "STANDARD".into(),
            expires_ts,
            scopes,
        }
    }

    #[tokio::test]
    async fn determinism_same_input_1000_times() {
        let now = Utc::now().timestamp_millis();
        let client = Arc::new(MockClient {
            passport: Some(passport(
                vec![ScopeEntry {
                    scope_id: "s1".into(),
                    action_class: "READ".into(),
                    resource_pattern: "data/*".into(),
                    context_constraints: HashMap::new(),
                    expires_ts: now + 60_000,
                    is_escalation: false,
                }],
                now + 120_000,
            )),
            error: None,
            calls: Arc::new(Mutex::new(0)),
        });
        let svc = ScopeEvaluatorService::new(client, 500, 60, 5);
        let first = svc.evaluate_scope(req()).await.expect("eval should pass");
        for _ in 0..1000 {
            let r = svc.evaluate_scope(req()).await.expect("eval should pass");
            assert_eq!(r.in_scope, first.in_scope);
            assert_eq!(r.matches, first.matches);
            assert_eq!(r.misses, first.misses);
        }
    }

    #[tokio::test]
    async fn expired_scope_never_matches() {
        let now = Utc::now().timestamp_millis();
        let client = Arc::new(MockClient {
            passport: Some(passport(
                vec![ScopeEntry {
                    scope_id: "s1".into(),
                    action_class: "READ".into(),
                    resource_pattern: "data/*".into(),
                    context_constraints: HashMap::new(),
                    expires_ts: now - 1,
                    is_escalation: false,
                }],
                now + 60_000,
            )),
            error: None,
            calls: Arc::new(Mutex::new(0)),
        });
        let svc = ScopeEvaluatorService::new(client, 500, 60, 5);
        let res = svc.evaluate_scope(req()).await.expect("eval should pass");
        assert!(!res.in_scope);
        assert_eq!(res.misses[0].miss_reason, "SCOPE_EXPIRED");
    }

    #[tokio::test]
    async fn expired_passport_never_in_scope() {
        let now = Utc::now().timestamp_millis();
        let client = Arc::new(MockClient {
            passport: Some(passport(
                vec![ScopeEntry {
                    scope_id: "s1".into(),
                    action_class: "READ".into(),
                    resource_pattern: "data/*".into(),
                    context_constraints: HashMap::new(),
                    expires_ts: now + 10_000,
                    is_escalation: false,
                }],
                now - 1,
            )),
            error: None,
            calls: Arc::new(Mutex::new(0)),
        });
        let svc = ScopeEvaluatorService::new(client, 500, 60, 5);
        let res = svc.evaluate_scope(req()).await.expect("eval should pass");
        assert!(!res.in_scope);
        assert!(res.passport_expired);
    }

    #[tokio::test]
    async fn no_passport_returns_not_found() {
        let client = Arc::new(MockClient {
            passport: None,
            error: Some(ScopeError::NotFound),
            calls: Arc::new(Mutex::new(0)),
        });
        let svc = ScopeEvaluatorService::new(client, 500, 60, 5);
        let err = svc
            .evaluate_scope(req())
            .await
            .expect_err("expected not found");
        assert_eq!(err, ScopeError::NotFound);
    }

    #[tokio::test]
    async fn returns_all_matching_scopes() {
        let now = Utc::now().timestamp_millis();
        let scopes = vec![
            ScopeEntry {
                scope_id: "s1".into(),
                action_class: "READ".into(),
                resource_pattern: "data/*".into(),
                context_constraints: HashMap::new(),
                expires_ts: now + 1_000,
                is_escalation: false,
            },
            ScopeEntry {
                scope_id: "s2".into(),
                action_class: "READ".into(),
                resource_pattern: "data/**".into(),
                context_constraints: HashMap::new(),
                expires_ts: now + 1_000,
                is_escalation: true,
            },
        ];
        let client = Arc::new(MockClient {
            passport: Some(passport(scopes, now + 10_000)),
            error: None,
            calls: Arc::new(Mutex::new(0)),
        });
        let svc = ScopeEvaluatorService::new(client, 500, 60, 5);
        let res = svc.evaluate_scope(req()).await.expect("eval should pass");
        assert!(res.in_scope);
        assert_eq!(res.matches.len(), 2);
    }

    #[tokio::test]
    async fn context_constraints_match_and_fail() {
        let now = Utc::now().timestamp_millis();
        let mut constraints = HashMap::new();
        constraints.insert("env".into(), "prod".into());
        let client = Arc::new(MockClient {
            passport: Some(passport(
                vec![ScopeEntry {
                    scope_id: "s1".into(),
                    action_class: "READ".into(),
                    resource_pattern: "data/*".into(),
                    context_constraints: constraints,
                    expires_ts: now + 60_000,
                    is_escalation: false,
                }],
                now + 60_000,
            )),
            error: None,
            calls: Arc::new(Mutex::new(0)),
        });
        let svc = ScopeEvaluatorService::new(client, 500, 60, 5);
        let mut ok_req = req();
        ok_req.context.insert("env".into(), "prod".into());
        let ok = svc.evaluate_scope(ok_req).await.expect("eval should pass");
        assert!(ok.in_scope);

        let bad = svc.evaluate_scope(req()).await.expect("eval should pass");
        assert!(!bad.in_scope);
        assert_eq!(bad.misses[0].miss_reason, "CONTEXT_CONSTRAINT_FAILED");
    }

    #[tokio::test]
    async fn context_rejects_extra_unexpected_keys() {
        let now = Utc::now().timestamp_millis();
        let mut constraints = HashMap::new();
        constraints.insert("env".into(), "prod".into());
        let client = Arc::new(MockClient {
            passport: Some(passport(
                vec![ScopeEntry {
                    scope_id: "s1".into(),
                    action_class: "READ".into(),
                    resource_pattern: "data/*".into(),
                    context_constraints: constraints,
                    expires_ts: now + 60_000,
                    is_escalation: false,
                }],
                now + 60_000,
            )),
            error: None,
            calls: Arc::new(Mutex::new(0)),
        });
        let svc = ScopeEvaluatorService::new(client, 500, 60, 5);
        let mut r = req();
        r.context.insert("env".into(), "prod".into());
        r.context.insert("extra_override".into(), "admin".into());
        let res = svc.evaluate_scope(r).await.expect("eval should pass");
        assert!(!res.in_scope);
    }

    #[tokio::test]
    async fn invalidate_cache_and_batch_order() {
        let now = Utc::now().timestamp_millis();
        let calls = Arc::new(Mutex::new(0));
        let client = Arc::new(MockClient {
            passport: Some(passport(
                vec![ScopeEntry {
                    scope_id: "s1".into(),
                    action_class: "READ".into(),
                    resource_pattern: "data/*".into(),
                    context_constraints: HashMap::new(),
                    expires_ts: now + 10_000,
                    is_escalation: false,
                }],
                now + 10_000,
            )),
            error: None,
            calls: calls.clone(),
        });
        let svc = ScopeEvaluatorService::new(client, 500, 60, 5);

        let first = svc.evaluate_scope(req()).await.expect("eval should pass");
        assert!(!first.cache_hit);
        let second = svc.evaluate_scope(req()).await.expect("eval should pass");
        assert!(second.cache_hit);

        assert!(svc
            .invalidate_cache("a1")
            .await
            .expect("invalidate should pass"));
        let third = svc.evaluate_scope(req()).await.expect("eval should pass");
        assert!(!third.cache_hit);

        let checks = vec![
            ScopeCheck {
                evaluation_id: "b1".into(),
                action_class: "READ".into(),
                resource: "data/users".into(),
                context: HashMap::new(),
            },
            ScopeCheck {
                evaluation_id: "b2".into(),
                action_class: "WRITE".into(),
                resource: "data/users".into(),
                context: HashMap::new(),
            },
        ];
        let out = svc
            .evaluate_scope_batch("a1", checks)
            .await
            .expect("batch should pass");
        assert_eq!(out.len(), 2);
        assert_eq!(out[0].evaluation_id, "b1");
        assert_eq!(out[1].evaluation_id, "b2");
    }

    #[tokio::test]
    async fn service_unavailable_and_circuit_breaker() {
        let calls = Arc::new(Mutex::new(0));
        let client = Arc::new(MockClient {
            passport: None,
            error: Some(ScopeError::Unavailable),
            calls: calls.clone(),
        });
        let svc = ScopeEvaluatorService::new(client, 500, 60, 5);
        for _ in 0..5 {
            let err = svc
                .evaluate_scope(req())
                .await
                .expect_err("expected unavailable");
            assert_eq!(err, ScopeError::Unavailable);
        }
        let before = *calls.lock().await;
        let err = svc
            .evaluate_scope(req())
            .await
            .expect_err("expected open-circuit unavailable");
        assert_eq!(err, ScopeError::Unavailable);
        let after = *calls.lock().await;
        assert_eq!(before, after);
    }

    #[tokio::test]
    async fn malformed_pattern_returns_invalid_pattern_miss() {
        let now = Utc::now().timestamp_millis();
        let client = Arc::new(MockClient {
            passport: Some(passport(
                vec![ScopeEntry {
                    scope_id: "s1".into(),
                    action_class: "READ".into(),
                    resource_pattern: "data/[abc]".into(),
                    context_constraints: HashMap::new(),
                    expires_ts: now + 1_000,
                    is_escalation: false,
                }],
                now + 10_000,
            )),
            error: None,
            calls: Arc::new(Mutex::new(0)),
        });
        let svc = ScopeEvaluatorService::new(client, 500, 60, 5);
        let res = svc.evaluate_scope(req()).await.expect("eval should pass");
        assert!(!res.in_scope);
        assert_eq!(res.misses[0].miss_reason, "INVALID_PATTERN");
    }

    #[tokio::test]
    async fn invalid_arguments_and_scope_set_paths() {
        let now = Utc::now().timestamp_millis();
        let client = Arc::new(MockClient {
            passport: Some(passport(
                vec![ScopeEntry {
                    scope_id: "s1".into(),
                    action_class: "READ".into(),
                    resource_pattern: "data/*".into(),
                    context_constraints: HashMap::new(),
                    expires_ts: now + 10_000,
                    is_escalation: false,
                }],
                now + 20_000,
            )),
            error: None,
            calls: Arc::new(Mutex::new(0)),
        });
        let svc = ScopeEvaluatorService::new(client, 500, 60, 5);

        let bad_req = EvaluateScopeRequest {
            evaluation_id: "".into(),
            agent_id: "a1".into(),
            action_class: "READ".into(),
            resource: "data/users".into(),
            context: HashMap::new(),
        };
        let err = svc
            .evaluate_scope(bad_req)
            .await
            .expect_err("expected invalid argument");
        assert_eq!(err, ScopeError::InvalidArgument);

        let scope_set = svc
            .get_agent_scope_set("a1")
            .await
            .expect("scope set should return");
        assert_eq!(scope_set.agent_id, "a1");
        assert_eq!(scope_set.scopes.len(), 1);
        assert_eq!(svc.cache_size().await, 1);

        let err = svc
            .get_agent_scope_set("")
            .await
            .expect_err("expected invalid argument");
        assert_eq!(err, ScopeError::InvalidArgument);

        let err = svc
            .evaluate_scope_batch("", vec![])
            .await
            .expect_err("expected invalid argument");
        assert_eq!(err, ScopeError::InvalidArgument);

        let err = svc
            .invalidate_cache("")
            .await
            .expect_err("expected invalid argument");
        assert_eq!(err, ScopeError::InvalidArgument);
    }
}
