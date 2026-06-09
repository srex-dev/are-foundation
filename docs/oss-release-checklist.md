# OSS Release Checklist

Use this checklist before changing repository visibility to public.

## 1. Boundary

- Confirm the release is S0/S1 foundation only.
- Confirm no Command Center, visual RAG, client demo frontend, BYOPolicy commercial UX, synthetic emulator, private proof packets, S2-S6 adaptive stages, or governance-strata internals are present.
- Confirm README and docs say this is a foundation runtime, not production certification or full ARE coverage.
- Confirm `docs/foundation-scope-and-limitations.md` answers delegation, denial recovery, policy expressiveness, sensitive proof, observability feedback, warm start, and governance layer/property questions.
- Confirm `docs/api-contract.md` has the security contract, a complete end-to-end example, and points to a real OPA policy.
- Confirm `docs/deployment-boundary.md` says Docker Compose is local-only and does not imply supported production hosting.
- Confirm `docs/dev-mode-security.md` documents every dev-mode flag used by Compose.
- Confirm `api/openapi.yaml` includes request/response schemas, examples, error envelopes, and the gateway/BFF health distinction.

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

## 7. Optional Homebrew Tap

Homebrew is a developer bootstrap path only; Docker Compose remains the runtime.

- Confirm the public release tag includes `bin/are-foundation`. If `v0.1.0`
  predates the helper CLI, cut `v0.1.1` or later.
- Compute the public source archive SHA:

```bash
VERSION=0.1.1
curl -L -o "are-foundation-v${VERSION}.tar.gz" \
  "https://github.com/srex-dev/are-foundation/archive/refs/tags/v${VERSION}.tar.gz"
shasum -a 256 "are-foundation-v${VERSION}.tar.gz"
```

- Create or update `srex-dev/homebrew-are`.
- Copy `packaging/homebrew/Formula/are-foundation.rb.template` to
  `Formula/are-foundation.rb` in the tap.
- Replace `__VERSION__` and `__SHA256__`.
- Test:

```bash
brew install --build-from-source ./Formula/are-foundation.rb
brew test are-foundation
are-foundation smoke
```

- Only then advertise:

```bash
brew tap srex-dev/are
brew install are-foundation
```
