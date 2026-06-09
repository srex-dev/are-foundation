# API Contract

ARE Foundation treats the API contract as part of the product. A developer
should be able to read `api/openapi.yaml`, run the local stack, and integrate
against the S0/S1 authority path without guessing what is safe or required.

## Contract Guarantees

- The gateway accepts only the public S0/S1 routes listed in `api/openapi.yaml`.
- Non-foundation routes fail closed.
- POST/check paths require an idempotency key.
- Scope and policy checks are check-only and do not execute customer actions.
- Public proof basics use safe IDs, decisions, source refs, and
  `executed=false`.
- Public outputs must not include raw payloads, bearer tokens, credentials,
  signatures, raw headers, protected evidence, or raw policy bodies.

## Security Contract

Every gateway request to the governed S0/S1 surface requires:

| Header | Required | Purpose |
|---|---:|---|
| `Authorization: Bearer <token>` | Yes | Authenticates the caller at the gateway. Local Compose supports `Bearer test-token` only because it is built with dev-mode flags. |
| `X-Request-ID` | Yes | Correlates logs, proof summaries, and client retries. |
| `X-ARE-Agent-ID` | Yes | Attributes the calling operator/client actor. This header is not a substitute for scoped passport authority. |
| `Idempotency-Key` | POST/check paths | Protects register, issue, verify, scope, and policy checks from duplicate side effects during retries. |
| `Content-Type: application/json` | JSON bodies | Required when a body is sent. |

Local Compose deliberately enables a dev-only token and anonymous metrics. See
`docs/dev-mode-security.md` before adapting the stack beyond localhost.

## End-To-End Model Promotion Check

This flow is the smallest useful integration:

```text
register release actor
  -> issue model promotion passport
  -> verify passport
  -> evaluate scope
  -> evaluate OPA policy
  -> return public-safe proof basics
```

It does not promote a model.

### 1. Start The Local Stack

```bash
make certs
make up
```

The local gateway listens at `http://localhost:18085`.

### 2. Shared Curl Settings

```bash
BASE=http://localhost:18085
TOKEN=test-token
REQ=demo-model-promotion-001
CALLER=demo-operator
```

### 3. Register The Actor

```bash
curl -sS -X POST "$BASE/v1/identity/agents" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Request-ID: $REQ-register" \
  -H "X-ARE-Agent-ID: $CALLER" \
  -H "Idempotency-Key: demo-agent-model-promotion" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_type": "demo.release-agent",
    "owner_id": "demo-owner",
    "metadata": {
      "example": "model-promotion-check-only",
      "public": true
    }
  }'
```

Expected shape:

```json
{
  "agent": {
    "agent_id": "agt-demo-agent-model-promotion",
    "agent_type": "demo.release-agent",
    "owner_id": "demo-owner",
    "status": "ACTIVE"
  }
}
```

### 4. Issue Scoped Authority

```bash
curl -sS -X POST "$BASE/v1/passports" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Request-ID: $REQ-passport" \
  -H "X-ARE-Agent-ID: $CALLER" \
  -H "Idempotency-Key: demo-passport-model-promotion" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agt-demo-agent-model-promotion",
    "passport_type": "standard",
    "requested_scopes": [
      {
        "action_class": "model.promote_to_production",
        "resource_pattern": "model/*"
      }
    ],
    "ttl_seconds": 3600,
    "issued_by": "demo-owner",
    "reason": "public foundation model promotion check"
  }'
```

Expected shape:

```json
{
  "passport": {
    "passport_id": "ppt-demo-passport-model-promotion",
    "agent_id": "agt-demo-agent-model-promotion",
    "passport_type": "standard",
    "scope_set": [
      {
        "scope_id": "scp-demo-passport-model-promotion-0",
        "action_class": "model.promote_to_production",
        "resource_pattern": "model/*"
      }
    ],
    "status": "ACTIVE",
    "issued_by": "demo-owner"
  }
}
```

### 5. Verify Passport Binding

```bash
curl -sS -X POST "$BASE/v1/passports:verify" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Request-ID: $REQ-verify" \
  -H "X-ARE-Agent-ID: $CALLER" \
  -H "Idempotency-Key: demo-verify-model-promotion" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agt-demo-agent-model-promotion",
    "passport_id": "ppt-demo-passport-model-promotion"
  }'
```

Expected result:

```json
{
  "verified": true,
  "reason": "passport verified",
  "passport_id": "ppt-demo-passport-model-promotion",
  "agent_id": "agt-demo-agent-model-promotion",
  "executed": false,
  "proof_status": "reference_only"
}
```

### 6. Evaluate Scope

```bash
curl -sS -X POST "$BASE/v1/enforcement/scope:evaluate" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Request-ID: $REQ-scope" \
  -H "X-ARE-Agent-ID: $CALLER" \
  -H "Idempotency-Key: demo-scope-model-promotion" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agt-demo-agent-model-promotion",
    "passport_id": "ppt-demo-passport-model-promotion",
    "action_class": "model.promote_to_production",
    "resource": "model/champion"
  }'
```

Expected result:

```json
{
  "decision": {
    "effect": "ALLOW",
    "reason": "scope matched requested passport",
    "agent_id": "agt-demo-agent-model-promotion",
    "action_class": "model.promote_to_production",
    "resource": "model/champion"
  },
  "executed": false
}
```

### 7. Evaluate Policy

```bash
curl -sS -X POST "$BASE/v1/policy/evaluations" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Request-ID: $REQ-policy" \
  -H "X-ARE-Agent-ID: $CALLER" \
  -H "Idempotency-Key: demo-policy-model-promotion" \
  -H "Content-Type: application/json" \
  -d '{
    "decision_id": "demo-model-promotion-policy",
    "agent_id": "agt-demo-agent-model-promotion",
    "action_class": "model.promote_to_production",
    "resource": "model/champion"
  }'
```

Expected result:

```json
{
  "decision": {
    "decision_id": "demo-model-promotion-policy",
    "effect": "ALLOW",
    "reason": "model promotion policy passed for governed model resource"
  }
}
```

### 8. Negative Policy Check

```bash
curl -sS -X POST "$BASE/v1/policy/evaluations" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Request-ID: $REQ-policy-deny" \
  -H "X-ARE-Agent-ID: $CALLER" \
  -H "Idempotency-Key: demo-policy-model-promotion-deny" \
  -H "Content-Type: application/json" \
  -d '{
    "decision_id": "demo-model-promotion-policy-deny",
    "agent_id": "agt-demo-agent-model-promotion",
    "action_class": "model.promote_to_production",
    "resource": "model/experimental-candidate"
  }'
```

Expected result:

```json
{
  "decision": {
    "decision_id": "demo-model-promotion-policy-deny",
    "effect": "DENY",
    "reason": "model promotion requires a governed non-experimental model resource"
  }
}
```

## Real OPA Policy

The local Compose stack loads the policy at
`sx/s0s1-rest-bff/policy/are_evaluatepolicy.rego`.

The model-promotion rule allows only:

- `action_class == "model.promote_to_production"`
- non-empty `agent_id`
- resources under `model/`
- resources that do not contain `experimental`

It denies experimental model promotion resources and still keeps the API
check-only. Scope evaluation must pass separately; policy does not replace
passport authority.

The policy is intentionally simple, but it is real Rego loaded by OPA in the
local runtime rather than just a README snippet.

## Error Semantics

| Condition | HTTP | Shape |
|---|---:|---|
| Missing request ID | `400` | plain text from gateway or JSON error from BFF direct calls |
| Missing bearer token | `401` | plain text from gateway |
| Missing idempotency key on POST/check path | `400` | plain text from gateway or JSON error from BFF direct calls |
| Unknown foundation record | `404` | JSON error envelope |
| Idempotency key reused with different register body | `409` | JSON error envelope |
| Policy engine unavailable | `502` | JSON error envelope |
| Policy dependency timeout | `504` | JSON error envelope |
| Non-foundation route | `404` | route guard message |

## Generated Clients

Use `api/openapi.yaml` as the source for client generation. The API is small on
purpose; consumers should model register, issue, verify, scope, and policy as
one authority-check workflow rather than independent dashboard calls.
