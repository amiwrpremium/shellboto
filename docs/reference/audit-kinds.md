# Audit-event kinds

Every value `audit_events.kind` can take. Source:
`internal/db/models/audit.go`.

| Kind | Fired when | Output blob? | Supernotify? |
|------|------------|:------------:|:------------:|
| `command_run` | A command finished | yes (per mode) | no |
| `danger_requested` | danger.Match tripped | no | no |
| `danger_confirmed` | User tapped ✅ Run | yes | no |
| `danger_expired` | User tapped ❌ Cancel or TTL elapsed | no | no |
| `file_download` | `/get` succeeded | metadata only | no |
| `file_upload` | File written | metadata only | no |
| `auth_reject` | Non-whitelisted/disabled user sent something | no | no |
| `shell_spawn` | New pty allocated | no | no |
| `shell_reset` | `/reset` or auto-reset on role change | no | no |
| `shell_reaped` | Idle-reaper closed a shell | no | no |
| `user_added` | `/adduser` flow completed | no | **yes** |
| `user_removed` | `/deluser` flow completed | no | **yes** |
| `user_banned` | Auto-ban (escalation, strict-ASCII, manual ban) | no | **yes** |
| `role_changed` | `/role` or UI promote/demote | no | **yes** |
| `startup` | Bot process started | no | no |
| `shutdown` | Bot process shutting down cleanly | no | no |

## Termination values (only on `command_run` etc.)

`audit_events.termination`:

| Value | Meaning |
|-------|---------|
| `completed` | Normal exit |
| `canceled` | `/cancel` SIGINT killed it |
| `killed` | `/kill` SIGKILL killed it |
| `timeout` | `default_timeout` SIGINT → SIGKILL escalation |
| `truncated` | `max_output_bytes` cap → auto SIGKILL |

First-write-wins on `termination` so spammed `/cancel`+`/kill`
doesn't confuse the audit record.

## Read next

- [../audit/kinds.md](../audit/kinds.md) — prose treatment with
  per-kind detail.
- [../audit/cli-search.md](../audit/cli-search.md) — filtering by
  kind.
