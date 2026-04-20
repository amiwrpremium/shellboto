# Hash chain — operational view

For the security / math perspective, see
[../security/audit-chain.md](../security/audit-chain.md). This doc
covers day-to-day operator usage.

## Running verify

```bash
shellboto audit verify
```

Output, happy path:

```
✅ audit chain OK — 12345 rows verified.
```

Output, post-prune (normal):

```
✅ audit chain OK — 5678 rows verified (post-prune).
```

The "(post-prune)" annotation means the oldest surviving row's id
> 1; the walker started from that row's stored `prev_hash` instead
of comparing against the seed.

Output, broken:

```
❌ audit chain BROKEN at row 9876 — expected hash X, got Y.
```

See [../runbooks/audit-chain-broken.md](../runbooks/audit-chain-broken.md)
for response.

## When to run verify

- **On incident.** First thing to do if anything feels off.
- **On schedule.** Cron / systemd timer; alert if non-zero exit.
  Every 6h is reasonable; daily is fine.
- **After a DB restore.** Confirms the restored DB is internally
  consistent.
- **After a reboot.** Especially if it was unexpected.

A cron example:

```
# /etc/cron.d/shellboto-audit-verify
0 */6 * * * root /usr/local/bin/shellboto audit verify || \
    echo "shellboto audit chain BROKEN on $(hostname)" | \
    mail -s "URGENT" you@you.net
```

Or systemd timer:

```
# /etc/systemd/system/shellboto-verify.timer
[Unit]
Description=shellboto audit verify

[Timer]
OnBootSec=15min
OnUnitActiveSec=6h
Persistent=true

[Install]
WantedBy=timers.target
```

```
# /etc/systemd/system/shellboto-verify.service
[Unit]
Description=shellboto audit verify run

[Service]
Type=oneshot
ExecStart=/usr/local/bin/shellboto audit verify
StandardOutput=journal
```

Enable:

```bash
sudo systemctl enable --now shellboto-verify.timer
```

journald captures the runs; you can grep for failures.

## Exit codes

- `0` — chain OK.
- `3` — chain BROKEN or verification failed.

Other CLI subcommands use exit 3 for "sanity check failed" as
well — consistent.

## What verify does NOT check

- **Whether the rows themselves are meaningful.** A chain that's
  been fully re-written by an attacker *who knew the seed* would
  verify clean. The attacker needed root + the seed; you're
  pwned at a layer below. See
  [../security/threat-model.md](../security/threat-model.md).
- **Whether the DB's schema matches.** Schema changes are caught
  at startup by the migrator, not by verify.
- **Whether `audit_outputs` blobs match their `output_sha256`.**
  The hash chain includes `output_sha256` in its canonical form,
  so the chain attests that the field's value hasn't changed —
  but if an attacker edits the blob + recomputes the sha + re-
  verifies the chain forward, the blob-vs-hash match holds. That
  would cost them the full-chain rewrite attack again.

If paranoid about blob integrity specifically:

```bash
sudo sqlite3 /var/lib/shellboto/state.db <<'SQL'
SELECT ae.id,
       ae.output_sha256 AS stored_hash,
       lower(hex(sha256(
           (SELECT uncompress(blob) FROM audit_outputs WHERE audit_event_id = ae.id)
       ))) AS computed_hash
FROM audit_events ae
JOIN audit_outputs ao ON ao.audit_event_id = ae.id
WHERE stored_hash IS NOT NULL
  AND stored_hash != computed_hash;
SQL
```

SQLite doesn't ship `uncompress`, so this is pseudocode. Real
cross-check uses `shellboto audit replay` with a journald mirror
— see [cli-replay.md](cli-replay.md).

## Verify + pruning interaction

The retention pruner (hourly) deletes rows older than
`audit_retention`. Pruning doesn't touch `prev_hash` / `row_hash`
on surviving rows.

After pruning:

- Oldest surviving row's `id` > 1.
- Its `prev_hash` points at the (deleted) previous row's
  `row_hash`.
- Verify treats this as expected; uses the stored `prev_hash` as
  the starting baseline.
- Further rows verify against that baseline normally.

The only way pruning affects verify is the "(post-prune)" note.
It's not a failure.

## What operators should do

- Know `audit verify` exists. Run it after any DB operation.
- Have it scheduled. Without a schedule, a chain break goes
  undetected until the next ad-hoc run.
- Have an alert path. "Verify failed" should page you, not just
  log.
- Read [../runbooks/audit-chain-broken.md](../runbooks/audit-chain-broken.md)
  before you need it.

## Read next

- [cli-verify.md](cli-verify.md) — the CLI subcommand
  reference.
- [cli-replay.md](cli-replay.md) — journald cross-check.
