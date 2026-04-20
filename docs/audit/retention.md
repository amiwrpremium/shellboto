# Retention + pruning

Audit rows are append-only under normal operation, but the pruner
goroutine deletes rows older than `audit_retention` on an hourly
ticker.

## How it works

On startup, `AuditRepo.PruneLoop(ctx)` is spawned:

```go
go func() {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            func() {
                defer func() {
                    if r := recover(); r != nil {
                        logger.Error("prune panic", zap.Any("panic", r))
                    }
                }()
                cutoff := time.Now().Add(-cfg.AuditRetention)
                a.PruneNow(ctx, cutoff)
            }()
        }
    }
}()
```

`PruneNow` executes:

```sql
DELETE FROM audit_events WHERE ts < ?;
```

`audit_outputs` rows cascade via `ON DELETE CASCADE`.

Wrapped in `recover()` so a driver-level panic (disk full,
constraint violation, whatever) doesn't crash the entire process.

## Hash chain after pruning

The pruner does NOT rewrite `prev_hash` on the oldest surviving
row. So after pruning, the chain looks like:

```
row 500   prev_hash = <hash of row 499, which no longer exists>
row 501   prev_hash = <row 500's row_hash>
row 502   prev_hash = <row 501's row_hash>
...
```

Verify handles this: when it sees the oldest row's id > 1, it
treats the stored `prev_hash` as the baseline and continues
forward. Annotates output as "(post-prune)".

## Tuning

```toml
audit_retention = "2160h"  # default = 90 days
```

Set shorter if:

- Disk is tight.
- You have compliance-driven data-minimisation requirements.
- Most of your audit rows are `auth_reject` noise that ages out
  quickly.

Set longer if:

- Your compliance regime requires 1-year or multi-year retention.
- You want to investigate long-past incidents without an offsite
  backup.

Change the config, restart the service. On next hourly tick, the
pruner applies the new cutoff.

## Going longer than disk allows

If you want 1-year retention but your VPS disk can't hold it:

1. Keep `audit_retention` at a local-affordable window (e.g. 90d).
2. Set up a scheduled export + offsite copy:

   ```bash
   # /etc/cron.daily/shellboto-audit-export
   shellboto audit export --format json --since 24h | \
       gzip > "/var/backups/shellboto/audit-$(date +%F).jsonl.gz"
   # then scp / s3 / whatever
   ```

3. Long-term audit lives in JSONL files offsite; the DB only
   holds the recent window.

## Manually pruning

No `shellboto audit prune` subcommand exists today. If you want
to force-prune (e.g. disk ran out, need to free space fast):

```bash
sudo sqlite3 /var/lib/shellboto/state.db \
    "DELETE FROM audit_events WHERE ts < datetime('now', '-30 days');"
sudo sqlite3 /var/lib/shellboto/state.db "VACUUM;"
```

Then run `shellboto audit verify` to confirm the chain still
verifies against the new oldest-surviving row.

## What pruning doesn't touch

- `users` table. Users stay in the DB forever (soft-deleted rows
  are still rows). Rationale: audit rows reference `user_id`;
  preserving the user row keeps `/auditme` et al. interpretable.
- Dynamic-TTL features (inline-keyboard sweep in supernotify) —
  those have their own timers.

## Reading the code

- `internal/db/repo/audit.go:PruneLoop`
- `internal/db/repo/audit.go:PruneNow`

## Read next

- [schema.md](schema.md) — what pruning deletes (full row + blob).
- [../database/backup.md](../database/backup.md) — before you lose
  data, snapshot it.
