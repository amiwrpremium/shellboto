# Telegram commands

Every slash command the bot accepts, grouped by role.

For the single-page cheat sheet, see
[../reference/telegram-commands.md](../reference/telegram-commands.md).

## Conventions

- **Rate-limit exempt:** `/cancel`, `/kill` (and inline buttons
  `j:c`, `j:k`). See [../security/rate-limiting.md](../security/rate-limiting.md).
- **Audit:** every command writes an audit row (see the full list
  of event kinds in [../reference/audit-kinds.md](../reference/audit-kinds.md)).
- **Role-gated commands** return a terse "you don't have permission"
  reply if a lower-privilege caller tries them.

## Everyone

### `/start`

- Registers first-contact metadata (username, name) in `users`
  table.
- Replies with a short greeting + hint to try a command.
- If caller isn't on the whitelist, the middleware short-circuits
  before this handler runs (silent auth_reject).

### `/help`

- Lists available commands filtered by caller's role.
- Non-admins don't see admin-only commands in the output.

### `/status`

- Reports the caller's per-user shell state:
  - `idle` + last-activity timestamp + shell uptime, or
  - `running` + current command + elapsed time + byte count.
- If no shell exists yet, says so.

### `<any text>` (send a command)

- Text not starting with `/` is treated as a shell command.
- Runs through `danger.Match` — a hit triggers the confirm flow
  (see [callbacks-and-flows.md](callbacks-and-flows.md)).
- Otherwise: `shellMgr.GetOrSpawn(userID).Run(cmd)` then
  `stream.Stream(chatID, job)`.
- Audit: `kind=command_run` on completion, with exit code,
  duration, bytes, termination reason, output SHA-256.

### `/cancel`

- If a Job is running: `shell.SigInt()` sends Ctrl+C to the pty.
  Caller gets a "⚠ canceling" reply.
- If idle: "nothing to cancel".
- **Rate-limit exempt.**
- Audit: `kind=command_run`, `termination=canceled`.

### `/kill`

- If a Job is running: `shell.SigKill()` — ioctl to find foreground
  pgid, SIGKILL to group. Refuses to signal bash itself.
- If idle: "nothing to kill".
- **Rate-limit exempt.**
- Audit: `kind=command_run`, `termination=killed`.

### `/reset`

- Closes the caller's current pty (via `Shell.Close`) — bash
  process group is SIGTERM'd then SIGKILL'd after 2s.
- Next message spawns a fresh shell with current config (e.g. new
  privileges after a role change).
- Audit: `kind=shell_reset`.

### `/get <path>`

- Reads `<path>` from the VPS (path-hardened — no `..`, no
  unauthorised absolute paths for `role=user`; see
  [file-transfer.md](file-transfer.md)).
- Uploads the bytes to Telegram as a File (max 50 MB per Bot API).
- Audit: `kind=file_download`.

### `/auditme`

- Shows caller's own last 10 audit rows (kind, cmd preview, exit,
  ts). No output blobs.
- Format: one row per line, Markdown-safe.

### File attachment (paperclip → File, not Photo)

- Optional caption: destination path (absolute or relative to
  shell cwd).
- Downloads the file from Telegram, writes to VPS.
- Audit: `kind=file_upload`.

## admin+ (admin or superadmin)

### `/users`

- Opens the user-browser inline-keyboard (see
  [callbacks-and-flows.md](callbacks-and-flows.md#users-browser)).
- Shows every active user: role, name, telegram_id.
- Tap a user → per-user action menu (Profile / Promote / Demote /
  Ban / ...).

### `/adduser <id> [user|admin]`

- Starts the add-user wizard (multi-step flow).
- First step: confirm the target ID resolves via Bot API
  (`GetChat`) — catches typos.
- Second step: prompt for a friendly name (regex `^[A-Za-z ]+$`).
  Invalid name = auto-ban attempt.
- Third step: inline Confirm / Cancel button.
- On confirm: `userRepo.Add(...)`, audit `kind=user_added`,
  supernotify the superadmin.
- `admin` role argument ignored for admin callers; only superadmin
  can create admins directly via adduser.

### `/deluser <id>`

- Admin can delete only `role=user`; superadmin can delete any
  non-superadmin.
- Shows inline Confirm button first.
- On confirm: `userRepo.SoftDelete(...)`, audit
  `kind=user_removed`, supernotify.
- Target's active shell is auto-closed.

### `/audit [N]`

- Shows the last N audit rows across all users (default 20, max
  50).
- Same format as `/auditme` but unfiltered.

### `/audit-out <event_id>`

- Fetches the captured output blob for event `<event_id>`.
- Decompresses, sends as text if it fits in one message, else as
  an `output.txt` file upload.
- Only works if `audit_output_mode` stored the blob (see
  [../configuration/audit-output-modes.md](../configuration/audit-output-modes.md)).

### `/audit-verify`

- Triggers `auditRepo.Verify()` — the hash-chain walker.
- Replies:
  - `✅ audit chain OK — N rows verified`, or
  - `❌ audit chain BROKEN at row <id> — <reason>`.
- See [../security/audit-chain.md](../security/audit-chain.md).

## superadmin only

### `/role <id> admin|user`

- Promote (`user → admin`) or demote (`admin → user`).
- Target's active pty shell is auto-closed on both directions.
- Audit: `kind=role_changed`, detail includes old/new roles.
- Supernotify fires.

## How commands are actually routed

`telegram/bot.go` registers handlers via the
`bot.MatchTypeExact` / `MatchTypeCommand` matchers from
`go-telegram/bot`:

- `/start` via the bot's default matcher.
- All `/cmd ...` entries via `bot.MatchTypeCommand`.
- Everything else (text not starting with `/`) via a default
  handler that points at `commands/exec.go`.

Non-command text without an active handler flow (e.g. while the
add-user wizard is waiting for a name) is intercepted by the
`flows` registry first; if no flow claims it, it falls through to
`exec`.

## What's not a command

- **Group messages.** By default, bots only see messages
  addressed to them in groups (see [../getting-started/create-telegram-bot.md](../getting-started/create-telegram-bot.md)).
  shellboto is designed for private DMs; group use-cases work but
  aren't the primary path.
- **Inline mode** (`@yourbot query...` in any chat). Not
  implemented.
- **Payments** / web-app mini-apps. Not implemented.

## Read next

- [callbacks-and-flows.md](callbacks-and-flows.md) — the
  interactive side.
- [../reference/telegram-commands.md](../reference/telegram-commands.md)
  — tabular cheat sheet.
