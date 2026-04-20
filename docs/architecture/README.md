# Architecture

How shellboto is built, from 30-second overview to package-level
detail. Read top-to-bottom if you're new, or jump to what you need.

| File | What it covers |
|------|----------------|
| [overview.md](overview.md) | ASCII block diagram, one-paragraph summary, who-does-what at a glance |
| [stack.md](stack.md) | Language, runtime, dependencies (Go stdlib, GORM, SQLite, zap, creack/pty, go-telegram/bot), what's pinned, what's pure-Go |
| [project-layout.md](project-layout.md) | Tour of every top-level directory (`cmd/`, `internal/`, `deploy/`, `packaging/`, `scripts/`, `.github/`) |
| [package-graph.md](package-graph.md) | Import dependency graph across `internal/*` — which packages depend on which |
| [runtime-model.md](runtime-model.md) | Process model: single binary, one long-running goroutine set, one pty per user, state in SQLite |
| [data-flow.md](data-flow.md) | Trace a Telegram message end-to-end: update → middleware → handler → shell → stream → audit → reply |
| [concurrency.md](concurrency.md) | Goroutines, channels, locks — the concurrency contract each package honours |
| [design-decisions.md](design-decisions.md) | Why pty, why fd-3 control pipe, why hash-chained audit, why single binary, why no Docker image, why Go |

## Quick mental model

```
┌────────────────────────────────────────────────────────────┐
│                    Telegram cloud                           │
└────────────────────────────────────────────────────────────┘
                    ▲        │
          Bot API   │        │  long-poll getUpdates
          HTTPS     │        │
                    │        ▼
    ┌───────────────────────────────────────┐
    │            shellboto process           │   single Go binary
    │  ┌──────────────────────────────────┐  │   running under systemd
    │  │ telegram  ← middleware ← dispatch │  │
    │  │   │                               │  │
    │  │   ▼                               │  │
    │  │  shell  (per-user pty + bash)     │  │
    │  │   │                               │  │
    │  │   ▼                               │  │
    │  │  stream  (edit-loop to Telegram)  │  │
    │  │   │                               │  │
    │  │   ▼                               │  │
    │  │  audit  (SQLite + hash chain)     │  │
    │  └──────────────────────────────────┘  │
    └───────────────────────────────────────┘
                    │
                    ▼
        /var/lib/shellboto/state.db   ←  SQLite (GORM), 0600
        /etc/shellboto/{env,config}   ←  operator-managed, 0600
        journald                       ←  all logs + audit mirror
```

One binary. One database file. One systemd unit. Everything else
(init scripts, Docker, k8s) is optional.

## Key invariants

- **One pty per Telegram user.** A user's shell persists across
  messages until idle-reap, `/reset`, or service restart.
- **One command at a time per pty.** A second command arriving while
  one is running is rejected (not queued) so a user always knows
  what their shell is doing.
- **All audit writes serialised.** A mutex around the hash-chain
  Append guarantees linear ordering — no races can fork the chain.
- **Bot token never reaches a user shell.** The `sanitizedEnv`
  helper strips `SHELLBOTO_*` env vars before `execve`-ing bash.
- **Single binary / no CGO.** `bin/shellboto` is the entire runtime.
  Copy it to any x86_64 or arm64 Linux box and run it.

Read [overview.md](overview.md) next.
