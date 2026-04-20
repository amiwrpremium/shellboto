# Migrations

shellboto uses GORM's auto-migrator. Additive-only. No rollback.

## Policy

- **Add columns? Yes.** GORM adds missing columns with their
  default value on next open.
- **Add indexes? Yes.** Same — created on next open.
- **Add tables? Yes.**
- **Rename a column? No.** Add the new column, write to both for
  a release, migrate data in-code, drop the old one in a later
  release. Or more commonly: don't rename; use a synonym.
- **Remove a column? Only after two shipped versions without it
  being read.** Then a separate migration script, not auto-
  migrator. We haven't done this in production yet.
- **Change a column's type? Avoid.** Requires a manual migration
  script + downtime. Probably a new column is cleaner.

## Why additive-only

- **Zero-downtime upgrades.** New binary can run against old-
  schema DB (missing column is NULL / default; handled in code).
  Old binary can run against new-schema DB (ignores new column).
- **Rollback to previous version never breaks.** `rollback.sh`
  swaps binaries; if the new binary added a column, the old
  binary just doesn't reference it.
- **Simpler than versioned migrations.** No migration version
  table to get out of sync. No dry-run ambiguity.

## How auto-migrator runs

On startup, `internal/db/migrate.go:AutoMigrate`:

```go
err := gormDB.AutoMigrate(
    &models.User{},
    &models.AuditEvent{},
    &models.AuditOutput{},
)
```

GORM introspects the Go struct tags, compares against the live
schema, issues `ALTER TABLE ADD COLUMN …` for missing columns
and `CREATE INDEX` for missing indexes.

Failures at this stage are fatal — the bot logs the error and
exits before opening the bot connection.

## Adding a column — worked example

Say we want a `users.last_seen_at` field.

1. **Model change** — `internal/db/models/user.go`:

   ```go
   type User struct {
       TelegramID int64       `gorm:"primaryKey"`
       // ... existing fields ...
       LastSeenAt *time.Time `gorm:"column:last_seen_at"`
   }
   ```

2. **No SQL migration file.** GORM handles it.

3. **Code that writes it** — in middleware's metadata-touch:

   ```go
   user.LastSeenAt = ptr(time.Now().UTC())
   userRepo.Save(user)
   ```

4. **Code that reads it** — defensive against older rows with
   NULL:

   ```go
   if user.LastSeenAt != nil { ... }
   ```

5. **Ship it.** On upgrade, GORM adds the column with NULL
   defaults; existing rows have NULL until they next message the
   bot.

6. **Release note.** Mention the new column if operators query
   the DB directly.

## Removing a column — the rare case

Don't remove columns unless they've been dead for 2+ releases.
When you do:

1. Stop writing to the column (two releases ago).
2. Stop reading (one release ago — you already wanted a fallback
   since the last release).
3. This release: drop from the Go struct. GORM's auto-migrator
   won't drop the SQL column (it's additive-only), but nothing
   reads it anymore.
4. To reclaim disk space, ship an out-of-band migration:

   ```bash
   sudo systemctl stop shellboto
   sudo sqlite3 /var/lib/shellboto/state.db "ALTER TABLE users DROP COLUMN the_dead_column;"
   sudo systemctl start shellboto
   ```

   SQLite 3.35+ supports `DROP COLUMN` natively. Older SQLite
   requires a table-copy dance.

## Hash chain implications

**Schema changes do not affect the hash chain.** The canonical
form includes a fixed list of fields (see
[../audit/schema.md](../audit/schema.md)). Adding new columns to
`audit_events` that aren't in the canonical form → chain
unaffected.

Adding a new column that *is* in the canonical form → chain
breaks at the schema boundary (old rows' hashes were computed
without the new field).

**Plan accordingly.** If you ever add a field to the canonical
form, treat it as a chain reset: update the seed, note the
discontinuity in operator docs.

## Tools NOT used

- No goose / atlas / flyway. GORM's auto-migrator is sufficient
  and keeps the tool surface small.
- No `migrate up` / `migrate down` commands. Everything is at
  startup.
- No migration linter. Adding a renamed column and forgetting to
  write the compat path is the main way migrations can go wrong —
  code review is the defence.

## Reading the code

- `internal/db/migrate.go` — the single AutoMigrate call.
- `internal/db/models/*.go` — struct tags define the schema.

## Read next

- [schema.md](schema.md) — current schema state.
- [backup.md](backup.md) — before migrating: snapshot.
