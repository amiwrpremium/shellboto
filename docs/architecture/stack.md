# Technology stack

## Language and runtime

- **Go 1.26.** `go.mod` pins `go 1.26.0` with `toolchain go1.26.2`.
- **Pure Go; CGO disabled.** `.goreleaser.yaml` sets
  `CGO_ENABLED=0` for every build target. SQLite uses
  `modernc.org/sqlite` (a CGO-free transpiled port), not
  `mattn/go-sqlite3`. Result: one static binary, no glibc
  dependency, copies to any Linux host.
- **Linux + macOS cross-compile.** Release matrix:
  `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`.
  macOS builds run the CLI subcommands but not the full bot (the
  `shell` package uses Linux-specific ptrace-adjacent syscalls for
  pty + pgid control).

## Build tooling

- **Make.** `Makefile` targets: `build`, `build-stripped`, `test`,
  `fmt`, `lint`, `vet`, `vuln`, `version`, `tarball`, `clean`,
  `help`, `help-cli`, `hooks-install`, `hooks-uninstall`,
  `release-snapshot`, `release-check`, plus `install`, `uninstall`,
  `rollback` shims.
- **goreleaser.** Cross-platform builds, .deb/.rpm (via embedded
  nfpm), Homebrew tap formula, CycloneDX SBOMs via syft, checksums,
  GitHub release notes grouped by Conventional Commit type.
- **goinstall fallback for CI.** `.github/workflows/ci.yml` uses
  `install-mode: goinstall` to compile golangci-lint under the
  runner's Go 1.26 — prebuilt binaries are still Go-1.24-built.

## Key Go dependencies

| Module | Why | Pinned at |
|--------|-----|-----------|
| `gorm.io/gorm` | ORM for SQLite: migrations, simple queries, transaction wrapper | `v1.32.1` |
| `gorm.io/driver/sqlite` | GORM ↔ modernc SQLite bridge | `v1.6.0` |
| `modernc.org/sqlite` | Pure-Go SQLite (transitive via the driver) | driver's lockfile |
| `github.com/creack/pty` | Allocate pseudo-terminal, set window size, get fd | `v1.1.24` |
| `github.com/go-telegram/bot` | Telegram Bot API client: long poll, typed updates, inline keyboards | `v1.20.0` |
| `go.uber.org/zap` | Structured logging (JSON under systemd, console for dev) | `v1.27.1` |
| `golang.org/x/sys` | `syscall.Credential`, ioctls (TIOCGPGRP for pgid lookup), pty resize | `v0.43.0` |
| `github.com/BurntSushi/toml` | TOML config parser | `v1.6.0` |
| `gopkg.in/yaml.v3` | YAML config parser | `v3.0.1` |
| (std `encoding/json`) | JSON config parser | stdlib |

`go mod tidy -diff` runs on every commit (lefthook pre-commit), so
`go.mod` and `go.sum` stay clean.

## Why these over alternatives

- **`modernc.org/sqlite` over `mattn/go-sqlite3`.** No CGO. The
  binary cross-compiles without a C toolchain per target. Perf is
  slightly worse for very write-heavy workloads, but we insert
  O(commands-per-minute) — nowhere near where it matters.
- **`go-telegram/bot` over `go-telegram-bot-api/telegram-bot-api`.**
  The latter is inactive; the former is actively maintained, types
  all the Bot API's inline-keyboard shapes, and has context.Context
  support throughout.
- **`creack/pty` over spawning `script`/`unbuffer`.** Direct
  `openpty`-backed primitives. Gets us the pty fd for `ioctl`
  calls (cursor-window-size resize, pgid query) without subprocess
  shell-outs.
- **`zap` over stdlib `log/slog`.** slog landed in 1.21; zap has
  more mileage, better perf, and structured fields with types.
  No strong reason to migrate.
- **GORM over raw `database/sql`.** We only use it for migrations +
  trivial CRUD. Raw SQL would save a dep, but GORM's migrator
  halves the schema-evolution code.

## Datastore

- **SQLite.** Embedded. Single file at `/var/lib/shellboto/state.db`
  (chmod 0600 on open).
- **Two tables:** `users` (whitelist + role + promoted_by) and
  `audit_events` (every interesting event, hash-chained).
- **WAL mode** via the modernc driver's default PRAGMAs.
- **Instance lock** — `flock(LOCK_EX)` on
  `/var/lib/shellboto/shellboto.lock` prevents two shellboto
  processes from racing on the audit chain.

Full schema: [../database/schema.md](../database/schema.md).

## External integrations

- **Telegram Bot API** (HTTPS, outbound only).
- **systemd / journald** (for service management and log ingestion).
- **No other outbound network**. No telemetry, no update-check, no
  crash reporter.

## File-system footprint

| Path | Owner | Mode | Purpose |
|------|-------|------|---------|
| `/usr/local/bin/shellboto` | root:root | 0755 | the binary |
| `/usr/local/bin/shellboto.prev` | root:root | 0755 | previous binary (for `rollback.sh`) |
| `/etc/shellboto/` | root:root | 0700 | config dir |
| `/etc/shellboto/env` | root:root | 0600 | secrets: token, superadmin id, audit seed |
| `/etc/shellboto/config.{toml,yaml,json}` | root:root | 0600 | runtime config |
| `/etc/systemd/system/shellboto.service` | root:root | 0644 | unit file |
| `/var/lib/shellboto/` | root:root | 0700 | state dir (created by `StateDirectory=` in unit) |
| `/var/lib/shellboto/state.db` | root:root | 0600 | SQLite file |
| `/var/lib/shellboto/shellboto.lock` | root:root | 0600 | instance flock |

See also [../reference/file-paths.md](../reference/file-paths.md).

## Test stack

- **Unit + integration tests.** `go test ./...` with a real
  temporary SQLite via `newTestRepo(t)` helpers — mocks of the DB
  are deliberately not used (`CONTRIBUTING.md` enforces this).
- **Shell-pty tests.** `internal/shell/shell_test.go` spawns real
  bash processes with real ptys under `t.TempDir()`. Tests verify
  boundary detection, SIGINT handling, idle reaping.
- **Deploy script tests.** `deploy/lib_test.sh` unit-tests the bash
  helpers in `deploy/lib.sh` with shellcheck + manual asserts.
- **Danger matcher, redactor, audit canonical form.** All have
  exhaustive table-driven tests.

CI runs all of these on every PR — [../development/ci.md](../development/ci.md).

## What's **not** in the stack

- **No ORM migration tool other than GORM's auto-migrator.** Schema
  changes are additive-only; we don't use Goose / Atlas / Flyway.
- **No dependency injection framework.** Wire, uber/fx, etc. — none
  used. `main.go` composes dependencies explicitly.
- **No actor framework.** Goroutines + channels, no ergo / gosiris.
- **No RPC / gRPC.** In-process only.

Read next: [project-layout.md](project-layout.md).
