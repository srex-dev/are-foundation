use std::str::FromStr;
use std::sync::Arc;

use tonic::{Request, Response, Status};
use uuid::Uuid;

use crate::repository::LedgerRepository;
use crate::service::{
    EventPublisher, ImmutableLedgerService, QueryEntriesInput, ServiceError, WriteEntryInput,
};

pub mod pb {
    tonic::include_proto!("are.ledger.v1");
}

#[derive(Clone)]
pub struct ImmutableLedgerGrpc<R: LedgerRepository, P: EventPublisher> {
    service: Arc<ImmutableLedgerService<R, P>>,
}

impl<R: LedgerRepository, P: EventPublisher> ImmutableLedgerGrpc<R, P> {
    pub fn new(service: Arc<ImmutableLedgerService<R, P>>) -> Self {
        Self { service }
    }
}

#[tonic::async_trait]
impl<R: LedgerRepository + 'static, P: EventPublisher + 'static>
    pb::immutable_ledger_service_server::ImmutableLedgerService for ImmutableLedgerGrpc<R, P>
{
    async fn write_entry(
        &self,
        request: Request<pb::WriteEntryRequest>,
    ) -> Result<Response<pb::WriteEntryResponse>, Status> {
        let input = request.into_inner();
        let output = self
            .service
            .write_entry(WriteEntryInput {
                entry_type: input.entry_type,
                agent_id: input.agent_id,
                content: input.content,
                content_type: input.content_type,
                source_id: input.source_id,
                correlation_id: empty_to_none(input.correlation_id),
                idempotency_key: empty_to_none(input.idempotency_key),
            })
            .await
            .map_err(map_err)?;
        Ok(Response::new(pb::WriteEntryResponse {
            entry_id: output.entry_id.to_string(),
            entry_hash: output.entry_hash,
            chain_position: output.chain_position.to_string(),
            written_ts: output.written_ts_ms,
        }))
    }

    async fn get_entry(
        &self,
        request: Request<pb::GetEntryRequest>,
    ) -> Result<Response<pb::GetEntryResponse>, Status> {
        let entry_id = parse_uuid(&request.get_ref().entry_id)?;
        let entry = self.service.get_entry(entry_id).await.map_err(map_err)?;
        Ok(Response::new(pb::GetEntryResponse {
            entry: Some(pb::LedgerEntry {
                entry_id: entry.entry_id.to_string(),
                entry_type: entry.entry_type,
                agent_id: entry.agent_id,
                content: entry.content,
                content_type: entry.content_type,
                source_id: entry.source_id,
                correlation_id: entry.correlation_id.unwrap_or_default(),
                entry_hash: entry.entry_hash,
                previous_hash: entry.previous_hash,
                chain_position: entry.chain_position,
                written_ts: entry.written_ts.timestamp_millis(),
            }),
        }))
    }

    async fn query_entries(
        &self,
        request: Request<pb::QueryEntriesRequest>,
    ) -> Result<Response<pb::QueryEntriesResponse>, Status> {
        let input = request.into_inner();
        let (entries, next_token, total_count) = self
            .service
            .query_entries(QueryEntriesInput {
                entry_type: empty_to_none(input.entry_type),
                agent_id: empty_to_none(input.agent_id),
                source_id: empty_to_none(input.source_id),
                correlation_id: empty_to_none(input.correlation_id),
                from_ts: if input.from_ts == 0 {
                    None
                } else {
                    Some(input.from_ts)
                },
                to_ts: if input.to_ts == 0 {
                    None
                } else {
                    Some(input.to_ts)
                },
                page_size: input.page_size,
                page_token: empty_to_none(input.page_token),
            })
            .await
            .map_err(map_err)?;
        Ok(Response::new(pb::QueryEntriesResponse {
            entries: entries
                .into_iter()
                .map(|entry| pb::LedgerEntry {
                    entry_id: entry.entry_id.to_string(),
                    entry_type: entry.entry_type,
                    agent_id: entry.agent_id,
                    content: entry.content,
                    content_type: entry.content_type,
                    source_id: entry.source_id,
                    correlation_id: entry.correlation_id.unwrap_or_default(),
                    entry_hash: entry.entry_hash,
                    previous_hash: entry.previous_hash,
                    chain_position: entry.chain_position,
                    written_ts: entry.written_ts.timestamp_millis(),
                })
                .collect(),
            next_page_token: next_token.unwrap_or_default(),
            total_count,
        }))
    }

    async fn verify_entry(
        &self,
        request: Request<pb::VerifyEntryRequest>,
    ) -> Result<Response<pb::VerifyEntryResponse>, Status> {
        let entry_id = parse_uuid(&request.get_ref().entry_id)?;
        let output = self.service.verify_entry(entry_id).await.map_err(map_err)?;
        Ok(Response::new(pb::VerifyEntryResponse {
            entry_id: output.entry_id.to_string(),
            hash_valid: output.hash_valid,
            chain_link_valid: output.chain_link_valid,
            failure_reason: output.failure_reason,
        }))
    }

    async fn verify_chain(
        &self,
        request: Request<pb::VerifyChainRequest>,
    ) -> Result<Response<pb::VerifyChainResponse>, Status> {
        let input = request.into_inner();
        if input.entry_type.is_empty() {
            return Err(Status::invalid_argument("entry_type is required"));
        }
        let output = self
            .service
            .verify_chain(
                &input.entry_type,
                if input.start_entry_id.is_empty() {
                    None
                } else {
                    Some(parse_uuid(&input.start_entry_id)?)
                },
                if input.end_entry_id.is_empty() {
                    None
                } else {
                    Some(parse_uuid(&input.end_entry_id)?)
                },
            )
            .await
            .map_err(map_err)?;
        Ok(Response::new(pb::VerifyChainResponse {
            chain_valid: output.chain_valid,
            entries_checked: output.entries_checked,
            first_invalid_entry_id: output
                .first_invalid_entry_id
                .map(|id| id.to_string())
                .unwrap_or_default(),
            failure_reason: output.failure_reason,
        }))
    }

    async fn get_chain_tip(
        &self,
        request: Request<pb::GetChainTipRequest>,
    ) -> Result<Response<pb::GetChainTipResponse>, Status> {
        let entry_type = request.get_ref().entry_type.clone();
        if entry_type.is_empty() {
            return Err(Status::invalid_argument("entry_type is required"));
        }
        let tip = self
            .service
            .get_chain_tip(&entry_type)
            .await
            .map_err(map_err)?;
        Ok(Response::new(pb::GetChainTipResponse {
            entry_id: tip.entry_id.to_string(),
            entry_hash: tip.hash,
            chain_position: tip.position,
            written_ts: tip.written_ts.timestamp_millis(),
        }))
    }
}

fn parse_uuid(raw: &str) -> Result<Uuid, Status> {
    Uuid::from_str(raw).map_err(|_| Status::invalid_argument("must be valid UUID"))
}

fn map_err(err: ServiceError) -> Status {
    match err {
        ServiceError::NotFound => Status::not_found("not found"),
        ServiceError::InvalidArgument(message) => Status::invalid_argument(message),
        ServiceError::Unavailable => Status::unavailable("unavailable"),
        ServiceError::Internal(message) => Status::internal(message),
    }
}

fn empty_to_none(value: String) -> Option<String> {
    if value.is_empty() {
        None
    } else {
        Some(value)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::AppConfig;
    use crate::repository::InMemoryLedgerRepository;
    use crate::service::NoopEventPublisher;

    fn config() -> Arc<AppConfig> {
        Arc::new(AppConfig {
            grpc_port: 9092,
            health_port: 8080,
            metrics_port: 8083,
            max_content_size_bytes: 1_048_576,
            db_connection_string: "postgres://local/test".to_string(),
            read_replica_connection_string: None,
            kafka_bootstrap_servers: "localhost:9092".to_string(),
            kafka_sasl_username: "user".to_string(),
            kafka_sasl_password: "pass".to_string(),
            genesis_hash_input: "ARE_LEDGER_GENESIS".to_string(),
        })
    }

    fn grpc_service() -> ImmutableLedgerGrpc<InMemoryLedgerRepository, NoopEventPublisher> {
        let service = Arc::new(ImmutableLedgerService::new(
            Arc::new(InMemoryLedgerRepository::default()),
            Arc::new(NoopEventPublisher),
            config(),
        ));
        ImmutableLedgerGrpc::new(service)
    }

    #[tokio::test]
    async fn helper_functions_cover_error_mappings() {
        assert!(parse_uuid("not-a-uuid").is_err());
        assert!(empty_to_none(String::new()).is_none());
        assert!(empty_to_none("x".to_string()).is_some());
        assert_eq!(
            map_err(ServiceError::NotFound).code(),
            tonic::Code::NotFound
        );
        assert_eq!(
            map_err(ServiceError::InvalidArgument("x".to_string())).code(),
            tonic::Code::InvalidArgument
        );
        assert_eq!(
            map_err(ServiceError::Unavailable).code(),
            tonic::Code::Unavailable
        );
        assert_eq!(
            map_err(ServiceError::Internal("x".to_string())).code(),
            tonic::Code::Internal
        );
    }

    #[tokio::test]
    async fn grpc_methods_cover_success_and_invalid_paths() {
        let grpc = grpc_service();

        let write = pb::immutable_ledger_service_server::ImmutableLedgerService::write_entry(
            &grpc,
            Request::new(pb::WriteEntryRequest {
                entry_type: "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
                agent_id: "agent-1".to_string(),
                content: b"payload".to_vec(),
                content_type: "application/json".to_string(),
                source_id: "ARE-FOUNDATION-PROOF".to_string(),
                correlation_id: String::new(),
                idempotency_key: "grpc-unit-1".to_string(),
            }),
        )
        .await
        .expect("write")
        .into_inner();
        assert!(!write.entry_id.is_empty());

        let _ = pb::immutable_ledger_service_server::ImmutableLedgerService::get_entry(
            &grpc,
            Request::new(pb::GetEntryRequest {
                entry_id: write.entry_id.clone(),
            }),
        )
        .await
        .expect("get");

        let _ = pb::immutable_ledger_service_server::ImmutableLedgerService::query_entries(
            &grpc,
            Request::new(pb::QueryEntriesRequest {
                entry_type: "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
                agent_id: String::new(),
                source_id: String::new(),
                correlation_id: String::new(),
                from_ts: 0,
                to_ts: 0,
                page_size: 10,
                page_token: String::new(),
            }),
        )
        .await
        .expect("query");

        let _ = pb::immutable_ledger_service_server::ImmutableLedgerService::verify_entry(
            &grpc,
            Request::new(pb::VerifyEntryRequest {
                entry_id: write.entry_id.clone(),
            }),
        )
        .await
        .expect("verify");

        let _ = pb::immutable_ledger_service_server::ImmutableLedgerService::verify_chain(
            &grpc,
            Request::new(pb::VerifyChainRequest {
                entry_type: "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
                start_entry_id: String::new(),
                end_entry_id: String::new(),
            }),
        )
        .await
        .expect("verify chain");

        let _ = pb::immutable_ledger_service_server::ImmutableLedgerService::get_chain_tip(
            &grpc,
            Request::new(pb::GetChainTipRequest {
                entry_type: "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
            }),
        )
        .await
        .expect("tip");

        let bad_uuid = pb::immutable_ledger_service_server::ImmutableLedgerService::get_entry(
            &grpc,
            Request::new(pb::GetEntryRequest {
                entry_id: "bad-uuid".to_string(),
            }),
        )
        .await
        .expect_err("invalid uuid");
        assert_eq!(bad_uuid.code(), tonic::Code::InvalidArgument);

        let missing_type =
            pb::immutable_ledger_service_server::ImmutableLedgerService::verify_chain(
                &grpc,
                Request::new(pb::VerifyChainRequest {
                    entry_type: String::new(),
                    start_entry_id: String::new(),
                    end_entry_id: String::new(),
                }),
            )
            .await
            .expect_err("missing type");
        assert_eq!(missing_type.code(), tonic::Code::InvalidArgument);
    }
}
