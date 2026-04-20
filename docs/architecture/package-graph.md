# Package import graph

Who depends on whom, within `internal/`. Keep this picture in mind
when refactoring — the graph is deliberately shallow and acyclic.

## Leaves (no internal deps)

These packages depend only on stdlib + third-party modules:

- **`config`** — parses TOML/YAML/JSON, validates, returns a typed
  `Config` struct. No dep on anything internal.
- **`logging`** — wraps zap. No dep on anything internal.
- **`danger`** — compiles regex patterns, returns `Matcher`. No dep
  on anything internal.
- **`redact`** — runs a list of regex replaces on a byte slice. No
  dep on anything internal.
- **`files`** — path helpers, reads + writes under the shell user's
  cwd. No dep on anything internal.

## Level 2 — depend on the leaves

- **`db`** — imports `config` (for DBPath + audit-retention) and
  `logging` (to pass a zap logger into repos).
- **`db/models`** — no internal deps; pure struct definitions.
- **`db/repo`** — imports `db/models` + `redact` (scrubs cmd and
  output before audit write) + `logging` (for the `audit` child
  logger).
- **`stream`** — imports `logging` only.

## Level 3 — shells and telegram core

- **`shell`** — imports `logging` + `config` + `db` (for the
  instance lockfile path; `shell.Manager` itself doesn't touch the
  DB at runtime but initialisation shares the state dir).
- **`telegram/common`** — imports `logging`, `redact` (for message-
  preview scrubbing), + small utility slices of `stream`.
- **`telegram/keyboards`** — pure builders; imports
  `telegram/namespaces` for prefix constants.
- **`telegram/namespaces`** — leaf; defines callback-data prefix
  constants (`j:`, `us:`, `dm:`, `pr:`, `dg:`, `ad:`).
- **`telegram/rbac`** — imports `db/models` (for the `User`
  struct) only.
- **`telegram/ratelimit`** — imports `logging`.
- **`telegram/views`** — imports `db/models` + `redact`.

## Level 4 — compositional

- **`telegram/middleware`** — imports `db/repo` (user lookup),
  `telegram/ratelimit`, `telegram/rbac`, `db/models`, `redact`.
- **`telegram/deps`** — imports pretty much everything; holds the
  dependency bundle passed to handlers.
- **`telegram/supernotify`** — imports `db/models`, `db/repo`,
  `telegram/keyboards`, `telegram/views`.
- **`telegram/flows`** — imports `telegram/deps`, `db/repo`,
  `telegram/keyboards`, `telegram/views`, `telegram/common`.
- **`telegram/commands`** — one file per command; imports
  `telegram/deps`, `db/repo`, `shell`, `stream`, `danger`,
  `telegram/common`, `telegram/keyboards`, `telegram/views`,
  `telegram/rbac`, `telegram/flows`, `telegram/namespaces`,
  `files`, `redact`.
- **`telegram/callbacks`** — inline-keyboard handlers; imports
  `telegram/deps`, `telegram/common`, `telegram/keyboards`,
  `telegram/views`, `telegram/rbac`, `telegram/flows`, `db/repo`,
  `shell`, `stream`, `telegram/supernotify`.

## Top — the bot

- **`telegram/bot.go`** — imports `telegram/middleware`,
  `telegram/commands`, `telegram/callbacks`, `telegram/deps`.
  It's the update-loop entrypoint.

## Cmd

- **`cmd/shellboto/main.go`** — imports everything: `config`, `db`,
  `db/repo`, `logging`, `shell`, `telegram`, `telegram/deps`,
  `danger`, `redact`, `stream`, `files`.
- **`cmd/shellboto/cmd_*.go`** — each subcommand imports only what
  it needs (e.g. `cmd_audit.go` doesn't import `shell` or
  `telegram`).

## ASCII picture

```
                         ┌────────────┐
                         │  main.go    │
                         └──────┬──────┘
                                │
                   ┌────────────▼────────────┐
                   │    telegram/bot.go      │
                   └──────┬──────────────────┘
                          │
      ┌───────────────────┼──────────────────────────┐
      │                   │                          │
┌─────▼──────┐    ┌──────▼──────┐           ┌───────▼────────┐
│ middleware │    │  commands/* │           │  callbacks/*   │
└─────┬──────┘    └──────┬──────┘           └───────┬────────┘
      │                  │                          │
      │          ┌───────┼──────────────────────────┘
      │          │       │
      ▼          ▼       ▼
  ┌─────────────────────────┐
  │     telegram/deps       │ ← dependency bundle
  └─────┬───────────────────┘
        │
        ├── shell ──────────┐
        ├── stream          │
        ├── danger          │
        ├── files           │
        ├── redact          │
        ├── logging         │
        ├── config          │
        └── db/repo ────────┤
                            │
                            ▼
                       db/models
                       (leaf structs)
```

## Invariants

- **No cycles.** `go vet ./...` would catch one, but more importantly
  the layering rules above make cycles structurally impossible:
  leaves → level 2 → ... → cmd.
- **`internal/` is truly internal.** No external module can import
  these. The Go compiler blocks it.
- **Domain helpers don't know about Telegram.** `shell`, `db`,
  `danger`, `redact` can be lifted into a different chat-platform
  project without editing. The Telegram-specific concerns all live
  under `internal/telegram`.
- **Handlers depend on `deps`, not the bot directly.** Handlers take
  a `*deps.Deps`, so they're unit-testable with a custom deps
  struct. The bot constructs the real one.

## Using this as a refactor compass

If you find yourself wanting:

- **A leaf package to import a level-3 package** — you have a cycle
  in progress. Split the leaf, or move one function inward.
- **`shell` or `db` to import `telegram`** — you're about to couple
  domain logic to the chat platform. Put the logic in a command or
  callback handler instead.
- **Two command handlers to share state** — that state probably
  belongs on `deps` (if long-lived) or in `telegram/flows` (if
  conversation-scoped).

Read next: [runtime-model.md](runtime-model.md).
