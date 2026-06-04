# S0/S1 REST BFF

Foundation REST service for identity, passport, scope, and policy checks.

Routes:

- `POST /v1/identity/agents`
- `GET /v1/identity/agents/{agent_id}`
- `POST /v1/passports`
- `GET /v1/passports/by-agent/{agent_id}`
- `POST /v1/passports:verify`
- `POST /v1/enforcement/scope:evaluate`
- `POST /v1/policy/evaluations`
- `GET /v1/meta/deployment`

The service stores public-safe agent and passport summaries in memory or Postgres.

It does not execute customer actions.

## Local

```bash
go test ./...
```
