# Disk full

**Symptom**: `df -h` shows `/var/lib/shellboto/`'s filesystem at
100%. The bot can't write new audit rows; SQLite errors out;
service may be flailing.

## 1. Confirm the source

```bash
df -h
sudo du -sh /var/lib/shellboto/
sudo du -sh /var/lib/shellboto/state.db
```

If shellboto isn't the culprit (it might be `/var/log/journal/`,
some other tenant), you have a host-wide problem — fix that. The
rest of this runbook assumes shellboto's own data is the issue.

## 2. Buy time — vacuum

`shellboto db vacuum` reclaims freelist (no schema or row
changes). Requires service stop:

```bash
sudo systemctl stop shellboto
sudo shellboto db vacuum
sudo systemctl start shellboto
df -h
```

Often recovers 10–40% of DB size if there's been heavy churn.

## 3. Shorter retention

If vacuum doesn't help enough:

```bash
sudo vi /etc/shellboto/config.toml
# audit_retention = "720h"          # 30 days, was 2160h (90)
sudo systemctl restart shellboto
```

The hourly pruner picks this up on next tick. Old rows are
deleted. Then re-vacuum to reclaim space:

```bash
sudo systemctl stop shellboto
sudo shellboto db vacuum
sudo systemctl start shellboto
```

## 4. Aggressive prune

If you need space *now*:

```bash
sudo systemctl stop shellboto
sudo sqlite3 /var/lib/shellboto/state.db \
    "DELETE FROM audit_events WHERE ts < datetime('now', '-7 days');"
sudo sqlite3 /var/lib/shellboto/state.db "VACUUM;"
sudo systemctl start shellboto
sudo shellboto audit verify         # confirm chain still intact
```

Loses 7 days of forensic depth. Get it back via backups (you
have backups offsite, right?).

## 5. Check for an attacker spam

Disk fills suddenly = often an `auth_reject` flood:

```bash
sudo shellboto audit search --kind auth_reject --since 24h | wc -l
```

If that number is huge:

- Check `auth_reject_burst` and `auth_reject_refill_per_sec` —
  are they set tightly enough? Defaults are 5 + 0.05/sec.
- Investigate the source telegram_id(s) of the spam:

  ```bash
  sudo shellboto audit search --kind auth_reject --since 24h \
      | awk '{print $3}' | sort | uniq -c | sort -rn | head
  ```

If a single ID dominates: there's no built-in block-list (the
rate limiter handles it), but you can identify the attacker for
manual followup.

## 6. Check output blob size

```bash
sudo sqlite3 /var/lib/shellboto/state.db <<SQL
SELECT
    COUNT(*) AS rows,
    SUM(LENGTH(blob)) / 1024.0 / 1024 AS mib
FROM audit_outputs;
SQL
```

If output blobs are eating most of the disk, consider:

- Lower `audit_max_blob_bytes` (post-redact storage cap).
- Switch `audit_output_mode` to `errors_only` (only failed
  commands store output) or `never` (no blobs ever).

```bash
sudo vi /etc/shellboto/config.toml
# audit_output_mode = "errors_only"
# audit_max_blob_bytes = 5242880        # 5 MiB
sudo systemctl restart shellboto
```

Effect is **prospective** — old blobs aren't deleted by config
change. Use the prune step to clean up.

## 7. Permanent: log forwarder + bigger disk

Long-term fixes:

- **Bigger disk.** Most VPS providers let you grow the volume.
- **Ship audit logs offsite.** Schedule
  `shellboto audit export --since 24h | gzip > /var/backups/...`
  + offsite copy. Then you can shorten local retention to 7 or
  14 days without losing forensic depth.
- **Compress journald aggressively.** `Storage=persistent` +
  `Compress=yes` in `/etc/systemd/journald.conf` if it isn't
  already.

## 8. Re-tighten alerting

If disk fills surprised you:

- Add a 80% disk-usage alert.
- Add a 95% disk-usage page.
- Add a "audit DB grew >2× last week" alert.

## Read next

- [../audit/retention.md](../audit/retention.md) — pruner
  semantics.
- [../database/vacuum.md](../database/vacuum.md) — when vacuum
  helps.
