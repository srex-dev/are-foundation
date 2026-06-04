mod postgres;

pub use postgres::PostgresLedgerRepository;

use std::collections::HashMap;
use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use thiserror::Error;
use tokio::sync::RwLock;
use uuid::Uuid;

#[derive(Debug, Clone)]
pub struct LedgerEntryRecord {
    pub entry_id: Uuid,
    pub entry_type: String,
    pub agent_id: String,
    pub content: Vec<u8>,
    pub content_type: String,
    pub source_id: String,
    pub correlation_id: Option<String>,
    pub idempotency_key: Option<String>,
    pub entry_hash: String,
    pub previous_hash: String,
    pub chain_position: i64,
    pub written_ts: DateTime<Utc>,
}

#[derive(Debug, Clone)]
pub struct OutboxRecord {
    pub outbox_id: Uuid,
    pub entry_id: Uuid,
    pub entry_type: String,
    pub payload: String,
    pub status: OutboxStatus,
    pub attempt_count: i32,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum OutboxStatus {
    Pending,
    Delivered,
    Failed,
}

#[derive(Debug, Clone)]
pub struct ChainTip {
    pub entry_id: Uuid,
    pub hash: String,
    pub position: i64,
    pub written_ts: DateTime<Utc>,
}

#[derive(Debug, Error, Clone)]
pub enum RepositoryError {
    #[error("entry not found")]
    NotFound,
    #[error("repository unavailable")]
    Unavailable,
    #[error("chain integrity violation")]
    ChainIntegrityViolation,
}

#[derive(Debug, Clone)]
pub struct WriteResult {
    pub entry: LedgerEntryRecord,
    pub outbox: OutboxRecord,
}

#[derive(Debug, Clone)]
pub struct EntryWriteInput {
    pub entry_type: String,
    pub agent_id: String,
    pub content: Vec<u8>,
    pub content_type: String,
    pub source_id: String,
    pub correlation_id: Option<String>,
    pub idempotency_key: Option<String>,
    pub entry_hash: String,
    pub previous_hash: String,
    pub written_ts: DateTime<Utc>,
    pub outbox_payload: String,
}

#[async_trait]
pub trait LedgerRepository: Send + Sync {
    async fn write_entry_with_outbox(
        &self,
        input: EntryWriteInput,
    ) -> Result<WriteResult, RepositoryError>;
    async fn get_entry(&self, entry_id: Uuid) -> Result<LedgerEntryRecord, RepositoryError>;
    async fn query_entries(&self) -> Result<Vec<LedgerEntryRecord>, RepositoryError>;
    async fn get_chain_tip(&self, entry_type: &str) -> Result<ChainTip, RepositoryError>;
    async fn get_entries_by_type(
        &self,
        entry_type: &str,
    ) -> Result<Vec<LedgerEntryRecord>, RepositoryError>;
    async fn find_idempotent(
        &self,
        entry_type: &str,
        idempotency_key: &str,
    ) -> Result<Option<LedgerEntryRecord>, RepositoryError>;
    async fn pending_outbox(&self) -> Result<Vec<OutboxRecord>, RepositoryError>;
    async fn mark_outbox_delivered(&self, outbox_id: Uuid) -> Result<(), RepositoryError>;
}

#[derive(Default, Clone)]
pub struct InMemoryLedgerRepository {
    inner: Arc<RwLock<Inner>>,
}

#[derive(Default)]
struct Inner {
    entries: HashMap<Uuid, LedgerEntryRecord>,
    ordered_by_type: HashMap<String, Vec<Uuid>>,
    chain_tips: HashMap<String, ChainTip>,
    idempotency_index: HashMap<(String, String), Uuid>,
    outbox: HashMap<Uuid, OutboxRecord>,
}

#[async_trait]
impl LedgerRepository for InMemoryLedgerRepository {
    async fn write_entry_with_outbox(
        &self,
        input: EntryWriteInput,
    ) -> Result<WriteResult, RepositoryError> {
        let mut guard = self.inner.write().await;
        let next_position = guard
            .chain_tips
            .get(&input.entry_type)
            .map(|tip| tip.position + 1)
            .unwrap_or(1);

        if next_position == 1 && input.previous_hash.is_empty() {
            return Err(RepositoryError::ChainIntegrityViolation);
        }
        if next_position > 1 {
            let tip = guard
                .chain_tips
                .get(&input.entry_type)
                .ok_or(RepositoryError::ChainIntegrityViolation)?;
            if tip.hash != input.previous_hash {
                return Err(RepositoryError::ChainIntegrityViolation);
            }
        }

        let entry_id = Uuid::new_v4();
        let outbox_id = Uuid::new_v4();
        let entry = LedgerEntryRecord {
            entry_id,
            entry_type: input.entry_type.clone(),
            agent_id: input.agent_id,
            content: input.content,
            content_type: input.content_type,
            source_id: input.source_id,
            correlation_id: input.correlation_id,
            idempotency_key: input.idempotency_key.clone(),
            entry_hash: input.entry_hash.clone(),
            previous_hash: input.previous_hash,
            chain_position: next_position,
            written_ts: input.written_ts,
        };
        let outbox = OutboxRecord {
            outbox_id,
            entry_id,
            entry_type: input.entry_type.clone(),
            payload: input.outbox_payload,
            status: OutboxStatus::Pending,
            attempt_count: 0,
        };

        guard.entries.insert(entry_id, entry.clone());
        guard
            .ordered_by_type
            .entry(input.entry_type.clone())
            .or_default()
            .push(entry_id);
        guard.chain_tips.insert(
            input.entry_type,
            ChainTip {
                entry_id,
                hash: input.entry_hash,
                position: next_position,
                written_ts: entry.written_ts,
            },
        );
        if let Some(key) = input.idempotency_key {
            guard
                .idempotency_index
                .insert((entry.entry_type.clone(), key), entry_id);
        }
        guard.outbox.insert(outbox_id, outbox.clone());
        Ok(WriteResult { entry, outbox })
    }

    async fn get_entry(&self, entry_id: Uuid) -> Result<LedgerEntryRecord, RepositoryError> {
        self.inner
            .read()
            .await
            .entries
            .get(&entry_id)
            .cloned()
            .ok_or(RepositoryError::NotFound)
    }

    async fn query_entries(&self) -> Result<Vec<LedgerEntryRecord>, RepositoryError> {
        let guard = self.inner.read().await;
        let mut values: Vec<LedgerEntryRecord> = guard.entries.values().cloned().collect();
        values.sort_by_key(|entry| (entry.written_ts.timestamp_millis(), entry.chain_position));
        Ok(values)
    }

    async fn get_chain_tip(&self, entry_type: &str) -> Result<ChainTip, RepositoryError> {
        self.inner
            .read()
            .await
            .chain_tips
            .get(entry_type)
            .cloned()
            .ok_or(RepositoryError::NotFound)
    }

    async fn get_entries_by_type(
        &self,
        entry_type: &str,
    ) -> Result<Vec<LedgerEntryRecord>, RepositoryError> {
        let guard = self.inner.read().await;
        let Some(ids) = guard.ordered_by_type.get(entry_type) else {
            return Ok(Vec::new());
        };
        let mut out = Vec::with_capacity(ids.len());
        for id in ids {
            if let Some(entry) = guard.entries.get(id) {
                out.push(entry.clone());
            }
        }
        Ok(out)
    }

    async fn find_idempotent(
        &self,
        entry_type: &str,
        idempotency_key: &str,
    ) -> Result<Option<LedgerEntryRecord>, RepositoryError> {
        let guard = self.inner.read().await;
        let Some(entry_id) = guard
            .idempotency_index
            .get(&(entry_type.to_string(), idempotency_key.to_string()))
        else {
            return Ok(None);
        };
        Ok(guard.entries.get(entry_id).cloned())
    }

    async fn pending_outbox(&self) -> Result<Vec<OutboxRecord>, RepositoryError> {
        let guard = self.inner.read().await;
        Ok(guard
            .outbox
            .values()
            .filter(|record| record.status == OutboxStatus::Pending)
            .cloned()
            .collect())
    }

    async fn mark_outbox_delivered(&self, outbox_id: Uuid) -> Result<(), RepositoryError> {
        let mut guard = self.inner.write().await;
        let Some(record) = guard.outbox.get_mut(&outbox_id) else {
            return Err(RepositoryError::NotFound);
        };
        record.status = OutboxStatus::Delivered;
        Ok(())
    }
}

#[cfg(test)]
impl InMemoryLedgerRepository {
    /// Corrupts a stored entry hash (tests only).
    pub async fn test_corrupt_entry_hash(&self, entry_id: Uuid) -> bool {
        let mut g = self.inner.write().await;
        if let Some(e) = g.entries.get_mut(&entry_id) {
            e.entry_hash = "tampered-hash".to_string();
            return true;
        }
        false
    }
}

#[cfg(test)]
mod tests {
    use chrono::Utc;

    use super::*;

    #[tokio::test]
    async fn writes_and_reads_entry_with_tip_and_outbox() {
        let repo = InMemoryLedgerRepository::default();
        let result = repo
            .write_entry_with_outbox(EntryWriteInput {
                entry_type: "TYPE".to_string(),
                agent_id: "agent".to_string(),
                content: b"payload".to_vec(),
                content_type: "application/json".to_string(),
                source_id: "source".to_string(),
                correlation_id: None,
                idempotency_key: Some("idem".to_string()),
                entry_hash: "hash-1".to_string(),
                previous_hash: "genesis".to_string(),
                written_ts: Utc::now(),
                outbox_payload: "{\"x\":1}".to_string(),
            })
            .await
            .expect("write");
        let fetched = repo.get_entry(result.entry.entry_id).await.expect("get");
        assert_eq!(fetched.entry_hash, "hash-1");
        let tip = repo.get_chain_tip("TYPE").await.expect("tip");
        assert_eq!(tip.position, 1);
        let pending = repo.pending_outbox().await.expect("outbox");
        assert_eq!(pending.len(), 1);
        repo.mark_outbox_delivered(result.outbox.outbox_id)
            .await
            .expect("mark delivered");
        let pending_after = repo.pending_outbox().await.expect("outbox");
        assert!(pending_after.is_empty());
    }

    #[tokio::test]
    async fn rejects_chain_integrity_violation() {
        let repo = InMemoryLedgerRepository::default();
        let _ = repo
            .write_entry_with_outbox(EntryWriteInput {
                entry_type: "TYPE".to_string(),
                agent_id: "agent".to_string(),
                content: b"payload".to_vec(),
                content_type: "application/json".to_string(),
                source_id: "source".to_string(),
                correlation_id: None,
                idempotency_key: None,
                entry_hash: "hash-1".to_string(),
                previous_hash: "genesis".to_string(),
                written_ts: Utc::now(),
                outbox_payload: "{}".to_string(),
            })
            .await
            .expect("first");
        let err = repo
            .write_entry_with_outbox(EntryWriteInput {
                entry_type: "TYPE".to_string(),
                agent_id: "agent".to_string(),
                content: b"payload-2".to_vec(),
                content_type: "application/json".to_string(),
                source_id: "source".to_string(),
                correlation_id: None,
                idempotency_key: None,
                entry_hash: "hash-2".to_string(),
                previous_hash: "wrong-prev".to_string(),
                written_ts: Utc::now(),
                outbox_payload: "{}".to_string(),
            })
            .await
            .expect_err("should fail");
        assert!(matches!(err, RepositoryError::ChainIntegrityViolation));
    }
}
