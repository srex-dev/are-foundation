# Security Policy

Please report suspected vulnerabilities privately to security@srex.dev or to the repository maintainers through GitHub Security Advisories.

Do not open public issues containing tokens, credentials, raw payloads, private policy bodies, protected evidence, or proof material.

## Supported Surface

This repository supports the public S0/S1 foundation surface only. It is a local/developer foundation runtime and does not claim production readiness, legal certification, or full ARE governance coverage.

Supported release line:

| Version | Supported |
|---|---|
| 0.1.x | Yes |

## Secrets

Never commit:

- API keys or bearer tokens
- Private keys or credentials
- Raw headers or signatures
- Protected evidence bodies
- Private `.are-gates` proof archives

## Public Reports

Public-safe reports may contain request IDs, agent IDs, passport IDs, decisions, source refs, aggregate latency, and aggregate error counts.

They must not contain raw customer payloads, protected evidence, credentials, signatures, raw policy bodies, private proof packet contents, or bearer tokens.
