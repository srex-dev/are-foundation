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

After the repository is public and a release tarball SHA is available, publish a
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
2. Tag a release that includes `bin/are-foundation`.
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

