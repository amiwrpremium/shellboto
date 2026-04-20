# `audit verify` fails

Quick triage. For full incident response, escalate to
[../runbooks/audit-chain-broken.md](../runbooks/audit-chain-broken.md).

## "❌ chain BROKEN at row 1"

Most common cause: the audit seed in `/etc/shellboto/env`
doesn't match what was used when row 1 was written.

Possibilities:

- You rotated the seed without resetting the chain. See
  [../security/audit-seed.md](../security/audit-seed.md#rotation).
- You restored a DB backup but not the env file (or restored a
  different env's seed).
- The seed env var is empty, so verify is using the all-zeros
  fallback, but the live bot used the real seed.

**Fix:** confirm the seed:

```bash
sudo grep AUDIT_SEED /etc/shellboto/env
```

64-hex-char string, matching what the bot used when row 1 was
written. If you can't find it, you have to:

- Accept the chain break at row 1 (rest of chain still verifies
  internally).
- Or truncate from row 1 + start fresh.

## "❌ chain BROKEN at row N (N > 1)"

A specific row was tampered with, deleted, or corrupted.

Snapshot, then escalate to
[../runbooks/audit-chain-broken.md](../runbooks/audit-chain-broken.md).

## "✅ OK — N rows verified (post-prune)"

This is **not a failure**. Means:

- The oldest surviving row's id > 1 (the original genesis row
  has been pruned by retention).
- Verify uses that row's stored `prev_hash` as the baseline
  instead of the seed.
- Chain integrity from the surviving baseline forward is
  confirmed.

Expected after the chain is older than `audit_retention`.

## "verify returns 1 (not 3)"

`1` means verify couldn't even run — DB access error, file not
found, permission denied. Distinct from `3` which means "ran the
walk and found a problem."

```bash
ls -la /var/lib/shellboto/state.db
sudo systemctl status shellboto       # is it locked?
sudo shellboto doctor                 # baseline checks
```

Resolve the access issue and retry.

## Verify takes a really long time

For huge audit DBs (millions of rows), verify reads everything in
order + computes hashes. 1M rows ≈ 5–15 seconds depending on
disk.

Don't kill it. Wait.

If it's still running after a minute on a 1M-row DB: maybe disk
is contending. Snapshot via `db backup` (which IS faster, single
VACUUM INTO) and verify against the backup elsewhere.

## Verify pass on the live DB but fails on a backup

Either the backup is itself corrupt (run `PRAGMA integrity_check`
on it) or you're verifying with the wrong seed (the env was
different when the backup was made).

## How not to break the chain

- Don't `UPDATE audit_events ...` directly. Don't `DELETE` mid-
  chain.
- Don't rotate the seed without resetting the chain.
- Don't run two shellboto processes against the same DB. The
  flock prevents this; if you bypass it (different DB paths
  pointing at the same file via symlink, etc.), you're on your
  own.

## Read next

- [../security/audit-chain.md](../security/audit-chain.md) — the
  math.
- [../runbooks/audit-chain-broken.md](../runbooks/audit-chain-broken.md)
  — full incident response.
