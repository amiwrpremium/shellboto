# Database

shellboto's persistent state. One SQLite file, two primary tables
plus derived audit tables.

| File | What it covers |
|------|----------------|
| [schema.md](schema.md) | Full DDL for every table |
| [migrations.md](migrations.md) | Additive-only migration policy |
| [instance-lock.md](instance-lock.md) | `flock` on `shellboto.lock` |
| [backup.md](backup.md) | `shellboto db backup` online snapshot |
| [restore.md](restore.md) | Swap a backup in; verify |
| [vacuum.md](vacuum.md) | `shellboto db vacuum` — when and why |

## File layout

```
/var/lib/shellboto/
├── state.db            # SQLite, 0600
├── state.db-wal        # write-ahead log (during ops)
├── state.db-shm        # shared memory (during ops)
└── shellboto.lock      # flock file
```

systemd creates `/var/lib/shellboto/` with `StateDirectory=shellboto`
at 0700. The bot chmods `state.db` to 0600 on first open.

## Tables

- **`users`** — whitelist, roles, promotion lineage.
- **`audit_events`** — append-only event log with hash chain.
- **`audit_outputs`** — gzipped command output blobs, FK →
  audit_events.

Plus GORM's internal bookkeeping tables when migrations land.

## WAL mode

SQLite runs in WAL mode, so:

- Readers don't block writers.
- Writers don't block readers.
- Crash recovery is automatic on next open.

Consequence: `shellboto audit verify` works while the bot is
running. So does `shellboto db info`. Don't do `db vacuum` while
the service is running (see [vacuum.md](vacuum.md)).

## Read next

- [schema.md](schema.md) — tables in full.
- [backup.md](backup.md) — snapshot before doing anything risky.
