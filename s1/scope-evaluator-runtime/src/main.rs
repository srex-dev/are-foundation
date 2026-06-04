use std::net::SocketAddr;
use std::sync::Arc;

use async_trait::async_trait;
use axum::{routing::get, Router};
use tonic::transport::Server;
use tracing::info;

use are_scope_evaluator::grpc_passport::GrpcPassportClient;
use are_scope_evaluator::grpc_scope::ScopeGrpc;
use are_scope_evaluator::metrics::encode_prometheus;
use are_scope_evaluator::pb::are::scope::v1::scope_evaluator_service_server::ScopeEvaluatorServiceServer;
use are_scope_evaluator::service::{PassportClient, ScopeError, ScopeEvaluatorService};
use are_scope_evaluator::types::Passport;

/// Returns NotFound for every agent when passport backend is not configured.
#[derive(Clone, Default)]
struct StubPassportClient;

#[async_trait]
impl PassportClient for StubPassportClient {
    async fn get_agent_passport(&self, _agent_id: &str) -> Result<Passport, ScopeError> {
        Err(ScopeError::NotFound)
    }
}

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(std::env::var("RUST_LOG").unwrap_or_else(|_| "info".to_string()))
        .json()
        .init();

    let grpc_port = parse_u16("ARE_SCOPE_GRPC_PORT", 9097);
    let health_port = parse_u16("ARE_SCOPE_HEALTH_PORT", 8080);
    let metrics_port = parse_u16("ARE_SCOPE_METRICS_PORT", 8088);
    let cache_max = parse_usize("ARE_SCOPE_CACHE_MAX_ENTRIES", 500);
    let cache_ttl = parse_u64("ARE_SCOPE_CACHE_TTL_SECONDS", 60);
    let circuit_threshold = parse_u32("ARE_SCOPE_PASSPORT_CIRCUIT_THRESHOLD", 5);

    let passport_client: Arc<dyn PassportClient> = match std::env::var("ARE_SCOPE_PASSPORT_GRPC_ADDR")
    {
        Ok(addr) if !addr.trim().is_empty() => {
            let addr = addr.trim().to_string();
            match GrpcPassportClient::connect(addr.clone()).await {
                Ok(c) => {
                    info!(%addr, "scope evaluator using passport gRPC backend");
                    Arc::new(c)
                }
                Err(e) => {
                    tracing::error!(%addr, err = %e, "passport gRPC connect failed; using stub");
                    Arc::new(StubPassportClient)
                }
            }
        }
        _ => {
            tracing::warn!(
                "ARE_SCOPE_PASSPORT_GRPC_ADDR unset; scope evaluations will fail passport fetch"
            );
            Arc::new(StubPassportClient)
        }
    };

    let core = Arc::new(ScopeEvaluatorService::new(
        passport_client,
        cache_max,
        cache_ttl,
        circuit_threshold,
    ));
    let grpc_svc = ScopeEvaluatorServiceServer::new(ScopeGrpc::new(core));

    let grpc_addr = SocketAddr::from(([0, 0, 0, 0], grpc_port));
    info!(%grpc_addr, "scope evaluator gRPC listening");

    let health_app = Router::new()
        .route("/healthz", get(|| async { "ok" }))
        .route("/readyz", get(|| async { "ready" }));
    let health_addr = SocketAddr::from(([0, 0, 0, 0], health_port));
    let health_listener = tokio::net::TcpListener::bind(health_addr)
        .await
        .expect("health bind");
    info!(%health_addr, "scope evaluator health listening");

    let metrics_app = Router::new().route(
        "/metrics",
        get(|| async {
            let body = encode_prometheus();
            (
                [(
                    axum::http::header::CONTENT_TYPE,
                    "text/plain; version=0.0.4; charset=utf-8",
                )],
                body,
            )
        }),
    );
    let metrics_addr = SocketAddr::from(([0, 0, 0, 0], metrics_port));
    let metrics_listener = tokio::net::TcpListener::bind(metrics_addr)
        .await
        .expect("metrics bind");
    info!(%metrics_addr, "scope evaluator metrics listening");

    let grpc_task = async move {
        Server::builder()
            .add_service(grpc_svc)
            .serve(grpc_addr)
            .await
            .expect("grpc serve");
    };
    let health_task = async move {
        axum::serve(health_listener, health_app)
            .await
            .expect("health serve");
    };
    let metrics_task = async move {
        axum::serve(metrics_listener, metrics_app)
            .await
            .expect("metrics serve");
    };

    let _ = tokio::join!(grpc_task, health_task, metrics_task);
}

fn parse_u16(key: &str, default: u16) -> u16 {
    std::env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

fn parse_u32(key: &str, default: u32) -> u32 {
    std::env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

fn parse_u64(key: &str, default: u64) -> u64 {
    std::env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

fn parse_usize(key: &str, default: usize) -> usize {
    std::env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}
