# Database corruption

**Symptom**: SQLite reports integrity errors, the bot fails to
start with `database disk image is malformed`, or `audit verify`
fails in unexpected ways unrelated to chain logic.

## 1. Stop the service

```bash
sudo systemctl stop shellboto
```

The bot keeps the DB open with WAL active. Stopping reduces the
risk of further damage during diagnosis.

## 2. Snapshot what's left

Even a corrupt DB might be partially recoverable:

```bash
sudo cp -a /var/lib/shellboto/state.db   /var/backups/shellboto/corrupted-state-$(date +%s).db
sudo cp -a /var/lib/shellboto/state.db-wal /var/backups/shellboto/corrupted-state-$(date +%s).wal 2>/dev/null || true
sudo cp -a /var/lib/shellboto/state.db-shm /var/backups/shellboto/corrupted-state-$(date +%s).shm 2>/dev/null || true
sudo journalctl -u shellboto --since "30 days ago" > /var/backups/shellboto/corrupted-journal-$(date +%s).log
```

## 3. Confirm corruption

```bash
sudo sqlite3 /var/lib/shellboto/state.db "PRAGMA integrity_check;"
```

- `ok` → not actually corrupted; look elsewhere.
- Anything else → confirmed.

## 4. Restore from backup

If you have a recent `shellboto db backup` output:

See the full restore procedure in [../database/restore.md](../database/restore.md).
Short form:

```bash
sudo mv /var/lib/shellboto/state.db /var/lib/shellboto/state.db.corrupted
sudo cp /var/backups/shellboto/state-2026-04-19.db /var/lib/shellboto/state.db
sudo chmod 0600 /var/lib/shellboto/state.db
sudo chown root:root /var/lib/shellboto/state.db
sudo rm -f /var/lib/shellboto/state.db-wal /var/lib/shellboto/state.db-shm

sudo systemctl start shellboto
sudo shellboto doctor
sudo shellboto audit verify
```

## 5. Replay journald audit rows post-backup

The backup is from yesterday. Today's rows are gone from the
restored DB. journald has them.

```bash
# Extract journald audit rows newer than the backup's last ts.
LAST_TS=$(sudo sqlite3 /var/lib/shellboto/state.db "SELECT MAX(ts) FROM audit_events;")
sudo journalctl -u shellboto --since "$LAST_TS" --output=cat \
    | jq -c 'select(.msg == "audit")' > /tmp/post-backup.jsonl

# Replay against the restored DB.
sudo shellboto audit replay --file /tmp/post-backup.jsonl --verbose
```

`replay` reports MISSING_IN_DB for every row in the journal that
isn't in the restored DB — exactly the rows you lost.

To re-insert them: there's no `audit insert` CLI. Either:

- Accept the gap. The chain on the restored DB is still
  internally consistent.
- Manually craft INSERTs from the journal JSONL. Risky; chain
  hashes need to be computed in order. Ask in an issue if you
  need to do this — it might be worth a `shellboto audit
  reconstruct-from-journal` subcommand.

## 6. If no backup exists

Worst case: no backup, DB unsalvageable.

You'll lose audit history. Bot continues to function (whitelist
re-seeded from env, fresh audit chain starts):

```bash
sudo rm /var/lib/shellboto/state.db /var/lib/shellboto/state.db-wal /var/lib/shellboto/state.db-shm
sudo systemctl start shellboto
sudo shellboto doctor
```

The bot creates a fresh DB on start. Superadmin re-seeds from
env. Whitelist is empty otherwise — re-add users via `/adduser`.

This is exactly why backups matter. If you've reached this
runbook step, install daily backup cron right now:

```
# /etc/cron.daily/shellboto-backup
#!/bin/bash
set -eu
mkdir -p /var/backups/shellboto
/usr/local/bin/shellboto db backup /var/backups/shellboto/state-$(date -I).db
find /var/backups/shellboto -name 'state-*.db' -mtime +30 -delete
```

## 7. Investigate the cause

Most common causes of SQLite corruption:

- **Disk failure**. Check `dmesg`, `smartctl`, `df`.
- **Running shellboto on a flaky filesystem** (network FS, FUSE
  layer). Don't.
- **Two processes writing to the same DB without going through
  the instance flock**. Should be impossible (we enforce
  `flock`); if you see this, file a bug.
- **Power loss + WAL not properly flushed**. SQLite normally
  recovers; if not, hardware/FS may be lying about fsync.

Once you know the cause, mitigate (replace disk, move off the
fragile FS, etc.).

## 8. Re-tighten

- Daily backup cron, tested by monthly restore drill.
- Disk-pressure monitoring (alert at 80%, page at 95%).
- A `shellboto doctor` cron that runs every 15 min.

## Read next

- [../database/backup.md](../database/backup.md) — get a backup
  setup *before* you need it.
- [audit-chain-broken.md](audit-chain-broken.md) — adjacent
  failure mode.
