use std::sync::Arc;

use async_trait::async_trait;
use postgres_types::Json;
use serde_json::Value;
use tokio::sync::Mutex;
use tokio_postgres::error::SqlState;
use tokio_postgres::{Client, Row};
use uuid::Uuid;

use super::{
    ChainTip, EntryWriteInput, LedgerEntryRecord, LedgerRepository, OutboxRecord, OutboxStatus,
    RepositoryError, WriteResult,
};

/// Append-only ledger + outbox backed by PostgreSQL (`migrations/*.sql`).
pub struct PostgresLedgerRepository {
    client: Arc<Mutex<Client>>,
}

impl PostgresLedgerRepository {
    pub fn new(client: Arc<Mutex<Client>>) -> Self {
        Self { client }
    }

    fn map_row_entry(row: &Row) -> LedgerEntryRecord {
        LedgerEntryRecord {
            entry_id: row.get("entry_id"),
            entry_type: row.get("entry_type"),
            agent_id: row.get("agent_id"),
            content: row.get("content"),
            content_type: row.get("content_type"),
            source_id: row.get("source_id"),
            correlation_id: row.get("correlation_id"),
            idempotency_key: row.get("idempotency_key"),
            entry_hash: row.get("entry_hash"),
            previous_hash: row.get("previous_hash"),
            chain_position: row.get("chain_position"),
            written_ts: row.get("written_ts"),
        }
    }

    fn map_outbox_row(row: &Row) -> OutboxRecord {
        let status_raw: String = row.get("status");
        let status = match status_raw.to_ascii_uppercase().as_str() {
            "PENDING" => OutboxStatus::Pending,
            "DELIVERED" => OutboxStatus::Delivered,
            _ => OutboxStatus::Failed,
        };
        let payload_val: Value = row.get("payload");
        OutboxRecord {
            outbox_id: row.get("outbox_id"),
            entry_id: row.get("entry_id"),
            entry_type: row.get("entry_type"),
            payload: payload_val.to_string(),
            status,
            attempt_count: row.get("attempt_count"),
        }
    }
}

#[async_trait]
impl LedgerRepository for PostgresLedgerRepository {
    async fn write_entry_with_outbox(
        &self,
        input: EntryWriteInput,
    ) -> Result<WriteResult, RepositoryError> {
        let payload_json: Json<Value> = Json(
            serde_json::from_str::<Value>(&input.outbox_payload)
                .map_err(|_| RepositoryError::Unavailable)?,
        );

        let mut client = self.client.lock().await;
        let tx = client
            .transaction()
            .await
            .map_err(|_| RepositoryError::Unavailable)?;

        tx.execute(
            "SELECT pg_advisory_xact_lock(abs(hashtext($1::text))::bigint)",
            &[&input.entry_type],
        )
        .await
        .map_err(|_| RepositoryError::Unavailable)?;

        let tip_row = tx
            .query_opt(
                "SELECT entry_hash, chain_position
                 FROM are_ledger.ledger_entries
                 WHERE entry_type = $1
                 ORDER BY chain_position DESC
                 LIMIT 1",
                &[&input.entry_type],
            )
            .await
            .map_err(|_| RepositoryError::Unavailable)?;

        let next_position = match &tip_row {
            None => {
                if input.previous_hash.is_empty() {
                    let _ = tx.rollback().await;
                    return Err(RepositoryError::ChainIntegrityViolation);
                }
                1_i64
            }
            Some(row) => {
                let tip_hash: String = row.get("entry_hash");
                if tip_hash != input.previous_hash {
                    let _ = tx.rollback().await;
                    return Err(RepositoryError::ChainIntegrityViolation);
                }
                let pos: i64 = row.get("chain_position");
                pos + 1
            }
        };

        let entry_id = Uuid::new_v4();
        let outbox_id = Uuid::new_v4();
        let written_ts = input.written_ts;

        if let Err(e) = tx
            .execute(
                "INSERT INTO are_ledger.ledger_entries (
                    entry_id, entry_type, agent_id, content, content_type, source_id,
                    correlation_id, idempotency_key, entry_hash, previous_hash, chain_position, written_ts
                ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)",
                &[
                    &entry_id,
                    &input.entry_type,
                    &input.agent_id,
                    &input.content,
                    &input.content_type,
                    &input.source_id,
                    &input.correlation_id,
                    &input.idempotency_key,
                    &input.entry_hash,
                    &input.previous_hash,
                    &next_position,
                    &written_ts,
                ],
            )
            .await
        {
            let _ = tx.rollback().await;
            if let Some(db) = e.as_db_error() {
                if db.code() == &SqlState::UNIQUE_VIOLATION {
                    return Err(RepositoryError::ChainIntegrityViolation);
                }
            }
            return Err(RepositoryError::Unavailable);
        }

        if let Err(e) = tx
            .execute(
                "INSERT INTO are_ledger.ledger_write_outbox (
                    outbox_id, entry_id, entry_type, payload, status, attempt_count
                ) VALUES ($1, $2, $3, $4, 'PENDING', 0)",
                &[&outbox_id, &entry_id, &input.entry_type, &payload_json],
            )
            .await
        {
            let _ = tx.rollback().await;
            let _ = e;
            return Err(RepositoryError::Unavailable);
        }

        tx.commit()
            .await
            .map_err(|_| RepositoryError::Unavailable)?;

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
            written_ts,
        };
        let outbox = OutboxRecord {
            outbox_id,
            entry_id,
            entry_type: input.entry_type,
            payload: input.outbox_payload,
            status: OutboxStatus::Pending,
            attempt_count: 0,
        };
        Ok(WriteResult { entry, outbox })
    }

    async fn get_entry(&self, entry_id: Uuid) -> Result<LedgerEntryRecord, RepositoryError> {
        let client = self.client.lock().await;
        let row = client
            .query_opt(
                "SELECT entry_id, entry_type, agent_id, content, content_type, source_id,
                        correlation_id, idempotency_key, entry_hash, previous_hash, chain_position, written_ts
                 FROM are_ledger.ledger_entries WHERE entry_id = $1",
                &[&entry_id],
            )
            .await
            .map_err(|_| RepositoryError::Unavailable)?;
        row.map(|r| Self::map_row_entry(&r))
            .ok_or(RepositoryError::NotFound)
    }

    async fn query_entries(&self) -> Result<Vec<LedgerEntryRecord>, RepositoryError> {
        let client = self.client.lock().await;
        let rows = client
            .query(
                "SELECT entry_id, entry_type, agent_id, content, content_type, source_id,
                        correlation_id, idempotency_key, entry_hash, previous_hash, chain_position, written_ts
                 FROM are_ledger.ledger_entries
                 ORDER BY written_ts, chain_position",
                &[],
            )
            .await
            .map_err(|_| RepositoryError::Unavailable)?;
        Ok(rows.iter().map(Self::map_row_entry).collect())
    }

    async fn get_chain_tip(&self, entry_type: &str) -> Result<ChainTip, RepositoryError> {
        let client = self.client.lock().await;
        let row = client
            .query_opt(
                "SELECT entry_id, entry_hash, chain_position, written_ts
                 FROM are_ledger.ledger_entries
                 WHERE entry_type = $1
                 ORDER BY chain_position DESC
                 LIMIT 1",
                &[&entry_type],
            )
            .await
            .map_err(|_| RepositoryError::Unavailable)?;
        let row = row.ok_or(RepositoryError::NotFound)?;
        Ok(ChainTip {
            entry_id: row.get("entry_id"),
            hash: row.get("entry_hash"),
            position: row.get("chain_position"),
            written_ts: row.get("written_ts"),
        })
    }

    async fn get_entries_by_type(
        &self,
        entry_type: &str,
    ) -> Result<Vec<LedgerEntryRecord>, RepositoryError> {
        let client = self.client.lock().await;
        let rows = client
            .query(
                "SELECT entry_id, entry_type, agent_id, content, content_type, source_id,
                        correlation_id, idempotency_key, entry_hash, previous_hash, chain_position, written_ts
                 FROM are_ledger.ledger_entries
                 WHERE entry_type = $1
                 ORDER BY chain_position",
                &[&entry_type],
            )
            .await
            .map_err(|_| RepositoryError::Unavailable)?;
        Ok(rows.iter().map(Self::map_row_entry).collect())
    }

    async fn find_idempotent(
        &self,
        entry_type: &str,
        idempotency_key: &str,
    ) -> Result<Option<LedgerEntryRecord>, RepositoryError> {
        let client = self.client.lock().await;
        let row = client
            .query_opt(
                "SELECT entry_id, entry_type, agent_id, content, content_type, source_id,
                        correlation_id, idempotency_key, entry_hash, previous_hash, chain_position, written_ts
                 FROM are_ledger.ledger_entries
                 WHERE entry_type = $1 AND idempotency_key = $2",
                &[&entry_type, &idempotency_key],
            )
            .await
            .map_err(|_| RepositoryError::Unavailable)?;
        Ok(row.map(|r| Self::map_row_entry(&r)))
    }

    async fn pending_outbox(&self) -> Result<Vec<OutboxRecord>, RepositoryError> {
        let client = self.client.lock().await;
        let rows = client
            .query(
                "SELECT outbox_id, entry_id, entry_type, payload, status, attempt_count
                 FROM are_ledger.ledger_write_outbox
                 WHERE status = 'PENDING'
                 ORDER BY created_ts",
                &[],
            )
            .await
            .map_err(|_| RepositoryError::Unavailable)?;
        Ok(rows.iter().map(Self::map_outbox_row).collect())
    }

    async fn mark_outbox_delivered(&self, outbox_id: Uuid) -> Result<(), RepositoryError> {
        let client = self.client.lock().await;
        let n = client
            .execute(
                "UPDATE are_ledger.ledger_write_outbox
                 SET status = 'DELIVERED'
                 WHERE outbox_id = $1 AND status = 'PENDING'",
                &[&outbox_id],
            )
            .await
            .map_err(|_| RepositoryError::Unavailable)?;
        if n == 0 {
            return Err(RepositoryError::NotFound);
        }
        Ok(())
    }
}
