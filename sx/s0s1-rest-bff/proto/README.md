# Contract surface

This service implements the **OpenAPI v1** S0/S1 REST slice behind `are-api-gateway` (`RegisterAgent`, `GetAgent`, `IssuePassport`, `EvaluatePolicy`). It does not publish a separate gRPC package here.

**no concrete standalone protobuf schema** — contract checks use this README as the documented exception (see `tools/testing/check_contract_surface.py`).
