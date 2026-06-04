# Passport Issuance Engine (ARE-A-S0-005)

This component issues, verifies, renews, and revokes signed agent passports.

## Local development

- `go test ./...`
- `go build ./...`
- `./scripts/build-release.sh`

## Endpoints

- `GET /healthz` on `ARE_PASSPORT_HEALTH_PORT` (default `8080`)
- `GET /readyz` on `ARE_PASSPORT_HEALTH_PORT` (default `8080`)
- `GET /metrics` on `ARE_PASSPORT_HEALTH_PORT` (default `8080`)

## Build container

- `docker build -t are/passport-issuance:latest .`

## Kubernetes

- `k8s/deployment.yaml`
- `k8s/service.yaml`
- `k8s/configmap.yaml`
- `k8s/hpa.yaml`
- `k8s/pdb.yaml`

## Notes

- Schema and outbox DDL are in `migrations/`.
- Proto contract is in `proto/passport_issuance.proto`.
