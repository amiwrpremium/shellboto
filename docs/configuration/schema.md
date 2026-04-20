# Config schema reference

Every key the config file accepts, in the order they appear in
`deploy/config.example.toml`. Grouped by concern.

For the short tabular version, see
[../reference/config-keys.md](../reference/config-keys.md).

## Storage

### `db_path`

- **Type:** string
- **Default:** `/var/lib/shellboto/state.db`
- **Meaning:** SQLite state file. systemd's `StateDirectory=shellboto`
  creates `/var/lib/shellboto` on service start; the bot chmods the
  DB file to 0600 on open. Parent must exist and be writable by
  root.
- **Validation:** non-empty string. Path resolved relative to
  process cwd, which under systemd is `/`.

### `audit_retention`

- **Type:** duration string (`"2160h"` = 90 days)
- **Default:** `2160h` (90 days)
- **Meaning:** audit rows older than this are deleted by the hourly
  pruner goroutine. Chain continuity is preserved — the verify
  walker detects "pruned" and checks only the surviving chain.
- **Trade-off:** shorter = smaller DB, less forensic window.
  Longer = more storage (dominated by captured output blobs).

## Command semantics

### `default_timeout`

- **Type:** duration
- **Default:** `5m`
- **Meaning:** per-command hard timeout. After this, SIGINT is sent
  (SIGTERM equivalent via tty line discipline); after `kill_grace`
  more, SIGKILL.
- **Trade-off:** longer for legit long-running operations (build,
  migration). Shorter for tighter reaction when a command hangs.

### `kill_grace`

- **Type:** duration
- **Default:** `5s`
- **Meaning:** after `default_timeout` fires SIGINT, how long to
  wait before escalating to SIGKILL.

### `max_output_bytes`

- **Type:** int (bytes)
- **Default:** `52428800` (50 MiB)
- **Meaning:** per-command in-memory output buffer cap. When
  exceeded, the foreground process is SIGKILL'd and partial output
  is kept (Job.Truncated() returns true). Protects against
  `cat /dev/urandom` / `yes` style OOM.
- **`0`** = unlimited. Not recommended.

### `strict_ascii_commands`

- **Type:** bool
- **Default:** `false`
- **Meaning:** when true, any command with bytes outside printable
  ASCII (plus tab/newline/CR) is rejected with a clear error.
  Guards against unicode homoglyphs and control bytes in the audit
  log. Breaks legitimate unicode commands (`echo café`, i18n files)
  — leave off unless you're sure.

### `queue_while_busy`

- **Type:** bool
- **Default:** `true`
- **Meaning:** **reserved**. Currently "reject with a note" is the
  only path; queuing isn't implemented. The flag is in the schema
  so adding queueing later doesn't become a breaking change.

## Streaming + Telegram

### `max_message_chars`

- **Type:** int
- **Default:** `4096`
- **Meaning:** cap per Telegram message. Telegram's hard limit is
  4096 UTF-16 code units (counted post HTML-escape). When an edit
  would exceed this, shellboto rolls over to a new message at the
  last `\n` (fallback: last space; fallback: hard cut).
- **Don't raise this above 4096.** Telegram will reject the edit.

### `edit_interval`

- **Type:** duration
- **Default:** `1s`
- **Meaning:** streamer edits the Telegram message every
  `edit_interval` while output flows in. Shorter = snappier feedback
  but more Bot API calls; longer = smoother but the user waits.

### `heartbeat`

- **Type:** duration
- **Default:** `30s`
- **Meaning:** send a "still running" heartbeat edit every N
  seconds while a command is running but producing no output. Keeps
  Telegram's "typing..." indicator alive and reassures the user.

## Shell lifecycle

### `idle_reap`

- **Type:** duration
- **Default:** `1h`
- **Meaning:** per-user shells idle (no input or output) longer
  than this are closed by the reaper. Frees memory + slot in the
  shell manager. User's next command spawns a fresh shell.
- **Trade-off:** longer = preserves state across long waits;
  shorter = lower idle footprint.

## Audit output

### `audit_output_mode`

- **Type:** enum `always` | `errors_only` | `never`
- **Default:** `always`
- **Meaning:** whether captured command stdout/stderr is persisted
  in `audit_outputs`:
  - **`always`** — every command's output stored (most forensic).
  - **`errors_only`** — stored only when `exit_code != 0` (less
    privacy exposure).
  - **`never`** — never stored. Audit row still records metadata
    and a SHA-256 of the output.
- See [audit-output-modes.md](audit-output-modes.md) for detailed
  trade-offs.

### `audit_max_blob_bytes`

- **Type:** int (bytes)
- **Default:** `52428800` (50 MiB)
- **Meaning:** post-redact, pre-store cap on the gzipped output
  blob. If exceeded, the blob is dropped; the audit row still has
  metadata + `detail: output_oversized`.
- **`0`** = no separate cap (rely only on `max_output_bytes`).

## Danger matcher

### `extra_danger_patterns`

- **Type:** list of strings (regex)
- **Default:** `[]`
- **Meaning:** operator-supplied regex patterns merged with the
  built-in defaults. See
  [../security/danger-matcher.md](../security/danger-matcher.md)
  for the built-in list.
- **Validation:** each pattern must compile as a Go `regexp`.
  Failure at startup with a clear error + the pattern that failed.

### `confirm_ttl`

- **Type:** duration
- **Default:** `60s`
- **Meaning:** how long a danger-confirm token is valid. After
  this, the ✅ Run button no longer works; audit row
  `kind=danger_expired` fires.

## Logging

### `log_format`

- **Type:** enum `json` | `console`
- **Default:** `json`
- **Meaning:** zap output format. `json` is what you want under
  systemd (journald parses it). `console` is coloured text for dev.

### `log_level`

- **Type:** enum `debug` | `info` | `warn` | `error`
- **Default:** `info`
- **Meaning:** zap minimum level. `debug` is verbose; enable only
  to chase a specific issue.

### `log_rejected`

- **Type:** bool
- **Default:** `false`
- **Meaning:** when true, log pre-auth rejected updates to journald.
  Even when false, they land in the DB as `auth_reject` audit rows
  — but the journald log gets noisy fast if the bot's username is
  crawled by bots.

## Non-root shells

### `user_shell_user`

- **Type:** string (unix username)
- **Default:** `""`
- **Meaning:** unix account that `role=user` pty shells run as.
  Empty = dev mode (all shells run as root) with a startup warning.
  Set to a non-root username in production if you're whitelisting
  anyone other than yourself.
- See [non-root-shells.md](non-root-shells.md) for setup details
  (including the symlink-attack mitigation).

### `user_shell_home`

- **Type:** string (filesystem path)
- **Default:** `""` → `/home/<user_shell_user>`
- **Meaning:** base directory for per-telegram-user home subdirs.
  Each user gets `<user_shell_home>/<telegram_id>` (mode 0700,
  chown to `user_shell_user`). Parent must be root-owned 0755 to
  prevent symlink attacks.

## Rate limiting

### `rate_limit_burst`

- **Type:** int
- **Default:** `10`
- **Meaning:** post-auth token-bucket capacity, per Telegram user.
  Every non-exempt command/callback spends 1 token. `/cancel` and
  `/kill` are exempt so users can always interrupt.
- **`0`** = disabled.

### `rate_limit_refill_per_sec`

- **Type:** float
- **Default:** `1.0`
- **Meaning:** refill rate in tokens/sec. Default settles at 60
  actions/minute per user; bursts up to `rate_limit_burst` allowed.

### `auth_reject_burst`

- **Type:** int
- **Default:** `5`
- **Meaning:** pre-auth rate limiter on **writing auth_reject audit
  rows**. Keyed by Telegram From-id regardless of whitelist
  membership. Without this, an attacker spamming the bot fills the
  audit DB.
- **`0`** = disabled. **Do not disable in production.**

### `auth_reject_refill_per_sec`

- **Type:** float
- **Default:** `0.05` (one row per 20s steady-state)
- **Meaning:** refill rate for the pre-auth bucket. At default, a
  single attacker writes ≈ 4300 rows/day (~2 MB audit DB/day).

## Admin fan-out

### `super_notify_action_ttl`

- **Type:** duration
- **Default:** `10m`
- **Meaning:** lifetime of inline-keyboard buttons on superadmin
  DM notifications before a sweeper strips them via
  EditMessageReplyMarkup. Prevents an old "demote Alice" button
  from being tapped hours later by mistake.
- **`0`** = TTL disabled (buttons persist until tapped).

## Read next

- [environment.md](environment.md) — the three env vars (token,
  superadmin id, audit seed).
- [../reference/config-keys.md](../reference/config-keys.md) —
  single-page tabular cheat sheet.
