use std::sync::Arc;
use std::time::Instant;

use tonic::{Request, Response, Status};

use crate::metrics::{SCOPE_EVAL_DURATION_SECONDS, SCOPE_EVAL_TOTAL};
use crate::pb::are::scope::v1::scope_evaluator_service_server::ScopeEvaluatorService as GrpcScope;
use crate::pb::are::scope::v1::{
    EvaluateScopeBatchRequest, EvaluateScopeBatchResponse, EvaluateScopeRequest,
    EvaluateScopeResponse, GetAgentScopeSetRequest, GetAgentScopeSetResponse,
    InvalidateCacheRequest, InvalidateCacheResponse, ScopeMatch, ScopeMiss,
};
use crate::service::{ScopeError, ScopeEvaluatorService};
use crate::types::{EvaluateScopeRequest as DomainEvalReq, ScopeCheck as DomainScopeCheck};

pub struct ScopeGrpc {
    inner: Arc<ScopeEvaluatorService>,
}

impl ScopeGrpc {
    pub fn new(inner: Arc<ScopeEvaluatorService>) -> Self {
        Self { inner }
    }
}

#[tonic::async_trait]
impl GrpcScope for ScopeGrpc {
    async fn evaluate_scope(
        &self,
        request: Request<EvaluateScopeRequest>,
    ) -> Result<Response<EvaluateScopeResponse>, Status> {
        let r = request.into_inner();
        let t0 = Instant::now();
        let domain_req = DomainEvalReq {
            evaluation_id: r.evaluation_id,
            agent_id: r.agent_id,
            action_class: r.action_class,
            resource: r.resource,
            context: r.context,
        };
        let out = self.inner.evaluate_scope(domain_req).await;
        record_eval(&out, t0.elapsed().as_secs_f64());
        out.map(|v| Response::new(to_proto_eval(v)))
            .map_err(map_scope_err)
    }

    async fn evaluate_scope_batch(
        &self,
        request: Request<EvaluateScopeBatchRequest>,
    ) -> Result<Response<EvaluateScopeBatchResponse>, Status> {
        let r = request.into_inner();
        let checks: Vec<DomainScopeCheck> = r
            .checks
            .into_iter()
            .map(|c| DomainScopeCheck {
                evaluation_id: c.evaluation_id,
                action_class: c.action_class,
                resource: c.resource,
                context: c.context,
            })
            .collect();
        let t0 = Instant::now();
        let out = self.inner.evaluate_scope_batch(&r.agent_id, checks).await;
        let elapsed = t0.elapsed().as_secs_f64();
        match &out {
            Ok(rows) => {
                for row in rows {
                    let label = if row.in_scope { "permit" } else { "deny" };
                    SCOPE_EVAL_TOTAL.with_label_values(&[label]).inc();
                }
                if !rows.is_empty() {
                    SCOPE_EVAL_DURATION_SECONDS
                        .with_label_values(&["batch"])
                        .observe(elapsed);
                }
            }
            Err(_) => {
                SCOPE_EVAL_TOTAL.with_label_values(&["deny"]).inc();
            }
        }
        out.map(|results| {
            Response::new(EvaluateScopeBatchResponse {
                agent_id: r.agent_id,
                results: results.into_iter().map(to_proto_eval).collect(),
            })
        })
        .map_err(map_scope_err)
    }

    async fn get_agent_scope_set(
        &self,
        request: Request<GetAgentScopeSetRequest>,
    ) -> Result<Response<GetAgentScopeSetResponse>, Status> {
        let r = request.into_inner();
        self.inner
            .get_agent_scope_set(&r.agent_id)
            .await
            .map(|v| {
                Response::new(GetAgentScopeSetResponse {
                    agent_id: v.agent_id,
                    passport_id: v.passport_id,
                    passport_type: v.passport_type,
                    scopes: v
                        .scopes
                        .into_iter()
                        .map(|s| crate::pb::are::scope::v1::ScopeEntry {
                            scope_id: s.scope_id,
                            action_class: s.action_class,
                            resource_pattern: s.resource_pattern,
                            context_constraints: s.context_constraints,
                            expires_ts: s.expires_ts,
                            is_escalation: s.is_escalation,
                        })
                        .collect(),
                    cache_hit: v.cache_hit,
                    passport_expires_ts: v.passport_expires_ts,
                })
            })
            .map_err(map_scope_err)
    }

    async fn invalidate_cache(
        &self,
        request: Request<InvalidateCacheRequest>,
    ) -> Result<Response<InvalidateCacheResponse>, Status> {
        let r = request.into_inner();
        self.inner
            .invalidate_cache(&r.agent_id)
            .await
            .map(|was| {
                Response::new(InvalidateCacheResponse {
                    agent_id: r.agent_id,
                    was_cached: was,
                })
            })
            .map_err(map_scope_err)
    }
}

fn record_eval(out: &Result<crate::types::EvaluateScopeResponse, ScopeError>, secs: f64) {
    match out {
        Ok(r) => {
            let label = if r.in_scope { "permit" } else { "deny" };
            SCOPE_EVAL_TOTAL.with_label_values(&[label]).inc();
            SCOPE_EVAL_DURATION_SECONDS
                .with_label_values(&[label])
                .observe(secs);
        }
        Err(_) => {
            SCOPE_EVAL_TOTAL.with_label_values(&["deny"]).inc();
        }
    }
}

fn map_scope_err(e: ScopeError) -> Status {
    match e {
        ScopeError::InvalidArgument => Status::invalid_argument("invalid_argument"),
        ScopeError::NotFound => Status::not_found("not_found"),
        ScopeError::Unavailable => Status::unavailable("unavailable"),
    }
}

fn to_proto_eval(r: crate::types::EvaluateScopeResponse) -> EvaluateScopeResponse {
    EvaluateScopeResponse {
        evaluation_id: r.evaluation_id,
        agent_id: r.agent_id,
        in_scope: r.in_scope,
        matches: r.matches.into_iter().map(to_proto_match).collect(),
        misses: r.misses.into_iter().map(to_proto_miss).collect(),
        passport_id: r.passport_id,
        passport_type: r.passport_type,
        passport_expires_ts: r.passport_expires_ts,
        passport_expired: r.passport_expired,
        cache_hit: r.cache_hit,
        evaluated_ts: r.evaluated_ts,
    }
}

fn to_proto_match(m: crate::types::ScopeMatch) -> ScopeMatch {
    ScopeMatch {
        scope_id: m.scope_id,
        action_class: m.action_class,
        resource_pattern: m.resource_pattern,
        is_escalation: m.is_escalation,
        scope_expires_ts: m.scope_expires_ts,
    }
}

fn to_proto_miss(m: crate::types::ScopeMiss) -> ScopeMiss {
    ScopeMiss {
        scope_id: m.scope_id,
        action_class: m.action_class,
        miss_reason: m.miss_reason,
    }
}
