# Restore

Putting a backup back in place. Typically done only during an
incident ([../runbooks/db-corruption.md](../runbooks/db-corruption.md)
is the full runbook).

## Procedure

```bash
# 1. Stop the service.
sudo systemctl stop shellboto

# 2. Move the current DB aside (keep in case restore goes wrong).
sudo mv /var/lib/shellboto/state.db \
        /var/lib/shellboto/state.db.pre-restore.$(date +%s)
# WAL + SHM files too:
sudo mv /var/lib/shellboto/state.db-wal /tmp/ 2>/dev/null || true
sudo mv /var/lib/shellboto/state.db-shm /tmp/ 2>/dev/null || true

# 3. Copy the backup into place.
sudo cp /var/backups/shellboto/state-2026-04-19.db \
        /var/lib/shellboto/state.db
sudo chmod 0600 /var/lib/shellboto/state.db
sudo chown root:root /var/lib/shellboto/state.db

# 4. Start the service.
sudo systemctl start shellboto

# 5. Confirm.
sudo systemctl status shellboto
sudo shellboto doctor
sudo shellboto audit verify
```

## What could go wrong

### The backup's schema is older than the current binary

No problem — GORM's auto-migrator adds missing columns on next
open. Runs during `start`.

### The backup's schema is newer than the current binary

Unusual. Would only happen if you downgraded the binary. The
auto-migrator won't drop columns it doesn't know about; queries
will fail only if the newer schema added required fields. Very
unlikely unless you're juggling pre-release builds.

### `audit verify` reports chain broken at row 1

Means either:

- The audit seed in `/etc/shellboto/env` differs from the one
  that was in use when the backup's first row was written.
- Or the backup's first row was already pruned (post-prune), and
  the seed compared doesn't match the subset's baseline.

If you also restored `/etc/shellboto/env` alongside the DB, this
shouldn't happen. If it does:

```bash
# Confirm the seed matches.
sudo grep AUDIT_SEED /etc/shellboto/env
# Compare against what you had when the backup was made. If different,
# you have the wrong seed — find the right one from an offsite copy.
```

### `audit verify` reports chain broken mid-chain

The backup is **itself** corrupted. Fall back to a prior backup
or rebuild from journald (see
[../runbooks/db-corruption.md](../runbooks/db-corruption.md)).

## Partial restore — users only

Say you just want to recover the whitelist from a backup but
keep the live audit log:

```bash
sudo systemctl stop shellboto

# Dump just the users table from backup.
sudo sqlite3 /var/backups/shellboto/state-old.db \
    "SELECT 'INSERT OR REPLACE INTO users VALUES(' || quote(telegram_id) || ',' || quote(username) || ',' || quote(name) || ',' || quote(role) || ',' || quote(added_at) || ',' || quote(added_by) || ',' || quote(disabled_at) || ',' || quote(promoted_by) || ');'
     FROM users;" > /tmp/users-restore.sql

# Apply.
sudo sqlite3 /var/lib/shellboto/state.db < /tmp/users-restore.sql

sudo systemctl start shellboto
```

Risky — make sure your live DB's schema matches the dump. Prefer
full restore during incidents.

## Partial restore — recent audit rows from journald

Not really "restore" — it's rebuild. See
[../runbooks/db-corruption.md](../runbooks/db-corruption.md).

## Post-restore checklist

After any restore:

- [ ] `systemctl status shellboto` — active (running).
- [ ] `shellboto doctor` — all green.
- [ ] `shellboto audit verify` — OK or OK (post-prune).
- [ ] A test Telegram message → replies normally.
- [ ] `journalctl -u shellboto -n 50` — no errors, "audit chain
      ready" appears.

## Rollback the restore

If the restore went wrong:

```bash
sudo systemctl stop shellboto
sudo mv /var/lib/shellboto/state.db.pre-restore.* \
        /var/lib/shellboto/state.db
sudo systemctl start shellboto
```

Assuming you kept the `.pre-restore.*` file per step 2 above.

## Don't restore over a running bot

Stopping the service is not optional. A `cp` over the live DB
will confuse SQLite (inode changes mid-query); the WAL is stale
for the new inode; you'll see integrity errors at best and
chain breaks at worst.

## Reading the code

- No specific restore code — it's bash + sqlite3 CLI.

## Read next

- [vacuum.md](vacuum.md) — the other "stop the service first"
  operation.
- [../runbooks/db-corruption.md](../runbooks/db-corruption.md) —
  the full recovery playbook.
