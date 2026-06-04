# ARE Immutable Ledger (ARE-A-S0-003)

Rust gRPC service for append-only, hash-chained ledger persistence with per-entry-type chains.

## Implemented scope

- Write, read, query, verify-entry, verify-chain, and chain-tip gRPC methods
- Canonical SHA-256 entry hashing and per-type chain linking
- Idempotency key support per entry type
- Transactional outbox model at repository boundary with async outbox publisher worker
- Health/readiness HTTP endpoints (`/healthz`, `/readyz`)
- Deployment artifacts for Kubernetes and self-hosted systemd

## Required environment variables

- `ARE_LEDGER_DB_CONNECTION_STRING`
- `ARE_LEDGER_KAFKA_BOOTSTRAP_SERVERS`
- `ARE_LEDGER_KAFKA_SASL_USERNAME`
- `ARE_LEDGER_KAFKA_SASL_PASSWORD`

Optional:

- `ARE_LEDGER_READ_REPLICA_CONNECTION_STRING`
- `ARE_LEDGER_GRPC_PORT` (default `9092`)
- `ARE_LEDGER_HEALTH_PORT` (default `8080`)
- `ARE_LEDGER_METRICS_PORT` (default `8083`)
- `ARE_LEDGER_MAX_CONTENT_SIZE_BYTES` (default `1048576`)
- `ARE_LEDGER_GENESIS_HASH_INPUT` (default `ARE_LEDGER_GENESIS`)

## Build and test

```bash
cargo fmt
cargo test
cargo clippy -- -D warnings
cargo build --release
```

## Artifacts

- `proto/immutable_ledger.proto`
- `migrations/001_init.sql`
- `Dockerfile`
- `k8s/deployment.yaml`
- `k8s/service.yaml`
- `k8s/configmap.yaml`
- `k8s/hpa.yaml`
- `k8s/pdb.yaml`
- `systemd/ARE-A-S0-003.service`
- `scripts/build-release.sh`

