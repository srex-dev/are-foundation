# OPA Integration Layer (ARE-A-S1-001)

Loads active policy bundles and evaluates policy decisions with deny-by-default behavior.

## Local commands

- `npm ci`
- `npm run lint`
- `npm run test`
- `npm run typecheck`
- `npm run build`

## Runtime endpoints

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

## Build image

- `docker build -t are/opa-integration:latest .`

## Deployment artifacts

- `k8s/deployment.yaml`
- `k8s/service.yaml`
- `k8s/configmap.yaml`
- `k8s/hpa.yaml`
- `k8s/pdb.yaml`

## Contracts

- gRPC proto: `proto/opa_integration.proto`

## Bundle integrity

- Optional **`integritySha256`**: lowercase hex SHA-256 of `regoSource` (UTF-8).
- Optional **`bundleSignatureEd25519`**: base64-encoded raw Ed25519 signature (64 bytes) over UTF-8 bytes of **bundleId**, newline, **version**, newline, **integritySha256**. Requires **`integritySha256`** and runtime env **`ARE_OPA_BUNDLE_SIGNING_PUBLIC_KEY_PEM`** (SPKI PEM public key). Rego compilation remains the line-oriented integration parser unless a future **opa / WASM** hook is enabled (see `docs/audit/readiness-backlog-reconciliation.md` M-17).
