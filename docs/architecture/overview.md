# Architecture overview

shellboto is a single Go binary that:

1. Long-polls Telegram's Bot API for updates addressed to its bot
   token.
2. For each update, runs it through a stack of middleware
   (whitelist check, rate limit, metadata touch).
3. Dispatches to a handler based on message type (text command,
   slash command, callback query, file upload).
4. If the handler wants a shell, allocates or reuses a per-user
   pty-backed bash subprocess.
5. Streams the command's output back to Telegram by editing a single
   message in place; spills to `output.txt` when the 4096-char cap
   is exceeded.
6. Writes an append-only audit row after each interesting event,
   linked to the previous row via SHA-256 hash chain.

No external processes, no daemons, no sidecars. One Go binary
running under systemd, reading/writing `/var/lib/shellboto/state.db`.

## Block diagram

```
                       Telegram cloud
                       ▲             │
           updates     │             │  replies / edits
                       │             ▼
         ┌───────────────────────────────────────────────┐
         │                shellboto                        │
         │                                                 │
         │   ┌─────────────────────────────────────────┐  │
         │   │  internal/telegram/bot.go               │  │
         │   │  ─ long-poll loop                       │  │
         │   └────────────┬────────────────────────────┘  │
         │                │ Update                        │
         │   ┌────────────▼────────────────────────────┐  │
         │   │  middleware                              │  │
         │   │  ─ whitelist (users table)               │  │
         │   │  ─ rate limit (pre-auth + post-auth)     │  │
         │   │  ─ metadata touch / audit on reject      │  │
         │   └────────────┬────────────────────────────┘  │
         │                │ Ctx + user                    │
         │   ┌────────────▼────────────────────────────┐  │
         │   │  dispatch  →  commands/* or callbacks/*  │  │
         │   └────────────┬────────────────────────────┘  │
         │                │                               │
         │   ┌────────────▼────────────────────────────┐  │
         │   │  shell.Manager.GetOrSpawn(userID)        │  │
         │   │    ─ pty + bash                          │  │
         │   │    ─ fd-3 control pipe                   │  │
         │   │    ─ PROMPT_COMMAND dispatcher           │  │
         │   └────────────┬────────────────────────────┘  │
         │                │ Job (stdout bytes)            │
         │   ┌────────────▼────────────────────────────┐  │
         │   │  stream.Streamer                         │  │
         │   │    ─ edit-loop @ edit_interval           │  │
         │   │    ─ spill to output.txt @ 4096 chars    │  │
         │   └────────────┬────────────────────────────┘  │
         │                │                               │
         │   ┌────────────▼────────────────────────────┐  │
         │   │  db/repo.AuditRepo.Log                   │  │
         │   │    ─ redact(cmd, output)                 │  │
         │   │    ─ sha256(prev_hash‖canonical(row))   │  │
         │   │    ─ journald mirror (zap "audit")       │  │
         │   └──────────────────────────────────────────┘ │
         │                                                 │
         └───────────────────────────────────────────────┘
                    │
                    ▼
     /var/lib/shellboto/state.db            (audit + users, SQLite)
     /etc/shellboto/{env,config.toml}       (operator inputs, 0600)
     /etc/shellboto/shellboto.lock           (flock — instance lock)
     journald                                 (logs + audit mirror)
```

## One paragraph summary

A user sends a Telegram message. The bot's long-poll loop receives
the update, middleware confirms the sender is on the whitelist and
under rate limits, dispatch routes to the right handler, the handler
(if it needs a shell) asks the shell manager for the user's pty,
writes the command to it, a streamer goroutine edits a Telegram
message with output bytes as they arrive, and when bash's
`PROMPT_COMMAND` writes `done:<exitcode>\n` to fd 3, the handler
finalises the message and writes an audit row whose hash chains back
to the previous one.

## Who does what

| Concern | Location |
|---------|----------|
| Long-poll loop | `internal/telegram/bot.go` |
| Update validation + dispatch | `internal/telegram/middleware/` + `commands/` + `callbacks/` |
| Auth + RBAC | `internal/telegram/rbac/` + `internal/db/repo/users.go` |
| Rate limiting | `internal/telegram/ratelimit/` |
| Inline-keyboard flows (danger confirm, user-management) | `internal/telegram/callbacks/` + `internal/telegram/flows/` |
| Admin fan-out notifications | `internal/telegram/supernotify/` |
| Per-user shells + pty | `internal/shell/` |
| Streaming output to Telegram | `internal/stream/` |
| Persistent state | `internal/db/` (GORM, SQLite) |
| Audit log + hash chain | `internal/db/repo/audit.go` |
| Secret redaction | `internal/redact/` |
| Dangerous-command matching | `internal/danger/` |
| Config loading | `internal/config/` |
| Logging | `internal/logging/` |
| File up/download | `internal/files/` |
| Process entrypoint + ops CLI | `cmd/shellboto/` |
| Installer + uninstaller + rollback | `deploy/` |
| OS packaging | `packaging/` + `.goreleaser.yaml` |
| Hooks, CI, docs | `.lefthook.yml`, `.github/`, `docs/` |

## What's deliberately **not** here

- **No web UI / HTTP server.** Telegram is the only interface.
- **No IPC between multiple shellboto processes.** An `flock`
  prevents two instances from racing on the audit DB.
- **No external message broker.** Fan-out is in-process goroutines +
  channels.
- **No Docker image.** Pure binary + systemd unit by design; Docker
  is deferrable operator choice.
- **No separate admin UI.** All admin ops are either Telegram
  commands or `shellboto <subcommand>` on the host.

Read next: [stack.md](stack.md).
