# Build from source

## Quick build

```bash
make build
```

Produces `bin/shellboto` with the version stamp:

```
$ bin/shellboto -version
shellboto version=v0.1.0 gitSHA=abcdef0 built=2026-04-20T15:00:00Z
```

The version stamp is computed from `git describe --tags --always`
at make time.

Plain `go build` also works:

```bash
go build ./cmd/shellboto
```

But the resulting binary reports `version=dev` (no ldflags). Use
`make build` for anything going out.

## Stripped binary

```bash
make build-stripped
```

`-s -w` ldflags — smaller binary (~30% reduction), no debug
symbols. Goes to `bin/shellboto-stripped`.

Used internally by goreleaser for release artifacts.

## All the make targets

| Target | What it does |
|--------|--------------|
| `all` / `build` | `bin/shellboto` with version stamp |
| `build-stripped` | Stripped variant |
| `test` | `go test -count=1 -timeout 60s ./...` |
| `fmt` | `gofmt` + `goimports -local github.com/amiwrpremium/shellboto` |
| `lint` | `golangci-lint run --timeout=3m` |
| `vet` | `go vet ./...` |
| `vuln` | `govulncheck ./...` |
| `version` | `build` + `bin/shellboto -version` |
| `help-cli` | `build` + `bin/shellboto help` |
| `tarball` | Project tarball at `../shellboto.tar.gz` |
| `clean` | `rm -rf bin dist` |
| `hooks-install` | Wire lefthook into `.git/hooks` |
| `hooks-uninstall` | Remove lefthook hooks |
| `release-snapshot` | `goreleaser release --snapshot --clean` (no publish) |
| `release-check` | `lint + test + vet + vuln + goreleaser check` |
| `test-deploy` | Run `deploy/lib_test.sh` |
| `install` | `sudo ./deploy/install.sh` |
| `uninstall` | `sudo ./deploy/uninstall.sh` |
| `rollback` | `sudo ./deploy/rollback.sh` |
| `help` | Show all targets with one-line descriptions |

## Tests

Run everything:

```bash
make test
```

That includes:

- All Go unit + integration tests under `internal/*/` and
  `cmd/*/`.
- Real pty tests under `internal/shell/`. These fork real bash
  processes; Linux-only.
- Real SQLite tests under `internal/db/...`. `t.TempDir()`
  isolation.

Run a single package:

```bash
go test ./internal/danger/
```

With `-v` for per-test output:

```bash
go test -v ./internal/config/
```

With `-race` (CI always uses this):

```bash
go test -race ./...
```

## Linting

```bash
make lint
```

Or equivalent incremental (only reports new issues since last
commit, the pre-commit-hook mode):

```bash
golangci-lint run --new-from-rev=HEAD --timeout=2m
```

`.golangci.yml` profile explained in
[linting.md](linting.md).

## Vulnerability scan

```bash
make vuln
```

Runs `govulncheck ./...`. Checks the code against the Go
vulnerability database. Fatal on any hit so you don't ship a
known CVE.

## Release snapshot (locally)

```bash
make release-snapshot
```

Runs goreleaser in `--snapshot --clean` mode. Produces everything
a real release would (cross-platform binaries, .deb/.rpm, SBOMs,
checksums) under `dist/`. Does NOT publish to GitHub or push to
Homebrew tap.

Takes 1–2 minutes.

## Full pre-release gate

```bash
make release-check
```

Runs: `lint`, `test`, `vet`, `vuln`, `goreleaser check`. If any
fails, you're not shipping.

CI runs the same checks on every PR.

## CGO is disabled

The project is pure-Go. Do not introduce CGO dependencies:

- Causes cross-compile complications.
- Requires a C toolchain per target OS.
- Binary no longer self-contained.

The one C lib we'd want (SQLite) is replaced with
`modernc.org/sqlite` — pure Go.

If you feel you need a CGO dep, open an issue first.

## Reproducible builds

`make build` is nearly reproducible — the only non-determinism is
the build timestamp (`-X main.built=...`). To get identical hashes
across two builds of the same commit, pin the timestamp:

```bash
BUILT="2026-04-20T00:00:00Z" make build
```

goreleaser handles this in release mode automatically.

## Reading the code

- `Makefile` — all targets, ldflags, verbose output.
- `scripts/install-dev-tools.sh` — dep manager.

## Read next

- [testing.md](testing.md) — the testing philosophy.
- [hooks.md](hooks.md) — what gets run automatically.
