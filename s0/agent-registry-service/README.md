# ARE-A-S0-001 Agent Registry Service

Go implementation scaffold for the ARE Agent Registry Service.

## What this component does

- Registers agent identities with UUIDv4 `agent_id`
- Persists agent records in PostgreSQL
- Tracks lifecycle status transitions
- Writes lifecycle events to an outbox table for Kafka delivery
- Exposes health and metrics endpoints

## Layout

- `cmd/are-agent-registry` - process entrypoint
- `internal/config` - environment-based runtime config
- `internal/repository/postgres` - PostgreSQL persistence layer
- `internal/service` - domain behavior rules
- `internal/outbox` - background outbox processor
- `proto/agent_registry.proto` - gRPC contract source
- `migrations/` - SQL schema and outbox migration
- `k8s/` - Kubernetes deployment assets
- `systemd/` - self-hosted unit file

## Required environment variables

- `ARE_DB_CONNECTION_STRING`
- `ARE_KAFKA_BOOTSTRAP_SERVERS`
- `ARE_KAFKA_SASL_USERNAME`
- `ARE_KAFKA_SASL_PASSWORD`

Optional:

- `ARE_REDIS_CONNECTION_STRING`
- `ARE_GRPC_PORT` (default `9090`)
- `ARE_METRICS_PORT` (default `8081`)
- `ARE_HEALTH_PORT` (default `8080`)
- `ARE_KAFKA_TOPIC_LIFECYCLE` (default `are.agents.lifecycle`)
- `ARE_OUTBOX_POLL_INTERVAL_MS` (default `500`)
- `ARE_OUTBOX_MAX_ATTEMPTS` (default `10`)

## Local build and test

```bash
go mod tidy
go test ./...
go build ./...
```

## Notes

- `protoc` tooling is required to generate Go stubs from `proto/agent_registry.proto` and wire full gRPC method registration.
- This service currently starts gRPC health service and all infrastructure hooks required for full method implementation.
