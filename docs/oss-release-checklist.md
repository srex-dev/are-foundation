# OSS Release Checklist

Use this checklist before changing repository visibility to public.

## 1. Boundary

- Confirm the release is S0/S1 foundation only.
- Confirm no Command Center, visual RAG, client demo frontend, BYOPolicy commercial UX, synthetic emulator, private proof packets, S2-S6 adaptive stages, or governance-strata internals are present.
- Confirm README and docs say this is a foundation runtime, not production certification or full ARE coverage.

## 2. Hygiene

Run:

```bash
python tools/security/secret_scan.py
python tools/security/release_audit.py
git status --short --branch
```

Expected:

- scans pass
- working tree is clean
- no `.env`, `.are-gates`, `reports`, `node_modules`, `target`, generated TLS certs, or private evidence files are tracked

## 3. Gates

Run:

```bash
make test
make gate
make up
make smoke
make pressure-matrix
make down
```

Expected:

- `make smoke` returns `GREEN`
- pressure matrix returns `GREEN` or documented `WATCH`
- every report keeps `executed=false` and `receipt_created=false`

## 4. GitHub Settings

Before public launch:

- Keep default branch as `main`.
- Enable GitHub secret scanning and push protection if available.
- Enable Dependabot alerts.
- Add branch protection for `main`:
  - require Foundation CI
  - require PR review
  - disallow force-push
- Confirm SECURITY.md points to the private vulnerability reporting path.

## 5. Release

Suggested first release:

```bash
git tag -a v0.1.0 -m "ARE Foundation v0.1.0"
git push origin v0.1.0
```

Publish release notes from `docs/release-notes/v0.1.0.md`.

## 6. Visibility Flip

After all gates are green:

- Change repository visibility to public.
- Re-run GitHub Actions.
- Open the README, examples, and API spec from a logged-out browser session.
- Confirm no private links, tokens, evidence, protected policy bodies, or internal-only language appears.
