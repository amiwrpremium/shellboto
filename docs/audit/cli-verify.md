# `shellboto audit verify`

Walks the audit hash chain from the genesis seed (or post-prune
baseline) to the current tip.

## Usage

```bash
shellboto audit verify
shellboto audit verify -config /path/to/config.toml
```

No flags besides `-config`.

## Exit codes

- `0` — chain OK.
- `3` — chain BROKEN or verify failed.

## Output

### OK (fresh deployment)

```
✅ audit chain OK — 123 rows verified.
```

### OK (after retention pruning)

```
✅ audit chain OK — 5678 rows verified (post-prune).
```

### BROKEN

```
❌ audit chain BROKEN at row 9876.
   expected row_hash: 4a5b6c...
   stored row_hash:   9e8d7c...
   reason: prev_hash mismatch with row 9875
```

### Other failures

- DB locked / in use → error with `use 'shellboto db …' commands
  while the service is stopped, or wait for the current operation
  to finish`.
- DB missing → `no such database at /var/lib/shellboto/state.db`.

## What it does

1. Open DB via the same flock-backed path as the bot (conflicts
   if bot is running — will wait or error depending on SQLite's
   WAL mode; in practice WAL lets readers coexist).
2. Load seed from `SHELLBOTO_AUDIT_SEED` env var.
3. Read all rows WHERE `row_hash IS NOT NULL` ORDER BY `id`.
4. If oldest `id > 1`: use that row's stored `prev_hash` as the
   starting accumulator. Set `PostPrune=true`.
5. For each row:
   - Compute `expected_hash = sha256(accumulator ||
     canonical_json(row))`.
   - Compare to the stored `row_hash`.
   - On mismatch: record + continue walking.
6. Print summary.

## Under the hood

Implementation: `internal/db/repo/audit.go:Verify`.

Returns a `VerifyResult{OK bool, VerifiedRows int, FirstBadID
int64, Reason string, PostPrune bool}`. The CLI formats it for
humans.

## Running while the service is up

Safe — SQLite's WAL mode allows readers concurrent with the
writer. You'll see a stable snapshot even if the bot is writing
new audit rows during your verify.

New rows written during verify aren't included in the current
walk; they'll be there for the next run.

## Doing it from Telegram

Admin+ can run:

```
/audit-verify
```

Same implementation, same output (formatted for Telegram).

## Schedule it

See [hash-chain.md](hash-chain.md#when-to-run-verify) for cron +
systemd-timer examples.

## Read next

- [hash-chain.md](hash-chain.md) — the full "running verify"
  treatment.
- [../runbooks/audit-chain-broken.md](../runbooks/audit-chain-broken.md)
  — what to do when it fails.
