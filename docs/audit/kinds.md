# Audit-event kinds

Every possible value of `audit_events.kind`.

Source: `internal/db/models/audit.go` constants.

| Kind | Fired when | `cmd` | Notes |
|------|-----------|-------|-------|
| `command_run` | A shell command finished | the command text | with exit/duration/termination/bytes_out |
| `danger_requested` | `danger.Match` tripped, waiting for user confirm | pending cmd | `danger_pattern` column has the matched regex |
| `danger_confirmed` | User tapped ✅ Run on the danger prompt | cmd | `danger_pattern` populated |
| `danger_expired` | User tapped ❌ Cancel or `confirm_ttl` elapsed | cmd | combined; UI distinguishes Cancel from timeout |
| `file_download` | `/get <path>` succeeded | the path | `bytes_out` = file size |
| `file_upload` | User uploaded a file that was written | destination path | `bytes_out` = file size; `detail` may include original filename |
| `auth_reject` | Non-whitelisted or disabled user sent something | first ~200 chars | rate-limited by `auth_reject_burst` |
| `shell_spawn` | A new pty was allocated for a user | — | once per GetOrSpawn |
| `shell_reset` | User ran `/reset` (or auto-reset on role change) | — | — |
| `shell_reaped` | Idle-reaper closed a shell | — | `detail` includes last-activity delta |
| `user_added` | `/adduser` flow completed | — | `detail`: new role, name, added_by |
| `user_removed` | `/deluser` flow completed | — | `detail`: target role at time of deletion |
| `user_banned` | Auto-ban (escalation, strict-ASCII, or manual ban button) | — | `detail`: reason |
| `role_changed` | `/role` or UI promote/demote | — | `detail`: "old → new (by <actor>)" |
| `startup` | Bot process started | — | `detail`: git sha, version, config summary |
| `shutdown` | Bot process shutting down cleanly | — | `detail`: graceful=true/false |

## Which kinds include captured output

The `audit_outputs` blob is populated (subject to `audit_output_mode`)
only for:

- `command_run`
- `danger_confirmed` (cmd actually ran)
- `file_download` (captured file bytes? no — just metadata)
- `file_upload` (metadata only)

All other kinds are metadata-only by nature.

## Which kinds fire supernotify

Events that DM the superadmin (and promoter, where applicable):

- `user_added`
- `user_removed`
- `user_banned`
- `role_changed`

Non-fanout kinds stay in the DB + journald only.

## Filtering by kind

```bash
shellboto audit search --kind command_run --limit 20
shellboto audit search --kind auth_reject --since 24h
shellboto audit search --kind role_changed --limit 50
```

## Counting by kind

No first-class `shellboto audit stats`, but SQL does it:

```bash
sudo sqlite3 /var/lib/shellboto/state.db \
    "SELECT kind, COUNT(*) FROM audit_events GROUP BY kind ORDER BY 2 DESC;"
```

Output roughly:

```
command_run|12345
auth_reject|678
danger_requested|42
shell_spawn|34
...
```

## Read next

- [schema.md](schema.md) — the row shape each kind writes.
- [cli-search.md](cli-search.md) — the search CLI.
