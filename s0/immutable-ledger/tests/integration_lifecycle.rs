use std::sync::Arc;

use are_immutable_ledger::config::AppConfig;
use are_immutable_ledger::repository::InMemoryLedgerRepository;
use are_immutable_ledger::service::{ImmutableLedgerService, NoopEventPublisher, WriteEntryInput};

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

#[tokio::test]
async fn it001_write_read_verify_cycle() {
    let service = ImmutableLedgerService::new(
        Arc::new(InMemoryLedgerRepository::default()),
        Arc::new(NoopEventPublisher),
        config(),
    );
    let write = service
        .write_entry(WriteEntryInput {
            entry_type: "LEDGER_ENTRY_TYPE_ACTION_RECEIPT".to_string(),
            agent_id: "agent-a".to_string(),
            content: b"content".to_vec(),
            content_type: "application/json".to_string(),
            source_id: "ARE-FOUNDATION-PROOF".to_string(),
            correlation_id: None,
            idempotency_key: None,
        })
        .await
        .expect("write");
    let entry = service.get_entry(write.entry_id).await.expect("get");
    assert_eq!(entry.entry_hash, write.entry_hash);
    let verify = service.verify_entry(write.entry_id).await.expect("verify");
    assert!(verify.hash_valid);
    assert!(verify.chain_link_valid);
}

#[tokio::test]
async fn it002_chain_integrity_for_many_entries() {
    let service = ImmutableLedgerService::new(
        Arc::new(InMemoryLedgerRepository::default()),
        Arc::new(NoopEventPublisher),
        config(),
    );
    for i in 0..1000 {
        let _ = service
            .write_entry(WriteEntryInput {
                entry_type: "LEDGER_ENTRY_TYPE_AGENT_LIFECYCLE".to_string(),
                agent_id: format!("agent-{}", i),
                content: format!("payload-{}", i).into_bytes(),
                content_type: "application/json".to_string(),
                source_id: "ARE-A-S0-001".to_string(),
                correlation_id: None,
                idempotency_key: Some(format!("idem-{}", i)),
            })
            .await
            .expect("write");
    }
    let verify = service
        .verify_chain("LEDGER_ENTRY_TYPE_AGENT_LIFECYCLE", None, None)
        .await
        .expect("verify chain");
    assert!(verify.chain_valid);
    assert_eq!(verify.entries_checked, 1000);
}

#[tokio::test]
async fn it003_concurrent_multi_type_writes() {
    let service = ImmutableLedgerService::new(
        Arc::new(InMemoryLedgerRepository::default()),
        Arc::new(NoopEventPublisher),
        config(),
    );
    let entry_types = vec![
        "LEDGER_ENTRY_TYPE_ACTION_RECEIPT",
        "LEDGER_ENTRY_TYPE_POLICY_EVAL",
        "LEDGER_ENTRY_TYPE_AGENT_LIFECYCLE",
        "LEDGER_ENTRY_TYPE_CREDENTIAL_LIFECYCLE",
        "LEDGER_ENTRY_TYPE_PASSPORT_LIFECYCLE",
        "LEDGER_ENTRY_TYPE_ESCALATION",
        "LEDGER_ENTRY_TYPE_DRIFT_EVENT",
        "LEDGER_ENTRY_TYPE_GATE_DECISION",
        "DELEGATION_ADMITTED",
        "CONSTRAINT_DRIFT_DETECTED",
    ];

    let mut jobs = Vec::new();
    for entry_type in entry_types {
        let svc = service.clone();
        let current_type = entry_type.to_string();
        jobs.push(tokio::spawn(async move {
            for i in 0..100 {
                svc.write_entry(WriteEntryInput {
                    entry_type: current_type.clone(),
                    agent_id: format!("agent-{}", i),
                    content: format!("{}-payload-{}", current_type, i).into_bytes(),
                    content_type: "application/json".to_string(),
                    source_id: "ARE-A-S0-003".to_string(),
                    correlation_id: None,
                    idempotency_key: Some(format!("{}-idem-{}", current_type, i)),
                })
                .await
                .expect("write");
            }
            current_type
        }));
    }

    for job in jobs {
        let entry_type = job.await.expect("join");
        let verify = service
            .verify_chain(&entry_type, None, None)
            .await
            .expect("verify chain");
        assert!(verify.chain_valid);
        assert_eq!(verify.entries_checked, 100);
    }
}

#[tokio::test]
async fn it004_delegation_admitted_entry_type_chain() {
    let service = ImmutableLedgerService::new(
        Arc::new(InMemoryLedgerRepository::default()),
        Arc::new(NoopEventPublisher),
        config(),
    );
    let w1 = service
        .write_entry(WriteEntryInput {
            entry_type: "DELEGATION_ADMITTED".to_string(),
            agent_id: "leaf-agent".to_string(),
            content: br#"{"chain_depth":2,"root_agent_id":"r","leaf_agent_id":"l"}"#.to_vec(),
            content_type: "application/json".to_string(),
            source_id: "ARE-A-S1-NTA".to_string(),
            correlation_id: None,
            idempotency_key: Some("idem-deleg-1".to_string()),
        })
        .await
        .expect("write");
    let tip = service
        .get_chain_tip("DELEGATION_ADMITTED")
        .await
        .expect("tip");
    assert_eq!(tip.entry_id, w1.entry_id);
    let verify = service
        .verify_chain("DELEGATION_ADMITTED", None, None)
        .await
        .expect("verify");
    assert!(verify.chain_valid);
}
