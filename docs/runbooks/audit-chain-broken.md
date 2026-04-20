# Audit chain BROKEN

**Symptom**: `shellboto audit verify` reports

```
❌ audit chain BROKEN at row <id> — <reason>
```

That's not normal. Don't restart the bot until you've snapshotted.

## 1. Snapshot

```bash
sudo shellboto db backup /var/backups/shellboto/chain-broken-$(date +%s).db
sudo journalctl -u shellboto --since "30 days ago" > /var/backups/shellboto/chain-broken-$(date +%s).log
```

Both. The DB has the chain; journald has the audit mirror. You
need both for cross-check.

## 2. Look at the bad row

```bash
ID=<from verify output>
sudo sqlite3 /var/lib/shellboto/state.db <<SQL
SELECT id, ts, user_id, kind, cmd, exit_code,
       length(output_sha256), length(prev_hash), length(row_hash)
FROM audit_events
WHERE id BETWEEN $((ID - 2)) AND $((ID + 2))
ORDER BY id;
SQL
```

What's adjacent to the break? Look for:

- A row with NULL or wrong-length hash columns (32 bytes
  expected).
- A timestamp out of order.
- A clearly-edited `cmd` field.

## 3. Cross-check journald

The bot writes a JSON log line per audit event. The journal copy
is independent of the DB:

```bash
sudo journalctl -u shellboto --output=cat --since "<window around ID>" \
    | jq -c 'select(.msg == "audit") | select(.id == '$ID')'
```

Compare every field — `cmd`, `kind`, `prev_hash`, `row_hash` —
against the DB row. Discrepancies tell you what was edited.

## 4. Decide: tampered, corruption, or pruning bug

- **`prev_hash` doesn't match the previous row's `row_hash`** —
  someone deleted a row in between. Or a pruner panic
  half-deleted. Look for a gap in the `id` sequence:

  ```bash
  sudo sqlite3 /var/lib/shellboto/state.db \
      "SELECT id, id - LAG(id) OVER (ORDER BY id) AS delta FROM audit_events ORDER BY id;" \
      | awk -F'|' '$2 > 1'
  ```

  Big deltas = gaps.

- **`row_hash` doesn't match `sha256(prev_hash || canonical(row))`**
  — the row's content was edited after-the-fact. Compare against
  journald — if journald's row matches your stored `row_hash`,
  the DB was tampered with. If they both diverge from the
  expected hash, both DB and journald may have been edited
  (more advanced attacker).

- **`row_hash` matches expected, but `prev_hash` is wrong** — a
  schema migration accidentally rewrote `prev_hash`. Unlikely;
  GORM's auto-migrator doesn't touch existing data.

## 5. Recover

### Option A: Restore from backup

If you have a backup from before the tamper:

1. Stop the service.
2. Restore the backup ([../database/restore.md](../database/restore.md)).
3. Re-replay journald from backup-time onwards into the restored
   DB:

   ```bash
   sudo journalctl -u shellboto --since "<backup ts>" --output=cat \
       | jq -c 'select(.msg == "audit")' > /tmp/replay.jsonl
   sudo shellboto audit replay --file /tmp/replay.jsonl --verbose
   ```

   replay reports MISSING_IN_DB for any rows in journal not in the
   restored DB. You can manually re-insert those, but more often
   you accept the gap (the tampered rows are on the journald side
   too — replay shows you which).

### Option B: Truncate + restart

If the tamper is recent and the lost data isn't critical:

```bash
sudo systemctl stop shellboto
# Delete from the bad row onwards.
sudo sqlite3 /var/lib/shellboto/state.db \
    "DELETE FROM audit_events WHERE id >= $ID;"
sudo systemctl start shellboto
sudo shellboto audit verify
# Should now be OK with reduced row count.
```

You've lost the rows after the break. Document the gap.

### Option C: Accept the broken chain

Sometimes the tamper is informational only — you've identified
who, when, what; you don't need the chain to verify cleanly going
forward. Just leave it. New rows after the bad row chain to it
normally; verify reports the same break every run until the
broken row(s) age out via retention.

## 6. Investigate the cause

- Did the audit seed get rotated without a clean reset? If so,
  the "break" is at row 1 — that's [seed rotation](../security/audit-seed.md),
  not tampering.
- Was there a manual SQL UPDATE? `journalctl -u shellboto`
  shouldn't show one (the bot doesn't UPDATE audit rows). If
  someone ran sqlite3 directly, find them.
- Was there a process crash mid-write? Hash chain writes are
  inside a mutex + transaction; mid-write crash should leave the
  row absent, not malformed. If you see a malformed row, file a
  bug.

## 7. Tighten

- Schedule `shellboto audit verify` more frequently (every hour
  if disk affords).
- Wire it to your alerting so the first break pages you, not just
  logs.
- Consider whether your backup cadence is short enough — 24h
  backups mean up to 24h of audit history may be unrecoverable.

## Read next

- [../security/audit-chain.md](../security/audit-chain.md) — the
  math.
- [../audit/cli-replay.md](../audit/cli-replay.md) — the journald
  cross-check.
