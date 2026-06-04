# policyfixtures (iter-002)

**Single source** for **EvaluatePolicy** stub test vectors used by:

- `sx/are-api-gateway/internal/gateway` (`StaticUpstreamClient.handleEvaluatePolicy`)
- `sx/s0s1-rest-bff/internal/policy` (`Stub` and HTTP tests)

## Contract

[`vectors.json`](vectors.json) is embedded via `//go:embed` and must stay aligned with:

- `sx/s0s1-rest-bff/policy/are_evaluatepolicy.rego` (OPA path)
- The **demo** action classes in both stubs: `demo.forbidden_action` (DENY, reason contains **forbidden**), `demo.read` (ALLOW), and resource **`timeout://…`** for dependency timeout (504 / `ErrDependencyTimeout` in BFF).

**Canonical row names** for tests: `demo_forbidden_deny`, `demo_read_allow`, `dependency_timeout`, `contract_shape_generic_read`.

## Usage

```go
import "github.com/srex-dev/are-foundation/tools/testing/policyfixtures"

v := policyfixtures.Must("demo_forbidden_deny")
body, _ := v.PolicyEvaluationBody("")
```

## Tests

- Package tests: `go test ./tools/testing/policyfixtures/...` from repo root
- Module consumers: `are-api-gateway` and `are-s0s1-rest-bff` (both `replace` this module in `go.mod`)
