use thiserror::Error;
use tokio_postgres::NoTls;

#[derive(Debug, Error)]
pub enum DbPermissionError {
    #[error("failed to connect to postgres: {0}")]
    Connect(String),
    #[error("failed to query postgres privileges: {0}")]
    Query(String),
    #[error("ledger role must have INSERT and SELECT on are_ledger.ledger_entries")]
    MissingReadWritePrivileges,
    #[error("ledger role must NOT have UPDATE or DELETE on are_ledger.ledger_entries")]
    LedgerEntriesMutable,
}

pub async fn verify_ledger_entries_immutable(
    connection_string: &str,
) -> Result<(), DbPermissionError> {
    let (client, connection) = tokio_postgres::connect(connection_string, NoTls)
        .await
        .map_err(|err| DbPermissionError::Connect(err.to_string()))?;
    tokio::spawn(async move {
        let _ = connection.await;
    });

    let row = client
        .query_one(
            "SELECT \
               has_table_privilege(current_user, 'are_ledger.ledger_entries', 'INSERT') AS can_insert, \
               has_table_privilege(current_user, 'are_ledger.ledger_entries', 'SELECT') AS can_select, \
               has_table_privilege(current_user, 'are_ledger.ledger_entries', 'UPDATE') AS can_update, \
               has_table_privilege(current_user, 'are_ledger.ledger_entries', 'DELETE') AS can_delete",
            &[],
        )
        .await
        .map_err(|err| DbPermissionError::Query(err.to_string()))?;

    let can_insert: bool = row.get("can_insert");
    let can_select: bool = row.get("can_select");
    let can_update: bool = row.get("can_update");
    let can_delete: bool = row.get("can_delete");
    evaluate_permissions(can_insert, can_select, can_update, can_delete)
}

fn evaluate_permissions(
    can_insert: bool,
    can_select: bool,
    can_update: bool,
    can_delete: bool,
) -> Result<(), DbPermissionError> {
    if !can_insert || !can_select {
        return Err(DbPermissionError::MissingReadWritePrivileges);
    }
    if can_update || can_delete {
        return Err(DbPermissionError::LedgerEntriesMutable);
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn accepts_insert_select_only() {
        let result = evaluate_permissions(true, true, false, false);
        assert!(result.is_ok());
    }

    #[test]
    fn rejects_missing_insert_or_select() {
        let result = evaluate_permissions(false, true, false, false);
        assert!(matches!(
            result,
            Err(DbPermissionError::MissingReadWritePrivileges)
        ));
    }

    #[test]
    fn rejects_update_or_delete() {
        let result = evaluate_permissions(true, true, true, false);
        assert!(matches!(
            result,
            Err(DbPermissionError::LedgerEntriesMutable)
        ));
    }
}
