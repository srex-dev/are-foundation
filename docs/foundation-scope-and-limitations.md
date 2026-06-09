# Foundation Scope And Limitations

This document answers the questions reviewers are likely to ask about what ARE
Foundation includes, what it intentionally leaves out, and what belongs to the
larger ARE platform.

The short version:

> ARE Foundation is the public S0/S1 wedge. It proves the base primitive:
> actor identity, scoped authority, scope/policy evaluation, and public-safe
> proof before execution. It is not the full commercial ARE platform.

Nothing in this document is a claim of production certification or full ARE
coverage.

## What S0/S1 Proves

ARE Foundation proves four things before a customer action can happen:

1. **Who is acting:** an actor/agent can be registered and read.
2. **What authority exists:** a scoped passport can be issued, listed, and
   verified.
3. **Whether the request fits authority and policy:** scope and policy can be
   evaluated for an action/resource pair.
4. **What public-safe proof can be kept:** smoke and pressure reports can show
   IDs, decisions, source refs, and `executed=false` without exposing secrets or
   payloads.

That is the foundation. It is deliberately smaller than the whole governance
system.

## Common Review Questions

| Question | Is It Missing? | Foundation Answer |
|---|---|---|
| Does OSS support multi-agent delegation chains? | Intentionally not in v0.1.x. | S0/S1 supports one actor, one or more scoped passports, and one proposed action check. Delegation depth, authority attenuation, parent/child agent custody, and chain proof belong to S2+ / commercial ARE or a future explicitly designed OSS extension. |
| What happens after a policy or scope denial? | Not part of Foundation runtime. | Foundation returns deny/reason and stops. HITL escalation, queueing, recovery proposals, retries, and operator workflows are higher-stage governance flows. The OSS default is fail-closed. |
| Can policy be stateful or content-aware? | Only at the OPA/check boundary in v0.1.x. | The public surface can call OPA or the local stub for deterministic checks. Stateful budget limits, PII/content-aware checks, policy simulation, BYOPolicy authoring, public policy packs, and policy promotion gates are outside this OSS release. |
| How does proof avoid leaking sensitive data? | Foundation defines the safe minimum, not the full proof system. | Public proof may include request IDs, fake/demo agent/passport IDs, decisions, source refs, and aggregate metrics. It must not include raw payloads, headers, signatures, credentials, raw policy bodies, protected evidence, or private proof packets. Rich redaction/evidence replay is a higher-stage concern. |
| Does ARE learn from observability feedback? | Not in Foundation. | Foundation is a governance check layer. Live Pulse, synthetic run monitoring, observability maps, incident/recovery timelines, and feedback into future governance decisions belong to the larger platform. |
| What is the bootstrap or warm-start posture? | Local compose is fail-closed for governed calls. | Required headers, bearer auth, idempotency, route guards, and upstream checks must pass. A "bootstrap passport" design may be useful later, but it is not an accidental fail-open path in this repo. |
| Is governance a layer or a property of every agent decision? | Foundation models the layer. | S0/S1 guards action boundaries through identity, authority, and policy checks. Embedding governance into planning, context windows, tool calls, retries, and recovery is broader platform work. |
| Does this execute customer actions? | No. | Foundation checks authority and policy. Examples and proof reports keep `executed=false` and `receipt_created=false`. |
| Is this production deployment-ready? | No. | The shipped Compose stack is local development/evaluation only. See `docs/deployment-boundary.md` and `docs/dev-mode-security.md`. |
| Is governance-strata included? | No. | Governance-strata is documented as an integration concept for higher-risk transitions. Its internals are not bundled here. |

## What Belongs To The Larger ARE Platform

The larger platform can build on the S0/S1 foundation with:

- delegated authority chains
- authority attenuation
- parent/child agent spawn custody
- HITL approval and escalation
- denial recovery and proposal/check-only workflows
- BYOPolicy intake, review, generation, simulation, and promotion gates
- policy-pack benchmark and readiness evidence
- ledger/evidence replay
- sensitive proof redaction and protected evidence handling
- observability, Live Pulse, synthetic run monitoring, and incident/recovery maps
- visual RAG and operator Command Center workflows
- S2-S6 adaptive source-truth systems
- governance-strata transition orchestration

Those capabilities are intentionally not part of `are-foundation` v0.1.x.

## Practicum Scenarios To Pressure Test Later

These are good future validation scenarios, but they are not Foundation launch
requirements:

1. **Delegation chain:** Agent A spawns Agent B, Agent B calls Tool C on behalf
   of User D, and B cannot gain more scope than A.
2. **Denied mid-workflow:** an agent proposes an unapproved action and the
   system routes to deny, HITL, or recovery without executing.
3. **Sensitive proof:** a governed action over protected data produces an audit
   proof without leaking the protected data.
4. **Feedback loop:** a governed remediation is checked against observed
   resolution and future policy can account for that evidence.
5. **Bootstrap posture:** a newly booted stack can prove it is fail-closed until
   identity, passport, and policy surfaces are ready.

## How To Explain The Boundary

Use this phrasing:

> ARE Foundation is not trying to be the whole governance system. It is the
> public, inspectable base primitive. We opened S0/S1 first because identity,
> scoped authority, policy evaluation, and proof-before-execution are the
> reusable foundation. The larger ARE platform builds on that with delegation,
> HITL, BYOPolicy, recovery, visual proof, observability, governance-strata, and
> adaptive S2-S6 controls.

## Release Rule

If a future change adds any of the larger platform capabilities to this repo, it
must update this document, `docs/public-boundary.md`, the OpenAPI contract, and
the release audit before the repository is made public.
