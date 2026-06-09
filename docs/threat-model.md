# ARE Foundation Threat Model

This document describes the public OSS S0/S1 foundation boundary.

## Assets

- Agent identifiers and public-safe metadata.
- Passport records and scoped authority summaries.
- Scope and policy decisions.
- Public-safe proof summaries.
- Local development credentials used only for Docker Compose.
- Dev-mode flags used only for local Docker Compose.

## Trust Boundaries

- The gateway is the public API perimeter.
- The S0/S1 REST BFF is the foundation aggregation surface.
- Postgres, OPA, JWKS, Kafka-compatible paths, and service internals stay behind the local compose network.
- Caller identity is represented by bearer auth plus request headers in the public API contract.

## Explicit Non-Goals

- No customer action execution by default.
- No policy activation or certification claim.
- No Command Center, visual RAG, client demo frontend, private proof packets, S2-S6 adaptive systems, or governance-strata internals.
- No storage of raw customer payloads, protected evidence bodies, credentials, private keys, bearer tokens, raw headers, signatures, or raw policy bodies in public reports.

## Main Risks

| Risk | Mitigation |
|---|---|
| Secret or private proof material enters the repo | `tools/security/secret_scan.py`, release audit, `.gitignore`, PR template, SECURITY.md |
| Private/commercial surface leaks into OSS | route guards, export scanner private marker checks, release checklist |
| OSS users mistake checks for execution | README/release docs and smoke reports state `executed=false` and `receipt_created=false` |
| Unknown agent/action appears allowed | runtime gates require unknown/revoked/expired paths to fail closed |
| Local demo credentials are reused in real environments | SECURITY.md and docs mark compose credentials as local-only |
| Dev-mode flags are copied into an exposed deployment | `docs/dev-mode-security.md` documents every unsafe local flag and the runtime refuses several unsafe combinations unless built for dev |
| Local Compose is mistaken for a production deployment | `docs/deployment-boundary.md` states the shipped artifacts are local-only and lists what is intentionally not provided |

## Public Proof Rule

Reports may include request IDs, agent IDs, passport IDs, decisions, source refs, aggregate latency, and aggregate error counts.

Reports must not include protected payloads, raw signatures, bearer tokens, credentials, raw headers, raw policy bodies, private evidence, or private proof packet contents.
