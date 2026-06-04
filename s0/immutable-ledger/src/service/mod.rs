use std::sync::Arc;

use async_trait::async_trait;
use chrono::Utc;
use serde_json::json;
use thiserror::Error;
use tokio::sync::RwLock;
use tracing::warn;
use uuid::Uuid;

use crate::config::AppConfig;
use crate::crypto::{canonical_entry_hash, sha256_hex};
use crate::repository::{
    EntryWriteInput, LedgerEntryRecord, LedgerRepository, OutboxRecord, RepositoryError,
};

#[derive(Debug, Clone)]
pub struct WriteEntryInput {
    pub entry_type: String,
    pub agent_id: String,
    pub content: Vec<u8>,
    pub content_type: String,
    pub source_id: String,
    pub correlation_id: Option<String>,
    pub idempotency_key: Option<String>,
}

#[derive(Debug, Clone)]
pub struct WriteEntryOutput {
    pub entry_id: Uuid,
    pub entry_hash: String,
    pub chain_position: i64,
    pub written_ts_ms: i64,
}

#[derive(Debug, Clone)]
pub struct VerifyEntryOutput {
    pub entry_id: Uuid,
    pub hash_valid: bool,
    pub chain_link_valid: bool,
    pub failure_reason: String,
}

#[derive(Debug, Clone)]
pub struct VerifyChainOutput {
    pub chain_valid: bool,
    pub entries_checked: i64,
    pub first_invalid_entry_id: Option<Uuid>,
    pub failure_reason: String,
}

#[derive(Debug, Clone, Default)]
pub struct QueryEntriesInput {
    pub entry_type: Option<String>,
    pub agent_id: Option<String>,
    pub source_id: Option<String>,
    pub correlation_id: Option<String>,
    pub from_ts: Option<i64>,
    pub to_ts: Option<i64>,
    pub page_size: i32,
    pub page_token: Option<String>,
}

#[derive(Debug, Error)]
pub enum ServiceError {
    #[error("not found")]
    NotFound,
    #[error("invalid argument: {0}")]
    InvalidArgument(String),
    #[error("unavailable")]
    Unavailable,
    #[error("internal: {0}")]
    Internal(String),
}

#[async_trait]
pub trait EventPublisher: Send + Sync {
    async fn publish(&self, key: &str, payload: &str) -> Result<(), String>;
}

#[derive(Default, Clone)]
pub struct NoopEventPublisher;

#[async_trait]
impl EventPublisher for NoopEventPublisher {
    async fn publish(&self, _key: &str, _payload: &str) -> Result<(), String> {
        Ok(())
    }
}

#[derive(Clone)]
pub struct ImmutableLedgerService<R: LedgerRepository, P: EventPublisher> {
    repo: Arc<R>,
    publisher: Arc<P>,
    config: Arc<AppConfig>,
    chain_halted: Arc<RwLock<std::collections::HashSet<String>>>,
}

impl<R: LedgerRepository + 'static, P: EventPublisher + 'static> ImmutableLedgerService<R, P> {
    pub fn new(repo: Arc<R>, publisher: Arc<P>, config: Arc<AppConfig>) -> Self {
        let service = Self {
            repo,
            publisher,
            config,
            chain_halted: Arc::new(RwLock::new(std::collections::HashSet::new())),
        };
        service.start_outbox_processor();
        service
    }

    fn start_outbox_processor(&self) {
        let repo = Arc::clone(&self.repo);
        let publisher = Arc::clone(&self.publisher);
        tokio::spawn(async move {
            loop {
                let pending = match repo.pending_outbox().await {
                    Ok(v) => v,
                    Err(_) => {
                        tokio::time::sleep(std::time::Duration::from_millis(500)).await;
                        continue;
                    }
                };
                for record in pending {
                    if publish_one(&*publisher, &repo, record).await.is_err() {
                        crate::metrics::OUTBOX_PUBLISH_FAILURE_TOTAL.inc();
                        warn!("failed to publish outbox record");
                    }
                }
                tokio::time::sleep(std::time::Duration::from_millis(500)).await;
            }
        });
    }

    pub async fn write_entry(
        &self,
        input: WriteEntryInput,
    ) -> Result<WriteEntryOutput, ServiceError> {
        let out = self.write_entry_impl(input).await;
        let label: &'static str = match &out {
            Ok(_) => "ok",
            Err(ServiceError::InvalidArgument(_)) => "invalid",
            Err(_) => "error",
        };
        crate::metrics::inc_write(label);
        out
    }

    async fn write_entry_impl(
        &self,
        input: WriteEntryInput,
    ) -> Result<WriteEntryOutput, ServiceError> {
        if input.entry_type.is_empty()
            || input.agent_id.is_empty()
            || input.content_type.is_empty()
            || input.source_id.is_empty()
        {
            return Err(ServiceError::InvalidArgument(
                "entry_type, agent_id, content_type and source_id are required".to_string(),
            ));
        }
        if input.content.len() > self.config.max_content_size_bytes {
            return Err(ServiceError::InvalidArgument(format!(
                "content exceeds max {} bytes",
                self.config.max_content_size_bytes
            )));
        }
        if self.chain_halted.read().await.contains(&input.entry_type) {
            return Err(ServiceError::Unavailable);
        }
        if let Some(key) = &input.idempotency_key {
            if let Some(existing) = self
                .repo
                .find_idempotent(&input.entry_type, key)
                .await
                .map_err(map_repo)?
            {
                return Ok(WriteEntryOutput {
                    entry_id: existing.entry_id,
                    entry_hash: existing.entry_hash,
                    chain_position: existing.chain_position,
                    written_ts_ms: existing.written_ts.timestamp_millis(),
                });
            }
        }

        for attempt in 0..5 {
            let previous_hash = match self.repo.get_chain_tip(&input.entry_type).await {
                Ok(tip) => tip.hash,
                Err(RepositoryError::NotFound) => {
                    sha256_hex(self.config.genesis_hash_input.as_bytes())
                }
                Err(err) => return Err(map_repo(err)),
            };
            let written_ts = Utc::now();
            let entry_hash = canonical_entry_hash(
                &input.entry_type,
                &input.agent_id,
                &input.content,
                &input.content_type,
                &input.source_id,
                written_ts.timestamp_millis(),
                &previous_hash,
            );
            let outbox_payload = json!({
                "event_id": Uuid::new_v4().to_string(),
                "event_type": "LEDGER_ENTRY_WRITTEN",
                "entry_type": input.entry_type,
                "agent_id": input.agent_id,
                "source_id": input.source_id,
                "entry_hash": entry_hash,
                "correlation_id": input.correlation_id,
                "written_ts": written_ts.timestamp_millis(),
                "schema_version": "1.0.0"
            })
            .to_string();

            match self
                .repo
                .write_entry_with_outbox(EntryWriteInput {
                    entry_type: input.entry_type.clone(),
                    agent_id: input.agent_id.clone(),
                    content: input.content.clone(),
                    content_type: input.content_type.clone(),
                    source_id: input.source_id.clone(),
                    correlation_id: input.correlation_id.clone(),
                    idempotency_key: input.idempotency_key.clone(),
                    entry_hash: entry_hash.clone(),
                    previous_hash,
                    written_ts,
                    outbox_payload,
                })
                .await
            {
                Ok(result) => {
                    return Ok(WriteEntryOutput {
                        entry_id: result.entry.entry_id,
                        entry_hash: result.entry.entry_hash,
                        chain_position: result.entry.chain_position,
                        written_ts_ms: result.entry.written_ts.timestamp_millis(),
                    });
                }
                Err(RepositoryError::ChainIntegrityViolation) if attempt < 4 => {
                    continue;
                }
                Err(RepositoryError::ChainIntegrityViolation) => {
                    // Repeated mismatches indicate potential corruption, halt writes for this type.
                    self.chain_halted
                        .write()
                        .await
                        .insert(input.entry_type.clone());
                    return Err(ServiceError::Internal(
                        "chain integrity violation".to_string(),
                    ));
                }
                Err(err) => return Err(map_repo(err)),
            }
        }
        Err(ServiceError::Internal(
            "chain integrity violation".to_string(),
        ))
    }

    pub async fn get_entry(&self, entry_id: Uuid) -> Result<LedgerEntryRecord, ServiceError> {
        self.repo.get_entry(entry_id).await.map_err(map_repo)
    }

    pub async fn query_entries(
        &self,
        query: QueryEntriesInput,
    ) -> Result<(Vec<LedgerEntryRecord>, Option<String>, i64), ServiceError> {
        let mut entries = self.repo.query_entries().await.map_err(map_repo)?;
        if let Some(filter) = query.entry_type.as_deref() {
            entries.retain(|entry| entry.entry_type == filter);
        }
        if let Some(filter) = query.agent_id.as_deref() {
            entries.retain(|entry| entry.agent_id == filter);
        }
        if let Some(filter) = query.source_id.as_deref() {
            entries.retain(|entry| entry.source_id == filter);
        }
        if let Some(filter) = query.correlation_id.as_deref() {
            entries.retain(|entry| entry.correlation_id.as_deref() == Some(filter));
        }
        if let Some(from) = query.from_ts {
            entries.retain(|entry| entry.written_ts.timestamp_millis() >= from);
        }
        if let Some(to) = query.to_ts {
            entries.retain(|entry| entry.written_ts.timestamp_millis() <= to);
        }
        entries.sort_by_key(|entry| (entry.written_ts.timestamp_millis(), entry.chain_position));
        let total = entries.len() as i64;

        let size = if query.page_size <= 0 {
            100
        } else if query.page_size > 1000 {
            1000
        } else {
            query.page_size
        } as usize;
        let offset = query
            .page_token
            .as_deref()
            .and_then(|raw| raw.parse::<usize>().ok())
            .unwrap_or(0);
        let page_entries = entries
            .into_iter()
            .skip(offset)
            .take(size)
            .collect::<Vec<_>>();
        let next_offset = offset + page_entries.len();
        let next_token = if next_offset < total as usize {
            Some(next_offset.to_string())
        } else {
            None
        };
        Ok((page_entries, next_token, total))
    }

    pub async fn verify_entry(&self, entry_id: Uuid) -> Result<VerifyEntryOutput, ServiceError> {
        let entry = self.repo.get_entry(entry_id).await.map_err(map_repo)?;
        let recomputed = canonical_entry_hash(
            &entry.entry_type,
            &entry.agent_id,
            &entry.content,
            &entry.content_type,
            &entry.source_id,
            entry.written_ts.timestamp_millis(),
            &entry.previous_hash,
        );
        let hash_valid = recomputed == entry.entry_hash;
        let chain_link_valid = if entry.chain_position == 1 {
            entry.previous_hash == sha256_hex(self.config.genesis_hash_input.as_bytes())
        } else {
            let chain = self
                .repo
                .get_entries_by_type(&entry.entry_type)
                .await
                .map_err(map_repo)?;
            let previous = chain
                .iter()
                .find(|candidate| candidate.chain_position == entry.chain_position - 1);
            match previous {
                Some(prev) => prev.entry_hash == entry.previous_hash,
                None => false,
            }
        };
        let failure_reason = if hash_valid && chain_link_valid {
            String::new()
        } else if !hash_valid {
            "entry_hash_mismatch".to_string()
        } else {
            "chain_link_mismatch".to_string()
        };

        Ok(VerifyEntryOutput {
            entry_id,
            hash_valid,
            chain_link_valid,
            failure_reason,
        })
    }

    pub async fn verify_chain(
        &self,
        entry_type: &str,
        start_entry_id: Option<Uuid>,
        end_entry_id: Option<Uuid>,
    ) -> Result<VerifyChainOutput, ServiceError> {
        let entries = self
            .repo
            .get_entries_by_type(entry_type)
            .await
            .map_err(map_repo)?;
        if entries.is_empty() {
            return Ok(VerifyChainOutput {
                chain_valid: true,
                entries_checked: 0,
                first_invalid_entry_id: None,
                failure_reason: String::new(),
            });
        }
        let start_position = if let Some(id) = start_entry_id {
            entries
                .iter()
                .find(|entry| entry.entry_id == id)
                .map(|entry| entry.chain_position)
                .ok_or(ServiceError::NotFound)?
        } else {
            1
        };
        let end_position = if let Some(id) = end_entry_id {
            entries
                .iter()
                .find(|entry| entry.entry_id == id)
                .map(|entry| entry.chain_position)
                .ok_or(ServiceError::NotFound)?
        } else {
            entries
                .iter()
                .map(|entry| entry.chain_position)
                .max()
                .unwrap_or(0)
        };
        let mut filtered = entries
            .into_iter()
            .filter(|entry| {
                entry.chain_position >= start_position && entry.chain_position <= end_position
            })
            .collect::<Vec<_>>();
        filtered.sort_by_key(|entry| entry.chain_position);
        for (idx, entry) in filtered.iter().enumerate() {
            let prev_hash = if idx == 0 && entry.chain_position == 1 {
                sha256_hex(self.config.genesis_hash_input.as_bytes())
            } else if idx > 0 {
                filtered[idx - 1].entry_hash.clone()
            } else {
                entry.previous_hash.clone()
            };
            if entry.previous_hash != prev_hash {
                crate::metrics::LEDGER_CHAIN_VERIFY_FAILURE_TOTAL.inc();
                return Ok(VerifyChainOutput {
                    chain_valid: false,
                    entries_checked: idx as i64 + 1,
                    first_invalid_entry_id: Some(entry.entry_id),
                    failure_reason: "chain_link_mismatch".to_string(),
                });
            }
            let recomputed = canonical_entry_hash(
                &entry.entry_type,
                &entry.agent_id,
                &entry.content,
                &entry.content_type,
                &entry.source_id,
                entry.written_ts.timestamp_millis(),
                &entry.previous_hash,
            );
            if entry.entry_hash != recomputed {
                crate::metrics::LEDGER_CHAIN_VERIFY_FAILURE_TOTAL.inc();
                return Ok(VerifyChainOutput {
                    chain_valid: false,
                    entries_checked: idx as i64 + 1,
                    first_invalid_entry_id: Some(entry.entry_id),
                    failure_reason: "entry_hash_mismatch".to_string(),
                });
            }
        }
        Ok(VerifyChainOutput {
            chain_valid: true,
            entries_checked: filtered.len() as i64,
            first_invalid_entry_id: None,
            failure_reason: String::new(),
        })
    }

    pub async fn get_chain_tip(
        &self,
        entry_type: &str,
    ) -> Result<crate::repository::ChainTip, ServiceError> {
        self.repo.get_chain_tip(entry_type).await.map_err(map_repo)
    }
}

async fn publish_one<R: LedgerRepository, P: EventPublisher>(
    publisher: &P,
    repo: &Arc<R>,
    record: OutboxRecord,
) -> Result<(), ServiceError> {
    publisher
        .publish(&record.entry_type, &record.payload)
        .await
        .map_err(ServiceError::Internal)?;
    repo.mark_outbox_delivered(record.outbox_id)
        .await
        .map_err(map_repo)?;
    Ok(())
}

fn map_repo(err: RepositoryError) -> ServiceError {
    match err {
        RepositoryError::NotFound => ServiceError::NotFound,
        RepositoryError::Unavailable => ServiceError::Unavailable,
        RepositoryError::ChainIntegrityViolation => {
            ServiceError::Internal("chain integrity violation".to_string())
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::metrics;
    use crate::repository::InMemoryLedgerRepository;
    use crate::repository::{ChainTip, EntryWriteInput, LedgerEntryRecord, WriteResult};
    use async_trait::async_trait;
    use chrono::Utc;
    use std::collections::HashMap;
    use tokio::sync::Mutex;
    use uuid::Uuid;

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

    fn service() -> ImmutableLedgerService<InMemoryLedgerRepository, NoopEventPublisher> {
        ImmutableLedgerService::new(
            Arc::new(InMemoryLedgerRepository::default()),
            Arc::new(NoopEventPublisher),
            config(),
        )
    }

    #[tokio::test]
    async fn write_then_get_and_verify() {
        let service = service();
        let write = service
            .write_entry(WriteEntryInput {
                entry_type: "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
                agent_id: "agent-1".to_string(),
                content: b"hello".to_vec(),
                content_type: "application/json".to_string(),
                source_id: "ARE-FOUNDATION-PROOF".to_string(),
                correlation_id: None,
                idempotency_key: None,
            })
            .await
            .expect("write");
        let verify = service.verify_entry(write.entry_id).await.expect("verify");
        assert!(verify.hash_valid);
        assert!(verify.chain_link_valid);
    }

    #[tokio::test]
    async fn idempotency_returns_same_entry() {
        let service = service();
        let first = service
            .write_entry(WriteEntryInput {
                entry_type: "LEDGER_ENTRY_TYPE_GATE_DECISION".to_string(),
                agent_id: "agent-1".to_string(),
                content: b"one".to_vec(),
                content_type: "application/json".to_string(),
                source_id: "ARE-FOUNDATION-PROOF".to_string(),
                correlation_id: None,
                idempotency_key: Some("idem-1".to_string()),
            })
            .await
            .expect("first");
        let second = service
            .write_entry(WriteEntryInput {
                entry_type: "LEDGER_ENTRY_TYPE_GATE_DECISION".to_string(),
                agent_id: "agent-1".to_string(),
                content: b"one".to_vec(),
                content_type: "application/json".to_string(),
                source_id: "ARE-FOUNDATION-PROOF".to_string(),
                correlation_id: None,
                idempotency_key: Some("idem-1".to_string()),
            })
            .await
            .expect("second");
        assert_eq!(first.entry_id, second.entry_id);
        assert_eq!(first.entry_hash, second.entry_hash);
    }

    #[tokio::test]
    async fn max_content_size_enforced() {
        let service = service();
        let err = service
            .write_entry(WriteEntryInput {
                entry_type: "LEDGER_ENTRY_TYPE_GATE_DECISION".to_string(),
                agent_id: "agent-1".to_string(),
                content: vec![0_u8; 1_048_577],
                content_type: "application/json".to_string(),
                source_id: "ARE-FOUNDATION-PROOF".to_string(),
                correlation_id: None,
                idempotency_key: None,
            })
            .await
            .expect_err("content should be rejected");
        assert!(matches!(err, ServiceError::InvalidArgument(_)));
    }

    #[tokio::test]
    async fn chain_positions_increment_without_gaps() {
        let service = service();
        for i in 0..5 {
            let _ = service
                .write_entry(WriteEntryInput {
                    entry_type: "LEDGER_ENTRY_TYPE_POLICY_EVAL".to_string(),
                    agent_id: "agent-1".to_string(),
                    content: format!("payload-{}", i).into_bytes(),
                    content_type: "application/json".to_string(),
                    source_id: "ARE-A-S1-004".to_string(),
                    correlation_id: None,
                    idempotency_key: Some(format!("idem-{}", i)),
                })
                .await
                .expect("write");
        }
        let entries = service
            .repo
            .get_entries_by_type("LEDGER_ENTRY_TYPE_POLICY_EVAL")
            .await
            .expect("list");
        let positions: Vec<i64> = entries.iter().map(|entry| entry.chain_position).collect();
        assert_eq!(positions, vec![1, 2, 3, 4, 5]);
    }

    #[tokio::test]
    async fn concurrent_same_type_writes_succeed_without_halt() {
        let service = service();
        let mut jobs = Vec::new();
        for i in 0..200 {
            let svc = service.clone();
            jobs.push(tokio::spawn(async move {
                svc.write_entry(WriteEntryInput {
                    entry_type: "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
                    agent_id: format!("agent-{}", i),
                    content: format!("payload-{}", i).into_bytes(),
                    content_type: "application/json".to_string(),
                    source_id: "ARE-FOUNDATION-PROOF".to_string(),
                    correlation_id: None,
                    idempotency_key: Some(format!("idem-{}", i)),
                })
                .await
            }));
        }
        for job in jobs {
            let result = job.await.expect("join");
            assert!(result.is_ok());
        }
        let verify = service
            .verify_chain("LEDGER_ENTRY_TYPE_ACTION_RECEIPT", None, None)
            .await
            .expect("verify");
        assert!(verify.chain_valid);
        assert_eq!(verify.entries_checked, 200);
    }

    #[tokio::test]
    async fn query_entries_filters_and_paginates() {
        let service = service();
        for i in 0..6 {
            let _ = service
                .write_entry(WriteEntryInput {
                    entry_type: if i % 2 == 0 {
                        "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string()
                    } else {
                        "LEDGER_ENTRY_TYPE_POLICY_EVAL".to_string()
                    },
                    agent_id: "agent-x".to_string(),
                    content: format!("payload-{}", i).into_bytes(),
                    content_type: "application/json".to_string(),
                    source_id: "ARE-A-S0-003".to_string(),
                    correlation_id: if i < 3 {
                        Some("corr-a".to_string())
                    } else {
                        None
                    },
                    idempotency_key: Some(format!("page-{}", i)),
                })
                .await
                .expect("write");
        }

        let (page1, next, total) = service
            .query_entries(QueryEntriesInput {
                entry_type: Some("LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string()),
                agent_id: Some("agent-x".to_string()),
                source_id: Some("ARE-A-S0-003".to_string()),
                correlation_id: None,
                from_ts: None,
                to_ts: None,
                page_size: 2,
                page_token: None,
            })
            .await
            .expect("query");
        assert_eq!(total, 3);
        assert_eq!(page1.len(), 2);

        let (page2, _, _) = service
            .query_entries(QueryEntriesInput {
                entry_type: Some("LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string()),
                agent_id: Some("agent-x".to_string()),
                source_id: Some("ARE-A-S0-003".to_string()),
                correlation_id: None,
                from_ts: None,
                to_ts: None,
                page_size: 2,
                page_token: next,
            })
            .await
            .expect("query");
        assert_eq!(page2.len(), 1);
    }

    #[tokio::test]
    async fn verify_chain_not_found_on_invalid_bounds() {
        let service = service();
        let _ = service
            .write_entry(WriteEntryInput {
                entry_type: "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
                agent_id: "agent-1".to_string(),
                content: b"payload".to_vec(),
                content_type: "application/json".to_string(),
                source_id: "ARE-FOUNDATION-PROOF".to_string(),
                correlation_id: None,
                idempotency_key: Some("bounds-1".to_string()),
            })
            .await
            .expect("seed");
        let err = service
            .verify_chain(
                "LEDGER_ENTRY_TYPE_ACTION_RECEIPT",
                Some(Uuid::new_v4()),
                Some(Uuid::new_v4()),
            )
            .await
            .expect_err("expected not found");
        assert!(matches!(err, ServiceError::NotFound));
    }

    #[derive(Default)]
    struct StaticRepo {
        entries: Mutex<HashMap<Uuid, LedgerEntryRecord>>,
        entries_by_type: Mutex<HashMap<String, Vec<LedgerEntryRecord>>>,
    }

    #[async_trait]
    impl LedgerRepository for StaticRepo {
        async fn write_entry_with_outbox(
            &self,
            _input: EntryWriteInput,
        ) -> Result<WriteResult, RepositoryError> {
            Err(RepositoryError::Unavailable)
        }
        async fn get_entry(&self, entry_id: Uuid) -> Result<LedgerEntryRecord, RepositoryError> {
            self.entries
                .lock()
                .await
                .get(&entry_id)
                .cloned()
                .ok_or(RepositoryError::NotFound)
        }
        async fn query_entries(&self) -> Result<Vec<LedgerEntryRecord>, RepositoryError> {
            Ok(Vec::new())
        }
        async fn get_chain_tip(&self, _entry_type: &str) -> Result<ChainTip, RepositoryError> {
            Err(RepositoryError::NotFound)
        }
        async fn get_entries_by_type(
            &self,
            entry_type: &str,
        ) -> Result<Vec<LedgerEntryRecord>, RepositoryError> {
            Ok(self
                .entries_by_type
                .lock()
                .await
                .get(entry_type)
                .cloned()
                .unwrap_or_default())
        }
        async fn find_idempotent(
            &self,
            _entry_type: &str,
            _idempotency_key: &str,
        ) -> Result<Option<LedgerEntryRecord>, RepositoryError> {
            Ok(None)
        }
        async fn pending_outbox(&self) -> Result<Vec<OutboxRecord>, RepositoryError> {
            Ok(Vec::new())
        }
        async fn mark_outbox_delivered(&self, _outbox_id: Uuid) -> Result<(), RepositoryError> {
            Ok(())
        }
    }

    #[tokio::test]
    async fn verify_chain_detects_tamper() {
        let repo = Arc::new(StaticRepo::default());
        let first = LedgerEntryRecord {
            entry_id: Uuid::new_v4(),
            entry_type: "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
            agent_id: "agent-1".to_string(),
            content: b"payload-1".to_vec(),
            content_type: "application/json".to_string(),
            source_id: "ARE-FOUNDATION-PROOF".to_string(),
            correlation_id: None,
            idempotency_key: None,
            entry_hash: "bad-hash".to_string(),
            previous_hash: sha256_hex(b"ARE_LEDGER_GENESIS"),
            chain_position: 1,
            written_ts: Utc::now(),
        };
        let first_id = first.entry_id;
        repo.entries_by_type.lock().await.insert(
            "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
            vec![first.clone()],
        );
        repo.entries.lock().await.insert(first_id, first);

        let service = ImmutableLedgerService::new(repo, Arc::new(NoopEventPublisher), config());
        let result = service
            .verify_chain("LEDGER_ENTRY_TYPE_ACTION_RECEIPT", None, None)
            .await
            .expect("verify chain");
        assert!(!result.chain_valid);
    }

    struct FailingRepo {
        error: RepositoryError,
    }

    #[async_trait]
    impl LedgerRepository for FailingRepo {
        async fn write_entry_with_outbox(
            &self,
            _input: EntryWriteInput,
        ) -> Result<WriteResult, RepositoryError> {
            Err(self.error.clone())
        }
        async fn get_entry(&self, _entry_id: Uuid) -> Result<LedgerEntryRecord, RepositoryError> {
            Err(self.error.clone())
        }
        async fn query_entries(&self) -> Result<Vec<LedgerEntryRecord>, RepositoryError> {
            Err(self.error.clone())
        }
        async fn get_chain_tip(&self, _entry_type: &str) -> Result<ChainTip, RepositoryError> {
            Err(self.error.clone())
        }
        async fn get_entries_by_type(
            &self,
            _entry_type: &str,
        ) -> Result<Vec<LedgerEntryRecord>, RepositoryError> {
            Err(self.error.clone())
        }
        async fn find_idempotent(
            &self,
            _entry_type: &str,
            _idempotency_key: &str,
        ) -> Result<Option<LedgerEntryRecord>, RepositoryError> {
            Err(self.error.clone())
        }
        async fn pending_outbox(&self) -> Result<Vec<OutboxRecord>, RepositoryError> {
            Ok(Vec::new())
        }
        async fn mark_outbox_delivered(&self, _outbox_id: Uuid) -> Result<(), RepositoryError> {
            Err(self.error.clone())
        }
    }

    #[tokio::test]
    async fn maps_repository_unavailable_errors() {
        let repo = Arc::new(FailingRepo {
            error: RepositoryError::Unavailable,
        });
        let service = ImmutableLedgerService::new(repo, Arc::new(NoopEventPublisher), config());
        let err = service
            .write_entry(WriteEntryInput {
                entry_type: "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
                agent_id: "agent-1".to_string(),
                content: b"payload".to_vec(),
                content_type: "application/json".to_string(),
                source_id: "ARE-FOUNDATION-PROOF".to_string(),
                correlation_id: None,
                idempotency_key: Some("unavail".to_string()),
            })
            .await
            .expect_err("must fail");
        assert!(matches!(err, ServiceError::Unavailable));
    }

    #[tokio::test]
    async fn write_entry_rejects_missing_required_fields() {
        let service = service();
        let err = service
            .write_entry(WriteEntryInput {
                entry_type: String::new(),
                agent_id: "agent".to_string(),
                content: b"payload".to_vec(),
                content_type: "application/json".to_string(),
                source_id: "src".to_string(),
                correlation_id: None,
                idempotency_key: None,
            })
            .await
            .expect_err("missing field should fail");
        assert!(matches!(err, ServiceError::InvalidArgument(_)));
    }

    #[tokio::test]
    async fn write_entry_halts_after_repeated_chain_integrity_violation() {
        struct AlwaysChainViolationRepo;

        #[async_trait]
        impl LedgerRepository for AlwaysChainViolationRepo {
            async fn write_entry_with_outbox(
                &self,
                _input: EntryWriteInput,
            ) -> Result<WriteResult, RepositoryError> {
                Err(RepositoryError::ChainIntegrityViolation)
            }
            async fn get_entry(
                &self,
                _entry_id: Uuid,
            ) -> Result<LedgerEntryRecord, RepositoryError> {
                Err(RepositoryError::NotFound)
            }
            async fn query_entries(&self) -> Result<Vec<LedgerEntryRecord>, RepositoryError> {
                Ok(Vec::new())
            }
            async fn get_chain_tip(&self, _entry_type: &str) -> Result<ChainTip, RepositoryError> {
                Ok(ChainTip {
                    entry_id: Uuid::new_v4(),
                    hash: "tip".to_string(),
                    position: 1,
                    written_ts: Utc::now(),
                })
            }
            async fn get_entries_by_type(
                &self,
                _entry_type: &str,
            ) -> Result<Vec<LedgerEntryRecord>, RepositoryError> {
                Ok(Vec::new())
            }
            async fn find_idempotent(
                &self,
                _entry_type: &str,
                _idempotency_key: &str,
            ) -> Result<Option<LedgerEntryRecord>, RepositoryError> {
                Ok(None)
            }
            async fn pending_outbox(&self) -> Result<Vec<OutboxRecord>, RepositoryError> {
                Ok(Vec::new())
            }
            async fn mark_outbox_delivered(&self, _outbox_id: Uuid) -> Result<(), RepositoryError> {
                Ok(())
            }
        }

        let service = ImmutableLedgerService::new(
            Arc::new(AlwaysChainViolationRepo),
            Arc::new(NoopEventPublisher),
            config(),
        );
        let first = service
            .write_entry(WriteEntryInput {
                entry_type: "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
                agent_id: "agent".to_string(),
                content: b"x".to_vec(),
                content_type: "application/json".to_string(),
                source_id: "src".to_string(),
                correlation_id: None,
                idempotency_key: None,
            })
            .await;
        assert!(matches!(first, Err(ServiceError::Internal(_))));

        let second = service
            .write_entry(WriteEntryInput {
                entry_type: "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
                agent_id: "agent".to_string(),
                content: b"y".to_vec(),
                content_type: "application/json".to_string(),
                source_id: "src".to_string(),
                correlation_id: None,
                idempotency_key: None,
            })
            .await;
        assert!(matches!(second, Err(ServiceError::Unavailable)));
    }

    #[tokio::test]
    async fn verify_entry_reports_chain_mismatch_when_previous_missing() {
        let repo = Arc::new(StaticRepo::default());
        let current = LedgerEntryRecord {
            entry_id: Uuid::new_v4(),
            entry_type: "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
            agent_id: "agent-1".to_string(),
            content: b"payload".to_vec(),
            content_type: "application/json".to_string(),
            source_id: "ARE-FOUNDATION-PROOF".to_string(),
            correlation_id: None,
            idempotency_key: None,
            entry_hash: "hash".to_string(),
            previous_hash: "missing-prev".to_string(),
            chain_position: 2,
            written_ts: Utc::now(),
        };
        repo.entries_by_type
            .lock()
            .await
            .insert(current.entry_type.clone(), vec![current.clone()]);
        repo.entries
            .lock()
            .await
            .insert(current.entry_id, current.clone());

        let service = ImmutableLedgerService::new(repo, Arc::new(NoopEventPublisher), config());
        let response = service
            .verify_entry(current.entry_id)
            .await
            .expect("verify");
        assert!(!response.chain_link_valid);
    }

    #[tokio::test]
    async fn verify_chain_reports_first_invalid_entry() {
        let repo = Arc::new(StaticRepo::default());
        let entry1 = LedgerEntryRecord {
            entry_id: Uuid::new_v4(),
            entry_type: "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
            agent_id: "agent-1".to_string(),
            content: b"payload-1".to_vec(),
            content_type: "application/json".to_string(),
            source_id: "ARE-FOUNDATION-PROOF".to_string(),
            correlation_id: None,
            idempotency_key: None,
            entry_hash: canonical_entry_hash(
                "LEDGER_ENTRY_TYPE_ACTION_RECEIPT",
                "agent-1",
                b"payload-1",
                "application/json",
                "ARE-FOUNDATION-PROOF",
                1,
                &sha256_hex(b"ARE_LEDGER_GENESIS"),
            ),
            previous_hash: sha256_hex(b"ARE_LEDGER_GENESIS"),
            chain_position: 1,
            written_ts: chrono::DateTime::<Utc>::from_timestamp_millis(1).expect("ts"),
        };
        let entry2 = LedgerEntryRecord {
            entry_id: Uuid::new_v4(),
            entry_type: "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
            agent_id: "agent-1".to_string(),
            content: b"payload-2".to_vec(),
            content_type: "application/json".to_string(),
            source_id: "ARE-FOUNDATION-PROOF".to_string(),
            correlation_id: None,
            idempotency_key: None,
            entry_hash: "bad".to_string(),
            previous_hash: "wrong-prev".to_string(),
            chain_position: 2,
            written_ts: chrono::DateTime::<Utc>::from_timestamp_millis(2).expect("ts"),
        };
        repo.entries_by_type
            .lock()
            .await
            .insert(entry1.entry_type.clone(), vec![entry1, entry2.clone()]);
        repo.entries
            .lock()
            .await
            .insert(entry2.entry_id, entry2.clone());

        let service = ImmutableLedgerService::new(repo, Arc::new(NoopEventPublisher), config());
        let result = service
            .verify_chain("LEDGER_ENTRY_TYPE_ACTION_RECEIPT", None, None)
            .await
            .expect("verify");
        assert!(!result.chain_valid);
        assert_eq!(result.first_invalid_entry_id, Some(entry2.entry_id));
    }

    #[tokio::test]
    async fn verify_chain_fails_after_hash_tamper() {
        let repo = Arc::new(InMemoryLedgerRepository::default());
        let svc = ImmutableLedgerService::new(repo.clone(), Arc::new(NoopEventPublisher), config());
        let entry_type = "LEDGER_ENTRY_TYPE_TAMPER_TEST";
        let w = svc
            .write_entry(WriteEntryInput {
                entry_type: entry_type.to_string(),
                agent_id: "agent-x".to_string(),
                content: b"payload-a".to_vec(),
                content_type: "application/json".to_string(),
                source_id: "ARE-TEST".to_string(),
                correlation_id: None,
                idempotency_key: None,
            })
            .await
            .expect("write");
        assert!(
            repo.test_corrupt_entry_hash(w.entry_id).await,
            "expected corrupt helper to find entry"
        );
        let v = svc
            .verify_chain(entry_type, None, None)
            .await
            .expect("verify");
        assert!(!v.chain_valid);
        assert_eq!(v.failure_reason, "entry_hash_mismatch");
    }

    #[tokio::test]
    async fn metrics_increment_on_failed_verify_chain() {
        let before: f64 = metrics::LEDGER_CHAIN_VERIFY_FAILURE_TOTAL.get();
        let repo = Arc::new(InMemoryLedgerRepository::default());
        let svc = ImmutableLedgerService::new(repo.clone(), Arc::new(NoopEventPublisher), config());
        let entry_type = "LEDGER_ENTRY_TYPE_METRIC_TEST";
        let w = svc
            .write_entry(WriteEntryInput {
                entry_type: entry_type.to_string(),
                agent_id: "agent-y".to_string(),
                content: b"p".to_vec(),
                content_type: "application/json".to_string(),
                source_id: "ARE-TEST".to_string(),
                correlation_id: None,
                idempotency_key: None,
            })
            .await
            .expect("write");
        repo.test_corrupt_entry_hash(w.entry_id).await;
        let _ = svc
            .verify_chain(entry_type, None, None)
            .await
            .expect("verify");
        let after: f64 = metrics::LEDGER_CHAIN_VERIFY_FAILURE_TOTAL.get();
        assert!(
            after > before,
            "expected chain verify failure counter to increment"
        );
    }
}
