# Backup

Snapshotting `/var/lib/shellboto/state.db` safely, online.

## `shellboto db backup`

Built-in. Uses SQLite's `VACUUM INTO`, which is online-safe (does
not require the service to be stopped):

```bash
sudo shellboto db backup /var/backups/shellboto-$(date -I).db
```

Output:

```
✅ backup written to /var/backups/shellboto-2026-04-20.db (3.2 MiB)
```

- Works while the bot is running.
- Produces a **consistent** snapshot — it's not just `cp`.
- The snapshot file is a complete SQLite DB.
- Wrapper around `VACUUM INTO 'dst'` which SQLite implements
  atomically at the DB level.

## Why not `cp /var/lib/shellboto/state.db backup.db`

Because:

- WAL mode means live writes are in `state.db-wal`, not
  `state.db` yet. A `cp` of just the main file loses recent
  writes.
- Even copying all three (`state.db`, `state.db-wal`,
  `state.db-shm`) while writes are in progress is racy. You'd
  need to checkpoint + lock, which is exactly what `VACUUM INTO`
  does.

## Perms

After `db backup`:

```
-rw------- root:root /var/backups/shellboto-2026-04-20.db
```

Inherits 0600 from the parent dir + umask. Your backup script
should continue that — readable only by root.

## What's in the backup

Everything:

- `users`
- `audit_events`
- `audit_outputs`
- All indexes
- SQLite pragmas

Restore the file = restore the full DB.

## What's NOT in the backup

- **The audit seed.** That's in `/etc/shellboto/env`. Without
  the seed, verify of the backup will report seed-mismatch at
  row 1 even though the chain itself is intact.
- **The bot token + superadmin ID.** Same file.
- **The config.** `/etc/shellboto/config.toml`.

A "complete" backup is DB + env + config. Typical script:

```bash
#!/bin/bash
set -eu
outdir=/var/backups/shellboto-$(date -I)
sudo mkdir -p "$outdir"
sudo chmod 0700 "$outdir"
sudo shellboto db backup "$outdir/state.db"
sudo cp -a /etc/shellboto/env "$outdir/env"
sudo cp -a /etc/shellboto/config.toml "$outdir/config.toml"
sudo chmod -R 0600 "$outdir"
echo "Backup written to $outdir"
```

Then rotate / ship offsite per your backup policy.

## Offsite shipping

Don't just leave backups next to the primary DB — a single disk
failure takes both out. Options:

- **`rsync` to another host.** Simple.
- **`restic` / `borg`** — deduplicated, encrypted, client-side.
  Good for retaining a long window without ballooning disk.
- **Object storage** (S3, B2, GCS). `aws s3 cp` /
  `b2 upload-file` after backup.

Encrypt at rest. The DB contains audit output (potentially
secrets, even after redaction).

## Restore

See [restore.md](restore.md).

## Scheduled backups

```
# /etc/cron.daily/shellboto-backup
#!/bin/bash
set -eu
outdir=/var/backups/shellboto
date=$(date -I)
mkdir -p "$outdir"
/usr/local/bin/shellboto db backup "$outdir/state-$date.db"
find "$outdir" -name 'state-*.db' -mtime +30 -delete
```

Keep 30 days of daily backups locally + weekly / monthly offsite.

## Verify backups are readable

Don't discover your backup is corrupt during an incident. Monthly:

```bash
BKP=/var/backups/shellboto/state-$(date -I).db
sudo sqlite3 "$BKP" "PRAGMA integrity_check;"   # should say "ok"
sudo SHELLBOTO_TOKEN=x SHELLBOTO_SUPERADMIN_ID=1 \
     shellboto -config /dev/null audit verify   # (see note below)
```

Note: the verify-against-backup trick needs either running
against a test config that points at the backup file, or
temporarily copying `state.db` aside. See
[../runbooks/db-corruption.md](../runbooks/db-corruption.md) for
the full restore dance.

## Reading the code

- `cmd/shellboto/cmd_db.go:cmdDBBackup`

## Read next

- [restore.md](restore.md) — after a backup, how to put it back.
- [../runbooks/db-corruption.md](../runbooks/db-corruption.md) —
  when it's no longer a drill.
