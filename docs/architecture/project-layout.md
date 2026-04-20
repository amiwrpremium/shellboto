# Project layout

A tour of every top-level directory. Pulled from the actual tree on
`master`, not aspirational.

```
shellboto/
├── cmd/
│   └── shellboto/           # main binary + ops subcommands
├── internal/
│   ├── config/              # multi-format config loader
│   ├── logging/             # zap logger + audit mirror
│   ├── db/                  # SQLite + GORM + hash-chained audit
│   ├── danger/              # regex-based dangerous-command matcher
│   ├── redact/              # secret scrubber (keys, tokens, passwords)
│   ├── files/               # file upload / download helpers
│   ├── stream/              # Telegram message-edit stream buffer
│   ├── shell/               # pty + bash subprocess management
│   └── telegram/            # bot core
│       ├── bot.go
│       ├── commands/        # / slash commands
│       ├── callbacks/       # inline-keyboard callbacks
│       ├── common/          # handler helpers
│       ├── deps/            # dependency-injection holder
│       ├── flows/           # multi-step conversation state machines
│       ├── keyboards/       # inline-keyboard builders
│       ├── middleware/      # auth, rate limit, metadata touch
│       ├── namespaces/      # callback-data prefix registry
│       ├── ratelimit/       # token-bucket per user
│       ├── rbac/            # Can{Promote,Demote,Act...} policy
│       ├── supernotify/     # admin fan-out notifier
│       └── views/           # markdown-safe view builders
├── deploy/
│   ├── install.sh           # interactive + -y installer
│   ├── uninstall.sh
│   ├── rollback.sh
│   ├── lib.sh               # shared bash helpers (color, prompt, trap)
│   ├── lib_test.sh          # shell unit tests for lib.sh
│   ├── shellboto.service    # systemd unit
│   ├── config.example.toml  # canonical config template
│   ├── config.example.yaml
│   ├── config.example.json
│   ├── env.example          # env file template
│   └── init/
│       ├── openrc/shellboto # OpenRC init script
│       ├── runit/shellboto/{run,log/run}
│       └── s6/shellboto/{run,type}   # execlineb, not sh
├── packaging/
│   ├── postinstall.sh       # nfpm .deb/.rpm post-install
│   ├── preremove.sh         # nfpm .deb/.rpm pre-remove
│   ├── homebrew/shellboto.rb # Homebrew formula scaffold
│   └── README.md
├── scripts/
│   ├── install-dev-tools.sh # bootstrap dev tooling (lefthook etc.)
│   ├── commit-msg-check.sh  # Conventional Commits regex
│   └── lefthook-drift-check.sh
├── docs/                    # this tree
├── .github/
│   ├── SETTINGS.md          # one-time GitHub UI checklist
│   ├── dependabot.yml
│   ├── PULL_REQUEST_TEMPLATE.md
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug.md
│   │   └── feature.md
│   └── workflows/
│       ├── ci.yml
│       ├── release.yml
│       ├── codeql.yml
│       ├── release-please.yml
│       └── dependabot-auto-merge.yml
├── .lefthook.yml            # pre-commit / commit-msg / pre-push hooks
├── .golangci.yml            # linter config (v2 schema)
├── .gitleaks.toml           # secret-scan rules
├── .yamllint                # relaxed yamllint defaults
├── .goreleaser.yaml         # release matrix + packaging rules
├── .release-please-manifest.json
├── release-please-config.json
├── .chglog/                 # (retired; release-please owns CHANGELOG now — kept only in git history)
├── go.mod
├── go.sum
├── Makefile
├── LICENSE                  # MIT
├── README.md
├── CONTRIBUTING.md
└── CHANGELOG.md             # maintained by release-please
```

## `cmd/shellboto/`

The single `package main`. Everything here is either the bot
entrypoint or an ops subcommand.

| File | Purpose |
|------|---------|
| `main.go` | Bot mode: loads config, opens DB, takes instance flock, spawns shell manager, runs Telegram long-poll, handles graceful shutdown |
| `cli.go` + `cli_test.go` | Subcommand dispatcher. First non-flag arg picks the subcommand |
| `cmd_doctor.go` | `shellboto doctor` — preflight checks |
| `cmd_config.go` | `shellboto config check` — validate a config file |
| `cmd_audit.go` | `shellboto audit verify|search|export` |
| `cmd_audit_replay.go` | `shellboto audit replay` — cross-check journald ↔ DB |
| `cmd_db.go` | `shellboto db backup|info|vacuum` |
| `cmd_users.go` | `shellboto users list|tree` |
| `cmd_simulate.go` | `shellboto simulate <cmd>` — dry-run the danger matcher |
| `cmd_mintseed.go` | `shellboto mint-seed` — print fresh 32-byte hex |
| `cmd_service.go` | `shellboto service <verb>` — systemd passthrough |
| `cmd_completion.go` | `shellboto completion bash|zsh|fish` |

See [../reference/cli.md](../reference/cli.md) for flag-by-flag
detail.

## `internal/`

Everything under `internal/` is importable only within this module —
Go's compiler enforces that. Consumer code of shellboto is `cmd/`
only.

- **`config/`** — TOML/YAML/JSON loader. Env-var overrides. Custom
  `Duration` unmarshalling so `5m` / `30s` work across all three
  formats.
- **`logging/`** — one zap logger constructor, plus a
  `logger.Named("audit")` child used to mirror every audit write to
  stderr → journald.
- **`db/`** — GORM + SQLite setup, auto-migrator, instance flock,
  audit repo, user repo. `models/` holds the typed row structs;
  `repo/` holds the queries.
- **`danger/`** — built-in regex table + `ExtraDangerPatterns`
  merge. `Matcher.Match(cmd)` returns the first matched pattern.
- **`redact/`** — a pattern list that scrubs secrets from a byte
  slice (used on cmd + output before audit write) and a terminal-
  escape stripper (so audit blobs don't contain cursor-addressing
  bytes that would confuse `less`).
- **`files/`** — `Upload(path, body)` / `Download(path)` with
  path hardening.
- **`stream/`** — Telegram message-edit state machine. Debounces,
  rolls over at 4096 chars, spills to `output.txt` upload.
- **`shell/`** — pty allocation, fd-3 control pipe, PROMPT_COMMAND
  dispatcher, SIGINT/SIGKILL, idle-reap manager, `SpawnOpts` for
  dropping privileges.
- **`telegram/`** — the bulk of the code. `bot.go` owns the
  long-poll loop and graceful-shutdown channel; every handler
  lives under its subdirectory (`commands/`, `callbacks/`,
  `flows/`). `middleware/` composes the auth + ratelimit stack;
  `views/` turns domain objects into Telegram-ready markdown;
  `rbac/` is the capability policy (who can do what).

## `deploy/`

Operator-facing scripts. Pure bash, shellcheck-clean, sourced
helpers live in `lib.sh`.

Three user-facing scripts:

- **`install.sh`** — 7-step idempotent installer. See
  [../deployment/installer.md](../deployment/installer.md).
- **`uninstall.sh`** — safe-by-default removal; audit DB deletion
  gated behind typed confirmation or magic flag.
- **`rollback.sh`** — atomic binary swap with `.prev`.

Plus the systemd unit and the templates (config.example.* and
env.example) that the installer materialises.

Under `init/`: OpenRC / runit / s6 init scripts for non-systemd
deployments. Covered in [../deployment/](../deployment/).

## `packaging/`

Consumed by goreleaser at release time:

- `postinstall.sh` — runs when a `.deb` / `.rpm` installs.
  `systemctl daemon-reload`, copies `config.example.toml` →
  `/etc/shellboto/config.toml` if absent (upgrade-safe), prints
  first-use hints.
- `preremove.sh` — stops and disables the service cleanly before
  package removal. Preserves `/etc/shellboto` and
  `/var/lib/shellboto`.
- `homebrew/shellboto.rb` — formula scaffolding. The real `url` +
  `sha256` are filled in by goreleaser per release before the push
  to the tap repo.

## `scripts/`

Dev-loop helpers. Not installed to the target host.

- `install-dev-tools.sh` — idempotent bootstrap of lefthook,
  golangci-lint, goreleaser, gitleaks, govulncheck, goimports, syft.
- `commit-msg-check.sh` — regex validator used by the `commit-msg`
  hook.
- `lefthook-drift-check.sh` — warns on `post-merge` / `post-checkout`
  if `.lefthook.yml` changed since last checkout.

## `.github/`

Everything for the GitHub-hosted side: issue + PR templates, the
one-time UI checklist, dependabot config, CI + release workflows.

Detail in [../development/ci.md](../development/ci.md).

## `docs/`

You are here.

## Top-level single files

- **`Makefile`** — the build/test/release interface. See
  [../development/build-from-source.md](../development/build-from-source.md).
- **`.lefthook.yml`** — commit/push hook definitions. See
  [../development/hooks.md](../development/hooks.md).
- **`.golangci.yml`** — linter profile. See
  [../development/linting.md](../development/linting.md).
- **`.gitleaks.toml`** — secret-scan rules. Has a Telegram-bot-token
  rule + allowlist entries for fixtures. See
  [../security/secret-redaction.md](../security/secret-redaction.md).
- **`.yamllint`** — relaxed yamllint defaults for GitHub Actions'
  `on:` key, long URLs, document-start.
- **`.goreleaser.yaml`** — release matrix + .deb/.rpm contents +
  Homebrew. See [../packaging/goreleaser.md](../packaging/goreleaser.md).
- **`release-please-config.json`** + **`.release-please-manifest.json`**
  — release-please v4 driver. See [../development/releasing.md](../development/releasing.md).

Read next: [package-graph.md](package-graph.md).
