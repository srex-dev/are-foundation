# Model Promotion Check-Only

This example checks whether a release agent has scoped authority and policy permission for `model.promote_to_production`.

It does not promote a model.

Run the full local smoke:

```bash
make up
make smoke
```

For the step-by-step API contract with curl commands and expected responses, see
`../../docs/api-contract.md`.

The local Compose stack also loads a real OPA policy from
`../../sx/s0s1-rest-bff/policy/are_evaluatepolicy.rego`. That policy allows model
promotion only for governed `model/` resources and denies experimental model
promotion resources.

You can test the policy directly with OPA:

```bash
opa eval -d ../../sx/s0s1-rest-bff/policy/are_evaluatepolicy.rego \
  -i policy-input-allow.json "data.are.evaluatepolicy.decision"

opa eval -d ../../sx/s0s1-rest-bff/policy/are_evaluatepolicy.rego \
  -i policy-input-deny.json "data.are.evaluatepolicy.decision"
```

Expected proof basics:

- scope decision: `ALLOW`
- policy decision: `ALLOW`
- executed: `false`
- receipt created: `false`

Try the negative policy path with resource `model/experimental-candidate`; policy
should return `DENY` while still executing no customer action.
