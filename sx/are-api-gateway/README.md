# ARE Foundation API Gateway

Public S0/S1 gateway perimeter for ARE Foundation.

Responsibilities:

- validate bearer JWTs or the local dev `test-token`
- require request and agent headers
- forward only foundation routes to the S0/S1 REST BFF
- fail closed for non-foundation routes
- expose health and Prometheus metrics

Supported REST routes are listed in `api/openapi.yaml`.

Commercial/operator surfaces are not part of this gateway export.

## Local

```bash
go test ./...
```

## Compose

From the repository root:

```bash
make certs
make up
make smoke
```

