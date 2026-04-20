# Config-key reference

Every key the config file accepts. Defaults shown.

## Storage

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `db_path` | string | `/var/lib/shellboto/state.db` | SQLite file location |
| `audit_retention` | duration | `2160h` (90d) | Pruner cutoff |

## Command semantics

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `default_timeout` | duration | `5m` | SIGINT cap per command |
| `kill_grace` | duration | `5s` | SIGINT → SIGKILL gap on timeout |
| `max_output_bytes` | int | `52428800` (50 MiB) | Per-Job buffer cap; exceed → SIGKILL |
| `strict_ascii_commands` | bool | `false` | Reject non-printable-ASCII in commands |
| `queue_while_busy` | bool | `true` | Reserved; reject-with-note today |

## Streaming + Telegram

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `max_message_chars` | int | `4096` | Telegram cap; don't raise |
| `edit_interval` | duration | `1s` | Streamer edit ticker |
| `heartbeat` | duration | `30s` | "still alive" trailer cadence |

## Shell lifecycle

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `idle_reap` | duration | `1h` | Idle-shell teardown threshold |

## Audit output

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `audit_output_mode` | enum | `always` | `always`/`errors_only`/`never` |
| `audit_max_blob_bytes` | int | `52428800` (50 MiB) | Post-redact storage cap |

## Danger matcher

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `extra_danger_patterns` | []string | `[]` | Regexes merged with built-ins |
| `confirm_ttl` | duration | `60s` | Validity of danger-confirm token |

## Logging

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `log_format` | enum | `json` | `json` or `console` |
| `log_level` | enum | `info` | `debug`/`info`/`warn`/`error` |
| `log_rejected` | bool | `false` | Mirror auth_reject events to journald log lines |

## Non-root shells

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `user_shell_user` | string | `""` | Unix account for `role=user` shells |
| `user_shell_home` | string | `""` → `/home/<user>` | Per-user home base dir |

## Rate limiting

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `rate_limit_burst` | int | `10` | Per-user post-auth bucket capacity |
| `rate_limit_refill_per_sec` | float | `1.0` | Per-user post-auth refill rate |
| `auth_reject_burst` | int | `5` | Pre-auth audit-write bucket per attacker-id |
| `auth_reject_refill_per_sec` | float | `0.05` | Pre-auth refill rate (1/20s) |

## Admin fan-out

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `super_notify_action_ttl` | duration | `10m` | Strip inline keyboards from supernotify DMs after this |

## Read next

- [../configuration/schema.md](../configuration/schema.md) —
  full prose treatment with rationale per key.
- [../configuration/formats.md](../configuration/formats.md) —
  TOML / YAML / JSON syntax for the same keys.
