# Common error messages

Error string → likely cause → fix.

## Startup

| Error | Cause | Fix |
|-------|-------|-----|
| `SHELLBOTO_TOKEN required` | env var missing or empty | Set in `/etc/shellboto/env`, restart |
| `SHELLBOTO_SUPERADMIN_ID must be a positive integer` | env var missing/invalid | Set to a positive int64; restart |
| `audit seed not set — using zero-seed fallback` | warn, not error | Mint via `shellboto mint-seed`; set; restart |
| `instance lock: resource temporarily unavailable` | another shellboto already running | Find via `lsof /var/lib/shellboto/shellboto.lock` |
| `database disk image is malformed` | DB corruption | [../runbooks/db-corruption.md](../runbooks/db-corruption.md) |
| `bad config: ...` | malformed config file | `shellboto config check` for details |
| `EnvironmentFile: No such file` (systemd) | env file missing | Re-run installer; check `/etc/shellboto/env` exists |

## Telegram API

| Error | Cause | Fix |
|-------|-------|-----|
| `Unauthorized` / `401` | bot token wrong / revoked | Re-check token; rotate if leaked |
| `Bad Request: chat not found` | sending to a chat the bot can't reach (user blocked it; channel they're not in) | check the chat |
| `Too Many Requests: retry after N` | bot exceeded Bot API rate limit | Likely transient; raise edit_interval if persistent |
| `Forbidden: bot was blocked by the user` | user blocked the bot | Tell them to unblock; or remove from whitelist |
| `Network unreachable` | outbound HTTPS failing | Check firewall + DNS |

## Shell

| Error | Cause | Fix |
|-------|-------|-----|
| `ErrBusy: shell busy` (in handler logs) | user sent a 2nd command while one is running | Expected; user sees "shell busy" reply. /cancel or /kill the first |
| `shell dead` | bash crashed in the user's pty | Next message spawns fresh shell. Investigate via `journalctl -u shellboto` |
| `pty: input/output error` | pty closed unexpectedly | Same as above |

## Audit

| Error | Cause | Fix |
|-------|-------|-----|
| `audit chain BROKEN at row N` | tampering or corruption | [audit-verify-fails.md](audit-verify-fails.md) |
| `output_oversized` (in audit detail field) | output exceeded `audit_max_blob_bytes` | Tune cap; metadata still in row |

## DB

| Error | Cause | Fix |
|-------|-------|-----|
| `attempt to write a readonly database` | DB file perms wrong, or systemd's filesystem hardening blocking writes | Check `/var/lib/shellboto/state.db` mode 0600 root:root; check unit's `ProtectSystem` |
| `database is locked` | another writer holds the lock; rare with WAL | `sudo systemctl stop shellboto`, retry, restart |
| `disk I/O error` | underlying disk failure | Check `dmesg`, `smartctl`. Snapshot + restore |

## Config

| Error | Cause | Fix |
|-------|-------|-----|
| `bad duration "5 minutes"` | not Go duration syntax | Use `5m` |
| `bad enum: ...` | bad value for `log_format` / `audit_output_mode` etc | See [../reference/config-keys.md](../reference/config-keys.md) |
| `pattern X failed to compile: ...` | bad regex in `extra_danger_patterns` | Fix regex syntax; test with `shellboto simulate` after |

## Installer

| Error | Cause | Fix |
|-------|-------|-----|
| `go 1.26+ not found` | wrong Go version | Install 1.26+; or `--skip-build` with pre-built binary |
| `permission denied` | not root | `sudo` |
| `make: command not found` | install make | `apt install make` etc. |
| `no such user 'shellboto-user'` (when configuring user_shell_user) | the unprivileged account doesn't exist | `useradd --system --shell /bin/bash shellboto-user` |

## CLI

| Error | Cause | Fix |
|-------|-------|-----|
| `another shellboto is running` (during `db vacuum`) | service must be stopped for vacuum | `sudo systemctl stop shellboto` |
| `unknown subcommand` | typo | `shellboto help` for the list |
| `bad flag` | typo | check the subcommand's flags |

## Hooks (lefthook)

| Error | Cause | Fix |
|-------|-------|-----|
| `golangci-lint: 0 issues` then exits non-zero | nothing | golangci-lint v2 sometimes returns non-zero on no-op runs; bump version |
| `gitleaks found 1 leak` | real or fake secret in staged files | Real → revoke + remove from history. Fake (test fixture) → add to `.gitleaks.toml` allowlist |
| `shellcheck SC1091: not following` | source of file not provided to shellcheck | Add `# shellcheck disable=SC1091 source=path/to/file` directive |
| `commit-msg failed` | message doesn't match Conventional Commits | Format: `<type>(<scope>)!: <description>` |

## Read next

- [../runbooks/](../runbooks/) — when an error indicates an
  incident.
- [../operations/logs.md](../operations/logs.md) — finding errors
  in journald.
