# Vacuum

SQLite's `VACUUM` rewrites the entire DB file, reclaiming space
freed by `DELETE`. Normally not needed — auto-vacuum keeps it
manageable — but can be useful after a large one-time prune.

## When it helps

- After a one-off mass delete (e.g. manual prune of year-old
  rows).
- After `audit_retention` was dramatically shortened and the
  hourly pruner's first pass freed a lot of space.
- When `shellboto db info` reports a large free-page count.

## When it DOESN'T help

- Routine operation. The hourly pruner's deletes go into
  auto-vacuum incremental mode; space is reclaimed automatically
  over time.
- When you think it'll "make SQLite faster." It won't, for our
  workload.

## `shellboto db vacuum`

```bash
sudo systemctl stop shellboto
sudo shellboto db vacuum
sudo systemctl start shellboto
```

Or, if you try to run it while the service is up, you get:

```
❌ another shellboto is running. Stop the service first:
       sudo systemctl stop shellboto
```

It checks `/var/lib/shellboto/shellboto.lock` with a non-blocking
flock attempt; if it fails, refuses to proceed. This prevents you
from racing the live bot on an exclusive DB connection.

## What it does

1. Acquires the instance lock (via non-blocking flock).
2. Opens the DB.
3. Runs `VACUUM;`.
4. Closes the DB.
5. Releases the lock.

`VACUUM` is an SQLite built-in: creates a temp DB, copies every
row, renames into place. Full-file rewrite. For a 1 GiB DB: about
10–30 seconds depending on disk speed.

## Size reclaim

```bash
ls -la /var/lib/shellboto/state.db
# before: 1.2 GiB

sudo shellboto db vacuum
# after:  620 MiB
```

Your mileage varies. A DB that's been pruning-but-not-truncating
for a year can recover 30–50% of file size. A freshly-pruned DB
might recover 1–2%.

## Risks

- **Downtime.** The service is stopped for the duration.
- **Disk space.** VACUUM needs 2× the DB size in free space
  (original + in-progress copy). Check `df -h` first.
- **Interrupt danger.** If VACUUM crashes partway, SQLite's
  recovery is robust — you'd end up with the original file,
  untouched. But don't kill it.

## Without service stop — the readonly alternative

If you can't afford the service stop, use `VACUUM INTO` to a
different file (that's what `shellboto db backup` does). It
doesn't shrink the live DB but produces a vacuumed copy:

```bash
sudo shellboto db backup /var/lib/shellboto/state-compact.db
sudo systemctl stop shellboto
sudo mv /var/lib/shellboto/state-compact.db /var/lib/shellboto/state.db
sudo systemctl start shellboto
```

Achieves the same outcome with seconds-not-tens-of-seconds of
downtime.

## Routine usage

Quarterly, or when you notice disk pressure. Not part of regular
ops. Skip if things look fine.

## Reading the code

- `cmd/shellboto/cmd_db.go:cmdDBVacuum`

## Read next

- [backup.md](backup.md) — the online-safe cousin.
- [../operations/updating.md](../operations/updating.md) — other
  maintenance operations that require a service stop.
