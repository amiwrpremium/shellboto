# File paths the bot touches

Every path shellboto reads from or writes to.

## Operator-managed

| Path | Mode | Owner | Purpose |
|------|:----:|-------|---------|
| `/etc/shellboto/` | 0700 | root:root | config dir |
| `/etc/shellboto/env` | 0600 | root:root | secrets (token, superadmin id, audit seed) |
| `/etc/shellboto/config.toml` (or `.yaml` / `.json`) | 0600 | root:root | runtime config |

Set up by `install.sh`. Read at startup. **Not** modified by the
bot at runtime.

## Bot-managed

| Path | Mode | Owner | Purpose |
|------|:----:|-------|---------|
| `/var/lib/shellboto/` | 0700 | root:root | state dir (created by systemd's `StateDirectory=`) |
| `/var/lib/shellboto/state.db` | 0600 | root:root | SQLite — users + audit |
| `/var/lib/shellboto/state.db-wal` | 0600 | root:root | SQLite write-ahead log |
| `/var/lib/shellboto/state.db-shm` | 0600 | root:root | SQLite shared memory |
| `/var/lib/shellboto/shellboto.lock` | 0600 | root:root | flock for instance-singleton |

Created and maintained by the bot.

## Binary

| Path | Mode | Owner | Purpose |
|------|:----:|-------|---------|
| `/usr/local/bin/shellboto` | 0755 | root:root | the executable |
| `/usr/local/bin/shellboto.prev` | 0755 | root:root | previous version (rollback target) |

`install.sh` writes these. `rollback.sh` swaps them.

## Service

| Path | Mode | Owner | Purpose |
|------|:----:|-------|---------|
| `/etc/systemd/system/shellboto.service` | 0644 | root:root | systemd unit |

For OpenRC: `/etc/init.d/shellboto`. For runit:
`/etc/sv/shellboto/{run,log/run}`. For s6: `/etc/s6-rc/source/shellboto/`.

## User-shell home dirs

When `user_shell_user` is configured:

| Path | Mode | Owner | Purpose |
|------|:----:|-------|---------|
| `/home/shellboto-user/` (or whatever `user_shell_home` says) | 0750 | root:`<group>` | parent home base — operator-set |
| `/home/shellboto-user/<telegram_id>/` | 0700 | `<user>:<group>` | per-user shell home (auto-created on first command) |

The parent must NOT be writable by `<user>` — see
[../configuration/non-root-shells.md](../configuration/non-root-shells.md)
for the symlink-attack rationale.

## Telegram-handled (transient)

When users `/get` a file or upload one:

- **Upload**: file lands in the user's pty cwd or the
  caption-specified path. Mode 0600, owner = the shell user.
- **Download**: the file is sent to Telegram; nothing persists
  on the VPS beyond the read.

## Process-internal

- **`/tmp/`** — bot doesn't write to /tmp at runtime (some Go
  stdlib pieces may transiently); user shells can use /tmp like
  any shell.
- **journald** — captures stdout/stderr from the systemd unit.
  Lives under `/var/log/journal/` (systemd-managed).

## Logs (alternative inits)

- **OpenRC**: `/var/log/shellboto.log` (operator-configured).
- **runit**: `/var/log/shellboto/current` (svlogd).
- **s6**: `/var/log/shellboto/` (s6-log).

## What shellboto does NOT touch

- **`/root/.ssh/`** — not unless an admin shell explicitly does.
- **`/etc/passwd`, `/etc/shadow`, `/etc/sudoers`** — not modified
  by the bot. Admin shells can if the human runs commands that
  do.
- **System cron** — no cron jobs installed.
- **Network ports** — no listeners. Outbound HTTPS only to
  `api.telegram.org`.
- **systemd unit reload during runtime** — the bot doesn't touch
  systemd. Operators do via `systemctl restart shellboto`.

## Read next

- [../deployment/installer.md](../deployment/installer.md) —
  installer that creates most of these paths.
- [../security/threat-model.md](../security/threat-model.md) —
  what the perms protect against.
