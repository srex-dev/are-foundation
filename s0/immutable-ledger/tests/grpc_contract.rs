use std::sync::Arc;

use are_immutable_ledger::config::AppConfig;
use are_immutable_ledger::grpc::pb::immutable_ledger_service_client::ImmutableLedgerServiceClient;
use are_immutable_ledger::grpc::{pb, ImmutableLedgerGrpc};
use are_immutable_ledger::repository::InMemoryLedgerRepository;
use are_immutable_ledger::service::{ImmutableLedgerService, NoopEventPublisher};
use tonic::transport::Server;

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
async fn ig006_grpc_contract_smoke() {
    let service = Arc::new(ImmutableLedgerService::new(
        Arc::new(InMemoryLedgerRepository::default()),
        Arc::new(NoopEventPublisher),
        config(),
    ));
    let grpc = ImmutableLedgerGrpc::new(service);

    let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind");
    let addr = listener.local_addr().expect("addr");
    let incoming = tokio_stream::wrappers::TcpListenerStream::new(listener);
    let server = tokio::spawn(async move {
        Server::builder()
            .add_service(
                pb::immutable_ledger_service_server::ImmutableLedgerServiceServer::new(grpc),
            )
            .serve_with_incoming(incoming)
            .await
            .expect("serve");
    });

    let mut client = ImmutableLedgerServiceClient::connect(format!("http://{}", addr))
        .await
        .expect("connect");
    let write = client
        .write_entry(pb::WriteEntryRequest {
            entry_type: "LEDGER_ENTRY_TYPE_GATE_DECISION".to_string(),
            agent_id: "agent-x".to_string(),
            content: b"payload".to_vec(),
            content_type: "application/json".to_string(),
            source_id: "ARE-FOUNDATION-PROOF".to_string(),
            correlation_id: String::new(),
            idempotency_key: "grpc-smoke".to_string(),
        })
        .await
        .expect("write")
        .into_inner();
    let get = client
        .get_entry(pb::GetEntryRequest {
            entry_id: write.entry_id.clone(),
        })
        .await
        .expect("get")
        .into_inner();
    assert!(get.entry.is_some());

    let verify = client
        .verify_entry(pb::VerifyEntryRequest {
            entry_id: write.entry_id,
        })
        .await
        .expect("verify")
        .into_inner();
    assert!(verify.hash_valid);
    assert!(verify.chain_link_valid);

    let query = client
        .query_entries(pb::QueryEntriesRequest {
            entry_type: "LEDGER_ENTRY_TYPE_GATE_DECISION".to_string(),
            agent_id: String::new(),
            source_id: String::new(),
            correlation_id: String::new(),
            from_ts: 0,
            to_ts: 0,
            page_size: 10,
            page_token: String::new(),
        })
        .await
        .expect("query")
        .into_inner();
    assert_eq!(query.total_count, 1);
    assert_eq!(query.entries.len(), 1);

    let chain_tip = client
        .get_chain_tip(pb::GetChainTipRequest {
            entry_type: "LEDGER_ENTRY_TYPE_GATE_DECISION".to_string(),
        })
        .await
        .expect("tip")
        .into_inner();
    assert!(!chain_tip.entry_id.is_empty());

    let chain_verify = client
        .verify_chain(pb::VerifyChainRequest {
            entry_type: "LEDGER_ENTRY_TYPE_GATE_DECISION".to_string(),
            start_entry_id: String::new(),
            end_entry_id: String::new(),
        })
        .await
        .expect("verify chain")
        .into_inner();
    assert!(chain_verify.chain_valid);

    let not_found = client
        .get_entry(pb::GetEntryRequest {
            entry_id: "11111111-1111-1111-1111-111111111111".to_string(),
        })
        .await;
    assert!(not_found.is_err());

    server.abort();
}
