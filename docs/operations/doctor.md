# `shellboto doctor`

Preflight / ongoing health checks. Designed to be runnable in seconds.

## Usage

```bash
sudo shellboto doctor
```

## What it checks

From `cmd/shellboto/cmd_doctor.go`:

- **Config file** — parses cleanly, validates (db_path non-empty,
  log_format legal, audit_output_mode legal).
- **Environment** —
  - `SHELLBOTO_TOKEN` is set (non-empty).
  - `SHELLBOTO_SUPERADMIN_ID` is a positive int64.
  - `SHELLBOTO_AUDIT_SEED` is set and is 64 hex chars (64 → 32
    bytes). If empty: warn (dev mode).
- **State directory** —
  - `/var/lib/shellboto/` exists.
  - Is readable/writable by the current process.
- **DB path** —
  - Parent directory exists.
  - If DB file exists, it opens + answers a trivial query.
- **user_shell_user** (if set) —
  - Unix account resolves via `user.Lookup`.
  - UID > 0 (not root).
  - Home base dir exists.

## Output

```
shellboto doctor
----------------
✅  config file /etc/shellboto/config.toml  parses
✅  SHELLBOTO_TOKEN  set
✅  SHELLBOTO_SUPERADMIN_ID  123456789
✅  SHELLBOTO_AUDIT_SEED  set (64 hex chars)
✅  state dir /var/lib/shellboto/  exists, 0700 root:root
✅  db open  state.db readable
✅  user_shell_user shellboto-user  uid=998 gid=998 home=/home/shellboto-user
----------------
all checks passed.
```

Or with issues:

```
shellboto doctor
----------------
✅  config file /etc/shellboto/config.toml  parses
✅  SHELLBOTO_TOKEN  set
✅  SHELLBOTO_SUPERADMIN_ID  123456789
⚠   SHELLBOTO_AUDIT_SEED  not set — using zero-seed fallback (dev mode only)
✅  state dir /var/lib/shellboto/  exists
❌  user_shell_user bad-acct  no such user
----------------
1 failure, 1 warning.
```

## Exit codes

- `0` — all checks pass (warnings are OK; warnings don't fail).
- `3` — at least one check failed.

## When to run

- **After install.** Installer runs it automatically as the last
  step.
- **After any config / env edit.** Before `systemctl restart`.
- **In monitoring.** Scheduled runs; alert on exit 3.
- **During incidents.** First thing to check.

## Cron example

```
# /etc/cron.d/shellboto-doctor
*/15 * * * * root /usr/local/bin/shellboto doctor > /dev/null 2>&1 \
    || echo "shellboto doctor failed on $(hostname)" | mail -s "URGENT" you@you.net
```

Or use a proper monitoring tool (Prometheus blackbox, Nagios
NRPE, etc.) that can wrap the exit code.

## What doctor does NOT check

- **That the bot token is actually valid.** doctor doesn't talk
  to Telegram; a revoked token looks fine from the bot's env.
  You'd notice via `journalctl` (the long-poll returning 401).
- **That the audit chain is unbroken.** Use `shellboto audit verify`
  for that.
- **That users can actually message the bot.** Send a `/start`
  yourself.
- **File system space / inode pressure.** OS-level monitoring is
  your job.

## Reading the code

- `cmd/shellboto/cmd_doctor.go`

## Read next

- [logs.md](logs.md) — the other primary signal.
- [monitoring.md](monitoring.md) — wiring doctor into your alerts.
