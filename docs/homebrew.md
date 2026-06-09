# Homebrew Developer Bootstrap

Homebrew support is for local developer bootstrap only. It installs the
`are-foundation` helper CLI and uses Docker Compose for the runtime.

The runtime remains Docker-based:

```bash
are-foundation up
are-foundation smoke
are-foundation pressure
are-foundation down
```

## Intended Public Install

After a public release tag and release tarball SHA are available, publish a
formula in a tap such as `srex-dev/homebrew-are`:

```bash
brew tap srex-dev/are
brew install are-foundation
are-foundation up
are-foundation smoke
```

Until that tap exists, developers can run from a checkout:

```bash
./bin/are-foundation up
./bin/are-foundation smoke
```

## Public Switch-On Steps

Use these steps after `srex-dev/are-foundation` is public. Do not point the tap
at a tag that does not include `bin/are-foundation`. If `v0.1.0` was cut before
the helper CLI landed, cut `v0.1.1` or later and use that tag.

Set the release version:

```bash
VERSION=0.1.1
```

Create or confirm the release tag:

```bash
git status --short --branch
git tag -a "v${VERSION}" -m "ARE Foundation v${VERSION}"
git push origin "v${VERSION}"
```

Download the public GitHub source archive and compute the SHA:

```bash
curl -L -o "are-foundation-v${VERSION}.tar.gz" \
  "https://github.com/srex-dev/are-foundation/archive/refs/tags/v${VERSION}.tar.gz"
shasum -a 256 "are-foundation-v${VERSION}.tar.gz"
```

Create the tap repo if it does not exist:

```bash
gh repo create srex-dev/homebrew-are --public --description "Homebrew tap for ARE developer tools"
git clone https://github.com/srex-dev/homebrew-are.git
cd homebrew-are
mkdir -p Formula
```

Copy the template into the tap:

```bash
cp ../are-foundation/packaging/homebrew/Formula/are-foundation.rb.template \
  Formula/are-foundation.rb
```

Replace placeholders in `Formula/are-foundation.rb`:

- `__VERSION__` -> release version without the leading `v`, for example `0.1.1`
- `__SHA256__` -> the `shasum -a 256` value

Test the formula locally:

```bash
brew install --build-from-source ./Formula/are-foundation.rb
brew test are-foundation
are-foundation version
are-foundation help
```

Push the tap:

```bash
git add Formula/are-foundation.rb
git commit -m "Add are-foundation formula"
git push origin main
```

Then update the public README install block only after this works:

```bash
brew tap srex-dev/are
brew install are-foundation
are-foundation up
are-foundation smoke
```

## What The CLI Does

- Finds the ARE Foundation checkout or Homebrew install root.
- Generates local development certificates.
- Starts or stops the Docker Compose stack.
- Runs smoke, pressure, pressure matrix, gate, and release-audit helpers.
- Prints the OpenAPI path.

It does not install or manage production services, execute customer actions, or
replace Docker Compose.

## Formula Publication Checklist

1. Make `srex-dev/are-foundation` public.
2. Tag a release that includes `bin/are-foundation`. Prefer `v0.1.1` or later if
   `v0.1.0` predates the helper CLI.
3. Download the GitHub source archive for that tag.
4. Compute `sha256`.
5. Copy `packaging/homebrew/Formula/are-foundation.rb.template` into the tap as
   `Formula/are-foundation.rb`.
6. Replace:
   - `__VERSION__`
   - `__SHA256__`
7. Run:

```bash
brew install --build-from-source ./Formula/are-foundation.rb
brew test are-foundation
```

8. Commit and push the tap.
