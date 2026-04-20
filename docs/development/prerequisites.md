# Prerequisites

## Required

- **Go 1.26+** — exact pin in `go.mod` (`go 1.26.0` +
  `toolchain go1.26.2`). Install from https://go.dev/doc/install.
- **make** — GNU make.
- **bash** — `deploy/lib.sh` uses bash-isms (`read -rs`, arrays).
- **git** — for `git describe` version stamp + history-aware
  tooling.

## Recommended (installed by `./scripts/install-dev-tools.sh`)

- **lefthook** — commit / commit-msg / pre-push hooks.
- **golangci-lint v2.11+** — linter. Note: `./scripts/install-dev-tools.sh`
  runs `go install` against your local Go, so the installed binary
  is compiled under Go 1.26 (correct). CI installs it via the
  `golangci/golangci-lint-action@v9` with `install-mode: goinstall`.
- **goreleaser v2+** — cross-platform build / release config check.
- **gitleaks** — secret scanning in pre-commit.
- **govulncheck** — pre-push vulnerability scan.
- **goimports** — import grouping.
- **syft** — SBOM generator (used by goreleaser).

All of these are installed into `$GOBIN` (typically
`$HOME/go/bin`). Make sure that's on your `PATH`:

```bash
export PATH="$PATH:$HOME/go/bin"
```

## OS packages (for CI parity)

- **shellcheck** — shell-script linter.
- **yamllint** — YAML linter.

On Debian/Ubuntu:

```bash
sudo apt install shellcheck yamllint
```

On Fedora:

```bash
sudo dnf install ShellCheck yamllint
```

On macOS:

```bash
brew install shellcheck yamllint
```

## First-time setup

```bash
./scripts/install-dev-tools.sh
```

Idempotent. Re-run any time to refresh the Go-based tools to the
latest (or per-project-pinned) versions.

Then wire hooks:

```bash
make hooks-install
```

This calls `lefthook install`, which writes `.git/hooks/pre-commit`
etc. that delegate to `lefthook run`.

## Verification

```bash
go version          # go1.26.X
make --version
lefthook version
golangci-lint version
shellcheck --version
yamllint --version
```

## OS support for development

- **Linux** — primary. All tests run here.
- **macOS** — supported for most things. The `shell` package's
  tests require features that only work on Linux ptys (we have
  build tags; macOS builds skip them). `make test` passes on
  macOS with some tests skipped.
- **Windows** — unsupported. WSL2 works if you must.

## Read next

- [build-from-source.md](build-from-source.md) — actually build it.
- [hooks.md](hooks.md) — what the hooks do.
