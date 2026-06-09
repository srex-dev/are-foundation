# Public Boundary

ARE Foundation is the public S0/S1 runtime for governed agent authority. It is
designed to be runnable, useful, and small enough to inspect.

The OSS story is:

```text
register an actor -> issue scoped authority -> evaluate scope/policy -> return public-safe proof basics
```

Every default example keeps `executed=false`. The repository does not execute
customer actions, activate policy, claim certification, or include the full
commercial ARE platform.

## Included vs. Out Of Scope

| Area | Included In `are-foundation` | Not Included Here | Notes |
|---|---|---|---|
| Identity | Public-safe actor registration and lookup | Persistent customer tenant/user administration | Actor records are demo/local foundation records. |
| Passport authority | Issue, list, and verify scoped authority summaries | Commercial passport administration UX, durable client authority workflows | Passport summaries are public-safe; no secret credentials are exposed. |
| Scope evaluation | Action/resource scope checks against active passports | Full adaptive execution gating | Scope `ALLOW` is authority proof, not action execution. |
| Policy evaluation | Local OPA/stub check-only allow/deny decisions | BYOPolicy commercial UX, policy activation, policy pack workspace | Policy evaluation is a draft/check surface only. |
| Proof basics | Public-safe request IDs, decision summaries, source refs, `executed=false` | Private proof packets, protected evidence bodies, synthetic emulator archives | Reports must not contain raw payloads, headers, signatures, credentials, or protected evidence. |
| Gateway | S0/S1 REST perimeter, auth/header enforcement, route guard | Command Center gateway routes, visual RAG, client demo frontend | Non-foundation routes fail closed. |
| Runtime | Local Docker Compose for development and evaluation | Helm, Terraform, managed cloud, production HA deployment artifacts | See `docs/deployment-boundary.md`. |
| Dev security | Guarded local dev flags and generated TLS certs | Production identity, secret management, public ingress hardening | See `docs/dev-mode-security.md`. |
| Governance-strata | Conceptual integration hook in docs | Governance-strata internals and commercial transition orchestration | Higher-risk transitions can be wrapped by commercial ARE/governance-strata later. |
| Advanced ARE stages | S0/S1 foundation only | S2-S6 adaptive systems and commercial source-truth maps | Later public expansion must be deliberate and reviewed. |

## Contribution Boundary

Good public contributions:

- API contract fixes for the S0/S1 surface.
- Identity, passport, scope, policy, and proof-root foundation improvements.
- Public-safe examples and docs.
- Local developer tooling, tests, and hygiene checks.
- Security hardening that preserves the public boundary.

Out-of-scope contributions:

- Command Center features.
- Visual RAG, operator widgets, or client demo frontend surfaces.
- BYOPolicy commercial workflow UI or policy-pack product UX.
- Private proof packet formats, protected evidence bodies, or synthetic emulator internals.
- S2-S6 adaptive systems or governance-strata implementation code.
- Deployment artifacts that imply supported production hosting before the project is ready to support them.

## Public Proof Rule

Public reports may include:

- request IDs
- fake/demo agent IDs
- fake/demo passport IDs
- allow/deny decisions
- source refs
- aggregate latency/error counts
- `executed=false`
- `receipt_created=false`

Public reports must not include:

- bearer tokens
- credentials
- raw headers
- signatures
- private keys
- protected payloads
- raw customer evidence
- raw policy bodies
- private proof packets

## Commercial Boundary

The full ARE platform adds operator Command Center, visual RAG, BYOPolicy
commercial workflows, richer policy-pack UX, advanced source-truth maps,
synthetic/emulator proof packets, S2-S6 adaptive systems, and governance-strata
transition orchestration.

Those are intentionally not bundled in this repository. The OSS repo is the
foundation layer that makes authority and policy checks visible before any
customer action executes.
