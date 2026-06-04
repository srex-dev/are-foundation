#![forbid(unsafe_code)]

use std::net::SocketAddr;
use std::sync::Arc;

use anyhow::Context;
use axum::{
    http::{header, StatusCode},
    routing::{get, post},
    Router,
};
use tokio::net::TcpListener;
use tokio::sync::watch;
use tonic::transport::Server;
use tracing::{info, warn};

use are_immutable_ledger::config::AppConfig;
use are_immutable_ledger::db_permissions::verify_ledger_entries_immutable;
use are_immutable_ledger::grpc::pb::immutable_ledger_service_server::ImmutableLedgerServiceServer;
use are_immutable_ledger::grpc::ImmutableLedgerGrpc;
use are_immutable_ledger::metrics;
use are_immutable_ledger::repository::PostgresLedgerRepository;
use are_immutable_ledger::service::{ImmutableLedgerService, NoopEventPublisher};
use tokio_postgres::NoTls;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter("info")
        .json()
        .with_current_span(false)
        .with_span_list(false)
        .init();

    let config = Arc::new(AppConfig::from_env().context("failed to load config")?);
    info!(
        grpc_port = config.grpc_port,
        health_port = config.health_port,
        metrics_port = config.metrics_port,
        "immutable ledger starting"
    );
    match verify_ledger_entries_immutable(&config.db_connection_string).await {
        Ok(_) => info!("verified DB permissions: ledger_entries is immutable for current role"),
        Err(err) => {
            warn!("DB permission verification failed: {err}");
            return Err(anyhow::anyhow!("db permission verification failed: {err}"));
        }
    }

    let pg = connect_postgres(&config.db_connection_string)
        .await
        .context("postgres connect for ledger repository")?;
    let repo = Arc::new(PostgresLedgerRepository::new(pg));
    let publisher = Arc::new(NoopEventPublisher);
    let service = Arc::new(ImmutableLedgerService::new(repo, publisher, config.clone()));
    let grpc = ImmutableLedgerGrpc::new(service);

    let grpc_addr = SocketAddr::from(([0, 0, 0, 0], config.grpc_port));
    let health_addr = SocketAddr::from(([0, 0, 0, 0], config.health_port));
    let metrics_addr = SocketAddr::from(([0, 0, 0, 0], config.metrics_port));
    let (shutdown_tx, _) = watch::channel(false);

    {
        let shutdown_tx = shutdown_tx.clone();
        tokio::spawn(async move {
            let _ = wait_for_shutdown_signal().await;
            let _ = shutdown_tx.send(true);
        });
    }

    let mut grpc_shutdown_rx = shutdown_tx.subscribe();
    let grpc_server = tokio::spawn(async move {
        Server::builder()
            .add_service(ImmutableLedgerServiceServer::new(grpc))
            .serve_with_shutdown(grpc_addr, async move {
                let _ = grpc_shutdown_rx.changed().await;
            })
            .await
    });

    let shutdown_tx_for_route = shutdown_tx.clone();
    let app = Router::new()
        .route("/healthz", get(|| async { "ok" }))
        .route("/readyz", get(|| async { "ready" }))
        .route(
            "/shutdownz",
            post(move || {
                let shutdown_tx = shutdown_tx_for_route.clone();
                async move {
                    let _ = shutdown_tx.send(true);
                    StatusCode::ACCEPTED
                }
            }),
        );
    let listener = TcpListener::bind(health_addr)
        .await
        .context("failed to bind health listener")?;
    let mut health_shutdown_rx = shutdown_tx.subscribe();
    let health_server = tokio::spawn(async move {
        axum::serve(listener, app)
            .with_graceful_shutdown(async move {
                let _ = health_shutdown_rx.changed().await;
            })
            .await
    });

    let metrics_app = Router::new().route(
        "/metrics",
        get(|| async {
            let body = metrics::encode_prometheus();
            (
                [(
                    header::CONTENT_TYPE,
                    "text/plain; version=0.0.4; charset=utf-8",
                )],
                body,
            )
        }),
    );
    let metrics_listener = TcpListener::bind(metrics_addr)
        .await
        .context("failed to bind metrics listener")?;
    let mut metrics_shutdown_rx = shutdown_tx.subscribe();
    let metrics_server = tokio::spawn(async move {
        axum::serve(metrics_listener, metrics_app)
            .with_graceful_shutdown(async move {
                let _ = metrics_shutdown_rx.changed().await;
            })
            .await
    });

    let (grpc_result, health_result, metrics_result) =
        tokio::try_join!(grpc_server, health_server, metrics_server)?;
    grpc_result.context("gRPC server failed")?;
    health_result.context("health server failed")?;
    metrics_result.context("metrics server failed")?;
    Ok(())
}

async fn connect_postgres(
    url: &str,
) -> anyhow::Result<std::sync::Arc<tokio::sync::Mutex<tokio_postgres::Client>>> {
    let (client, connection) = tokio_postgres::connect(url, NoTls)
        .await
        .with_context(|| format!("postgres connect to {}", url))?;
    tokio::spawn(async move {
        if let Err(err) = connection.await {
            tracing::error!(error = %err, "postgres connection task ended with error");
        }
    });
    Ok(std::sync::Arc::new(tokio::sync::Mutex::new(client)))
}

async fn wait_for_shutdown_signal() -> anyhow::Result<()> {
    #[cfg(unix)]
    {
        use tokio::signal::unix::{signal, SignalKind};
        let mut terminate = signal(SignalKind::terminate())?;
        tokio::select! {
            _ = tokio::signal::ctrl_c() => {}
            _ = terminate.recv() => {}
        }
    }
    #[cfg(not(unix))]
    {
        tokio::signal::ctrl_c().await?;
    }
    Ok(())
}
