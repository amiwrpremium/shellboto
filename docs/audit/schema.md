# Audit schema

SQL DDL for the two audit-related tables.

## `audit_events`

One row per audited event. Append-only (no `UPDATE`, no `DELETE`
except via the retention pruner).

```sql
CREATE TABLE audit_events (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    ts              DATETIME NOT NULL,                -- UTC
    user_id         INTEGER,                          -- telegram_id, NULL for system events
    kind            TEXT NOT NULL,                    -- see kinds.md
    cmd             TEXT,                             -- redacted
    exit_code       INTEGER,
    bytes_out       INTEGER,
    duration_ms     INTEGER,
    termination     TEXT,                             -- completed | canceled | killed | timeout | truncated
    danger_pattern  TEXT,                             -- regex that matched, if any
    detail          TEXT,                             -- free-form JSON or note
    output_sha256   TEXT,                             -- hex, over redacted output
    prev_hash       BLOB NOT NULL,                    -- 32 bytes
    row_hash        BLOB NOT NULL                     -- 32 bytes
);

CREATE INDEX idx_audit_events_ts ON audit_events (ts);
CREATE INDEX idx_audit_events_user_id ON audit_events (user_id);
CREATE INDEX idx_audit_events_kind ON audit_events (kind);
```

Pointer-type Go fields (`user_id`, `exit_code`, `bytes_out`,
`duration_ms`) are nullable in SQL; their canonical-form
serialisation uses `omitempty` so `null` is distinct from `0`.

## `audit_outputs`

Gzipped output blobs, foreign-keyed to `audit_events`. Separate
table so `audit_events` stays small + fast to scan without
dragging blob bytes into every query.

```sql
CREATE TABLE audit_outputs (
    audit_event_id  INTEGER PRIMARY KEY,
    blob            BLOB NOT NULL,                    -- gzipped, redacted
    FOREIGN KEY (audit_event_id) REFERENCES audit_events(id) ON DELETE CASCADE
);
```

One-to-one with `audit_events` (primary key is the FK). Not every
event has a blob — depends on `audit_output_mode`:

| Mode | Row in audit_outputs? |
|------|----------------------|
| `always` | ✅ always |
| `errors_only` | ✅ iff `exit_code != 0` |
| `never` | ❌ never |

If `audit_max_blob_bytes` is exceeded, the blob is dropped and no
row is created; the `audit_events.detail` gets an `output_oversized`
note.

`ON DELETE CASCADE` means pruning `audit_events` rows also drops
their output. No orphaned blobs.

## `users`

Not strictly audit, but referenced here because `audit_events.user_id`
points at `users.telegram_id`:

```sql
CREATE TABLE users (
    telegram_id   INTEGER PRIMARY KEY,
    username      TEXT,
    name          TEXT,
    role          TEXT NOT NULL,
    added_at      DATETIME,
    added_by      INTEGER,
    disabled_at   DATETIME,
    promoted_by   INTEGER
);
```

Full detail in [../database/schema.md](../database/schema.md).

## Indexes

- `idx_audit_events_ts` — for range scans by time (most
  `audit search --since …` queries).
- `idx_audit_events_user_id` — for per-user filtering.
- `idx_audit_events_kind` — for kind filtering.

No composite indexes shipped; SQLite's query planner does well
enough with single-column indexes for shellboto-scale workloads.

## Columns the hash includes

`row_hash = sha256(prev_hash || canonical_json(row))`, where
canonical JSON includes exactly these fields in this order:

1. `ts` (RFC3339Nano)
2. `user_id`
3. `kind`
4. `cmd`
5. `exit_code`
6. `bytes_out`
7. `duration_ms`
8. `termination`
9. `danger_pattern`
10. `detail`
11. `output_sha256`

NOT included:

- `id` — auto-increment, would make hashes depend on insertion
  order in a way that breaks re-verification after pruning.
- `prev_hash`, `row_hash` themselves — they're the output of the
  hash, not input.

## Inspecting directly

```bash
sudo sqlite3 /var/lib/shellboto/state.db
```

Then:

```sql
.schema audit_events
.schema audit_outputs

SELECT id, ts, kind, exit_code FROM audit_events ORDER BY id DESC LIMIT 10;

SELECT length(blob) FROM audit_outputs WHERE audit_event_id = 1234;

-- Pull out a blob (remember: gzipped):
SELECT hex(blob) FROM audit_outputs WHERE audit_event_id = 1234;
```

Preferred: use `shellboto audit …` commands rather than raw SQL.
Raw SQL during hot operation is fine (SQLite's WAL mode handles
readers concurrently with the writer) but easier to mis-interpret
the data.

## Read next

- [kinds.md](kinds.md) — what goes in `kind`.
- [hash-chain.md](hash-chain.md) — how `prev_hash` + `row_hash`
  work operationally.
