use std::collections::HashMap;

use async_trait::async_trait;
use tonic::{Code, Request};

use crate::pb::are::passport::v1::passport_issuance_service_client::PassportIssuanceServiceClient;
use crate::pb::are::passport::v1::GetAgentPassportRequest;
use crate::service::{PassportClient, ScopeError};
use crate::types::{Passport, ScopeEntry};

/// Fetches agent passports from ARE passport issuance gRPC (are.passport.v1).
#[derive(Clone)]
pub struct GrpcPassportClient {
    inner: PassportIssuanceServiceClient<tonic::transport::Channel>,
}

impl GrpcPassportClient {
    pub async fn connect(addr: impl Into<String>) -> Result<Self, tonic::transport::Error> {
        let inner = PassportIssuanceServiceClient::connect(addr.into()).await?;
        Ok(Self { inner })
    }
}

#[async_trait]
impl PassportClient for GrpcPassportClient {
    async fn get_agent_passport(&self, agent_id: &str) -> Result<Passport, ScopeError> {
        let req = Request::new(GetAgentPassportRequest {
            agent_id: agent_id.to_string(),
        });
        let resp = self.inner.clone().get_agent_passport(req).await.map_err(|e| {
            if e.code() == Code::NotFound {
                ScopeError::NotFound
            } else {
                ScopeError::Unavailable
            }
        })?;
        let p = resp.into_inner().passport.ok_or(ScopeError::NotFound)?;
        passport_from_proto(p)
    }
}

fn passport_from_proto(
    p: crate::pb::are::passport::v1::Passport,
) -> Result<Passport, ScopeError> {
    let mut scopes = Vec::with_capacity(p.scope_set.len());
    for s in p.scope_set.into_iter() {
        let mut cc = s.context_constraints;
        if cc.is_empty() {
            cc = HashMap::new();
        }
        scopes.push(ScopeEntry {
            scope_id: s.scope_id,
            action_class: s.action_class,
            resource_pattern: s.resource_pattern,
            context_constraints: cc,
            expires_ts: s.expires_ts,
            is_escalation: s.is_escalation,
        });
    }
    Ok(Passport {
        passport_id: p.passport_id,
        passport_type: p.passport_type,
        expires_ts: p.expires_ts,
        scopes,
    })
}
