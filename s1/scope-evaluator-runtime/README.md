# Scope Evaluator Runtime (ARE-A-S1-002)

Deterministic runtime scope evaluator for passport scope checks.

## Local commands

- `cargo fmt`
- `cargo test`
- `cargo clippy -- -D warnings`
- `cargo build --release`

## Endpoints

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

## Build image

- `docker build -t are/scope-evaluator:latest .`

## Deployment artifacts

- `k8s/deployment.yaml`
- `k8s/service.yaml`
- `k8s/configmap.yaml`
- `k8s/hpa.yaml`
- `k8s/pdb.yaml`

## Contracts

- gRPC proto: `proto/scope_evaluator.proto`
