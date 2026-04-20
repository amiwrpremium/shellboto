# Data flow

Trace a command end-to-end, from the moment the user hits Send in
Telegram to the moment the audit row is committed. Six stages.

## Stage 0 — user types a command

```
user in Telegram:   ls -la /etc/shellboto
```

Telegram packs it as an `update` with type `message` and text body,
routes it to the Bot API, which holds it in a per-bot queue until
the bot polls.

## Stage 1 — long-poll pickup

```go
// internal/telegram/bot.go — go-telegram/bot handles this transparently
for update := range longPoll.Updates() {
    dispatcher.Handle(ctx, update)
}
```

The go-telegram/bot client issues `getUpdates?offset=…&timeout=30`
in a loop. When Telegram has an update, it returns immediately;
otherwise the request holds open for 30s. Typical round-trip: a few
milliseconds after user's Send.

## Stage 2 — middleware chain

The message enters `middleware/middleware.go`'s `WrapText`:

1. **Resolve sender** — look up the telegram user ID in the `users`
   table. Unknown / inactive → write `kind=auth_reject` audit row
   (rate-limited via `auth_reject_burst` / `auth_reject_refill_per_sec`
   to prevent log-fill attacks), then silently drop. The sender
   never learns whether they're on the whitelist.
2. **Pre-auth rate-limit check** (for the auth-reject audit itself,
   not the handler). Keyed by telegram ID. Over-limit → drop
   without writing.
3. **Post-auth rate-limit check** (keyed by telegram ID, consumes
   a token from the user's bucket). Over-limit → reply with
   "rate limit" message, no handler call. `/cancel` and `/kill`
   (and their inline-button equivalents `j:c` / `j:k`) are exempt
   so you can always interrupt a runaway command.
4. **Metadata touch** — update the user's `username`, `name`,
   `last_seen_at` in the DB (best-effort; failure logged, not fatal).
5. **Strict-ASCII check** (if enabled) — reject commands containing
   non-printable-ASCII bytes with a clear error.

Middleware returns a `Ctx` with `user` and `logger` fields populated.

## Stage 3 — dispatch

`bot.go` looks at the message payload:

- Starts with `/` → `commands/*` routing (by first word, lowercased).
- Otherwise, shell dispatch in `commands/exec.go` (the "default"
  handler for any non-command text).
- File attachment → file-upload handler in `commands/` via the
  `files` package.
- Inline keyboard click → `callbacks/*` routing (by the `namespaces`
  prefix of `callback_data`).

For our example `ls -la /etc/shellboto`, it goes to
`commands/exec.go`.

## Stage 4 — the shell handler

`ExecHandler`:

1. **Danger check.** `danger.Match(cmd)` — no match for `ls`, so
   skip.
2. **Resolve shell.** `shellMgr.GetOrSpawn(userID)`:
   - If a `*Shell` exists for that userID, use it.
   - Else: allocate a pty, fork bash, set up fd 3 control pipe,
     install `PROMPT_COMMAND`, wait for the first `done:0\n`
     signal that bash is ready, audit `kind=shell_spawn`.
3. **Submit command.** `shell.Run(cmd)`:
   - Returns `ErrBusy` if a Job is already running (caller replies
     "shell busy"; no queueing).
   - Otherwise: stores the Job pointer in `shell.current.Store(j)`,
     writes `cmd + "\n"` to the pty, returns the Job.

## Stage 5 — streaming output back

`ExecHandler` hands the Job to `stream.Stream(ctx, chatID, job)`:

1. **Placeholder message.** SendMessage with a `<pre>cmd</pre>`
   header and the inline keyboard `[Cancel] [Kill]`. The returned
   message ID is cached.
2. **Edit loop.** On `edit_interval` ticks (default 1s), call
   `flush()`:
   - Snapshot the Job's buffer.
   - Render the buffer + a live-streaming trailer.
   - If the result would exceed Telegram's 4096-char cap, find the
     largest prefix ending at `\n` (fallback: space, fallback: hard
     cut) and finalise the current message; start a new one for
     the remainder.
   - EditMessage call via the Bot API. Skipped if the text is
     byte-identical to the last edit (debounce).
3. **Backchannel reads.** The shell's reader goroutine appends pty
   bytes to the Job's buffer as they arrive. The ctrl reader
   watches for `done:<exitcode>\n` on fd 3.
4. **Completion.** When bash writes `done:<exit>\n`, the ctrl reader
   fires `onCommandDone(exit)`. The shell waits a 30 ms drain
   window so any in-flight pty bytes land in the buffer, then
   calls `Job.finish(exit)` which closes `Job.Done`.
5. **Final flush.** `Stream` detects `<-Job.Done`, calls `flush()`
   one last time with the full buffer + the footer:
   `✅ exit 0 · 42ms` (or `❌ exit N` / `⚠ interrupted` /
   `⚠ output capped`), strips the inline keyboard.
6. **Multi-message spill** — if the stream rolled over into
   additional messages, `Stream` additionally uploads the full
   captured output as `output-<unix-ts>.txt` so the user has the
   unwrapped version too.

## Stage 6 — audit write

After `Stream` returns, the handler:

1. Compute `output_sha256` over the (redacted) output bytes.
2. Compute gzipped blob (if `audit_output_mode` allows).
3. Acquire the `audit.Log` mutex (serialises the whole chain).
4. Read the previous row's `row_hash` (or the genesis seed for the
   first row).
5. Build a canonical row struct: ts (UTC RFC3339Nano), userID, kind,
   cmd, exit, bytes_out, duration_ms, termination, danger_pattern,
   detail, output_sha256.
6. `row_hash = sha256(prev_hash || canonical_json(row))`.
7. INSERT into `audit_events` (and `audit_outputs` if output stored).
8. Write a mirror log line through the `audit` zap logger → journald.
9. Release the mutex.

Full detail on the hash chain: [../security/audit-chain.md](../security/audit-chain.md).

## Sequence diagram

```
user     Telegram     longpoll    middleware    dispatch    shell     pty+bash      stream    audit
 │           │            │            │            │          │           │            │        │
 │  send cmd │            │            │            │          │           │            │        │
 │──────────▶│            │            │            │          │           │            │        │
 │           │  update    │            │            │          │           │            │        │
 │           │───────────▶│            │            │          │           │            │        │
 │           │            │  WrapText  │            │          │           │            │        │
 │           │            │───────────▶│            │          │           │            │        │
 │           │            │            │  Ctx,user  │          │           │            │        │
 │           │            │            │───────────▶│          │           │            │        │
 │           │            │            │            │ GetOrSpawn│           │            │        │
 │           │            │            │            │─────────▶│           │            │        │
 │           │            │            │            │          │ Run(cmd)  │            │        │
 │           │            │            │            │          │──────────▶│            │        │
 │           │            │            │            │          │           │ write pty  │        │
 │           │            │            │            │          │           │◀───────────│        │
 │           │            │            │            │          │           │ output ...  │        │
 │           │            │ Stream(job)│            │          │           │───────────▶│        │
 │           │            │                         │  edit    │           │            │        │
 │           │◀────────────────────────edit msg────│──────────────────────────────────│        │
 │           │            │                         │          │           │ done:N     │        │
 │           │            │                         │          │           │──────────▶│        │
 │           │            │                         │          │           │            │ Log    │
 │           │            │                         │          │           │            │──────▶│
 │           │            │                         │                                            │
 │           │            │                         │  final edit: exit footer                   │
 │           │◀────────────────────────edit msg────│──────────────────────────────────────────│
```

## What happens when the user hits `/cancel`

Same path up through dispatch. The `CancelHandler`:

1. Looks up the user's shell. If no active Job → replies "nothing
   to cancel" (no signal sent).
2. `shell.SigInt()` — writes `0x03` to the pty. Line discipline
   forwards SIGINT to the foreground pgid.
3. Audit `kind=command_run` (same kind as normal commands) with
   `termination="canceled"`.

The running `stream.Stream` goroutine sees the Job finish and
updates the final footer to `⚠ interrupted`.

`/kill` is the same, but calls `SigKill()` instead — ioctl to find
the foreground pgid, SIGKILL to the group. Refuses to signal bash
itself (would leave the shell headless); `/reset` is the tool for
that.

## Concurrency boundaries

- **Shell level.** One Job at a time. `shell.Run` takes
  `currentMu.Lock()`, checks `current.Load()`, returns `ErrBusy`
  if non-nil.
- **Audit level.** One writer at a time. `audit.Log` takes
  `logMu.Lock()` around the read-previous-row + insert sequence —
  guarantees the hash chain is always linear.
- **Manager level.** `shells` map mutex is held only for
  lookup / insert / delete; not during actual pty I/O.
- **Streamer level.** Each streamer runs in its own goroutine; no
  shared state with other streamers.

## Read next

- [concurrency.md](concurrency.md) — the exact rules each goroutine
  and each mutex honours.
- [design-decisions.md](design-decisions.md) — why we picked this
  data-flow shape.
