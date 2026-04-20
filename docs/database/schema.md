# Database schema

All tables, in DDL form.

## `users`

Whitelist + RBAC.

```sql
CREATE TABLE users (
    telegram_id   INTEGER PRIMARY KEY,
    username      TEXT,                 -- @handle, refreshed each message
    name          TEXT,                 -- admin-entered friendly label
    role          TEXT NOT NULL,        -- superadmin | admin | user
    added_at      DATETIME,
    added_by      INTEGER,              -- FK → users.telegram_id, NULL for seeded superadmin
    disabled_at   DATETIME,             -- NULL = active
    promoted_by   INTEGER                -- FK → users.telegram_id; NULL for user + superadmin
);

CREATE INDEX idx_users_role ON users (role);
CREATE INDEX idx_users_disabled_at ON users (disabled_at);
```

Go model: `internal/db/models/user.go:User`.

`added_by` / `promoted_by` are foreign keys in spirit but not
enforced with `REFERENCES` — we want to keep the row even if the
referenced adder/promoter is later soft-deleted (audit continuity).

## `audit_events`

Append-only event log, hash-chained.

```sql
CREATE TABLE audit_events (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    ts              DATETIME NOT NULL,          -- UTC
    user_id         INTEGER,                    -- FK → users (not enforced)
    kind            TEXT NOT NULL,
    cmd             TEXT,                       -- redacted
    exit_code       INTEGER,
    bytes_out       INTEGER,
    duration_ms     INTEGER,
    termination     TEXT,
    danger_pattern  TEXT,
    detail          TEXT,
    output_sha256   TEXT,                       -- hex
    prev_hash       BLOB NOT NULL,              -- 32 bytes
    row_hash        BLOB NOT NULL               -- 32 bytes
);

CREATE INDEX idx_audit_events_ts      ON audit_events (ts);
CREATE INDEX idx_audit_events_user_id ON audit_events (user_id);
CREATE INDEX idx_audit_events_kind    ON audit_events (kind);
```

See [../audit/schema.md](../audit/schema.md) for detailed
per-column usage.

## `audit_outputs`

One-to-one with `audit_events`, holding gzipped command output
blobs.

```sql
CREATE TABLE audit_outputs (
    audit_event_id  INTEGER PRIMARY KEY,
    blob            BLOB NOT NULL,              -- gzip
    FOREIGN KEY (audit_event_id) REFERENCES audit_events(id) ON DELETE CASCADE
);
```

Rows exist per `audit_output_mode`:

- `always` — every command_run (and similar).
- `errors_only` — only when exit != 0.
- `never` — table stays empty.

## GORM migrations

Schema is managed by GORM's auto-migrator (`db.AutoMigrate(&User{},
&AuditEvent{}, ...)` in `internal/db/migrate.go`). At startup,
GORM adds any missing columns / indexes. It does not drop columns
or data.

Internally GORM uses `sqlite_master` to detect schema drift. No
separate migration table.

## Referential integrity

SQLite foreign keys are off by default; shellboto turns them on
via pragma during db.Open:

```
PRAGMA foreign_keys = ON;
```

So `ON DELETE CASCADE` on `audit_outputs.audit_event_id` works
when a `audit_events` row is pruned.

## Pragma settings

Configured at db.Open:

- `foreign_keys = ON`
- `journal_mode = WAL`
- `synchronous = NORMAL` (WAL-appropriate)
- `cache_size = -2000` (2 MiB)
- `busy_timeout = 5000` (ms)

Not configurable from shellboto config. Change the source if you
have strong reasons.

## Disk size

Empty shellboto DB: ~32 KiB (SQLite overhead + empty tables).

A busy month of operations:

- ~100 audit rows per day × 30 = ~3000 rows.
- Average row metadata ~200 bytes → 600 KiB.
- Output blobs dominate: 10 KiB average × 3000 = 30 MiB.

90-day retention: expect 1–2 GiB in the heaviest usage; < 100 MiB
for typical solo operator.

## Read next

- [migrations.md](migrations.md) — how schema changes land.
- [backup.md](backup.md) — snapshotting this file safely.
