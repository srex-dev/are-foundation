# Deployment Boundary

ARE Foundation ships a local Docker Compose runtime for development,
evaluation, and public-safe smoke/pressure testing. It does not ship a supported
production deployment stack in v0.1.x.

This is intentional.

## Shipped In OSS

- `deploy/compose/docker-compose.yml`
- generated local mTLS certificates via `make certs`
- local Postgres
- local OPA
- local JWKS nginx
- S0/S1 REST BFF
- S0/S1 gateway
- public-safe smoke, gate, and pressure scripts

The local gateway listens on `http://localhost:18085` in Compose so it does not
collide with a full ARE developer stack.

## Not Shipped In v0.1.x

- Helm charts
- Terraform modules
- Kubernetes manifests
- OpenShift manifests
- cloud provider deployment recipes
- production ingress configuration
- managed identity provider setup
- managed secret storage
- HA database or broker topology
- production certificate rotation
- production observability dashboards
- commercial ARE Command Center deployment

Those artifacts would imply support guarantees and operational boundaries this
foundation repo is not claiming yet.

## If You Adapt It Beyond Local

Treat the Compose file as a reference topology, not a hardened deployment. A
real environment should provide:

- TLS-only public ingress
- release build without the `dev` tag
- no insecure test token
- no plaintext conformance listeners
- authenticated or private metrics scraping
- real JWT/JWKS validation with rotation
- secrets manager-backed database credentials
- private network access to Postgres, OPA, JWKS, BFF, and health ports
- persistent database backups if you keep records
- rate limits and request-size limits at the ingress
- log redaction for payloads, headers, signatures, credentials, and raw policy bodies
- explicit operational ownership for patching base images

## Public API Ports In Compose

| Purpose | Compose Port | Notes |
|---|---:|---|
| Gateway API listener | `18085` | Local dev HTTP because `ARE_GW_ENABLE_INSECURE_CONFORMANCE=true`. |
| Gateway health listener | `18086` | Exposes `/healthz` and `/readyz` locally. |
| Gateway metrics listener | `18090` | Anonymous only because local compose enables it. |
| S0/S1 REST BFF mTLS | `8443` | Internal service port; gateway calls it with local mTLS. |
| S0/S1 REST BFF health | `8099` | Direct service health endpoint. |
| Postgres | `54329` | Local-only database. |
| Local JWKS | `8088` | Local-only static JWKS. |
| OPA | `8181` | Local-only policy engine. |

## Commercial Platform Boundary

The full commercial ARE platform has additional deployment concerns: Command
Center, visual RAG, BYOPolicy workflows, advanced source-truth maps, private
proof packet storage, synthetic/emulator evidence, S2-S6 adaptive systems, and
governance-strata transition orchestration.

Those are outside this OSS deployment boundary.
