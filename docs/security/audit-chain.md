# Audit hash chain

Every audit row carries `prev_hash` and `row_hash` columns. The
values link rows into a chain — editing any row in the middle
breaks every downstream hash, which the verify walker detects.

## The math

For each row:

```
row_hash = sha256( prev_hash || canonical_json(row) )
```

where:

- `prev_hash` is the previous row's `row_hash` (or the genesis
  seed for the first row).
- `canonical_json(row)` is the row's fields marshaled to JSON in a
  fixed field order, using a fixed representation.

## Canonical form

From `internal/db/repo/audit.go`, the `canonicalRow` struct has
exactly these fields, in this order:

```go
TS              // string, UTC, time.RFC3339Nano
UserID          // *int64, omitempty
Kind            // string
Cmd             // string, already redacted
ExitCode        // *int, omitempty
BytesOut        // *int, omitempty
DurationMS      // *int64, omitempty
Termination     // string, omitempty
DangerPattern   // string, omitempty
Detail          // string, omitempty
OutputSHA256    // string, hex, omitempty
```

Marshaled with Go's `encoding/json`. Pointer fields that are nil
serialise as `null` and are omitted; empty strings with `omitempty`
are also omitted.

Because TS is always UTC and RFC3339Nano, and JSON field order is
fixed, two rows with identical content always produce identical
canonical bytes — and therefore identical hashes.

## The genesis seed

The first row's `prev_hash` is the **audit seed**, loaded from the
`SHELLBOTO_AUDIT_SEED` env var as 32 bytes of hex.

```go
seed := parseHex(os.Getenv("SHELLBOTO_AUDIT_SEED"))    // 32 bytes
// or, if empty: seed = make([]byte, 32)  // all zeros, with a warning
```

Without the seed set, `prev_hash` of the first row is
`0000…0000` (32 zero bytes), which any attacker can compute.

With the seed set, an attacker who tampers with the DB cannot
silently recompute the first row's hash without knowing the seed
(which is in `/etc/shellboto/env`, not in the DB).

## Chain threading

```
genesis seed
      │
      ▼
  row 1 { prev_hash = seed }
      ▼
      computes row_hash = sha256(seed || canonical(row1))
      │
      ▼
  row 2 { prev_hash = row1.row_hash }
      ▼
      computes row_hash = sha256(row1.row_hash || canonical(row2))
      │
      ▼
  row 3 …
```

`audit.Log` takes a package-level mutex before performing the
read-previous / compute-hash / insert sequence, so concurrent
writes don't fork the chain.

## The verify walker

`shellboto audit verify` (implementation in
`internal/db/repo/audit.go`'s `Verify`):

```go
func (a *AuditRepo) Verify(ctx context.Context) (VerifyResult, error)
```

Behaviour:

1. Read all rows WHERE `row_hash IS NOT NULL` ORDER BY id.
2. If the oldest surviving row's id > 1, the chain has been pruned
   (expected, due to retention). Use the stored `prev_hash` of
   that row as the starting baseline; skip the seed comparison.
   Set `PostPrune=true` in the result.
3. For each row:
   - Compute `expected = sha256(prev_hash_accumulator ||
     canonical(row))`.
   - Compare `expected` to the row's stored `row_hash`.
   - On mismatch: record the row id + reason + return.
4. If all rows match: return `OK=true, VerifiedRows=N`.

The walker **does not short-circuit on first break**; it records the
first mismatch id and continues walking to the end, counting rows.
This lets `audit verify` report a diagnostic like "row 1234 is the
first bad row; 5000 total rows scanned."

## What a tamper looks like

### Scenario 1 — edit a `cmd` column

```sql
UPDATE audit_events
SET cmd = 'innocent thing'
WHERE id = 1234;
```

That row's `row_hash` doesn't match `sha256(prev_hash ||
canonical(new_row))` anymore — because the stored `row_hash` is
still the hash of the original row. Verify catches it at row 1234.

Trying to also rewrite `row_hash`:

```sql
UPDATE audit_events SET cmd='…', row_hash=<new> WHERE id=1234;
```

Now row 1234 passes verify individually. But row 1235's `prev_hash`
column still points at the *old* hash of row 1234. So verify
catches the break at row 1235, calling row 1234 the "first bad
row" still (because the accumulator no longer aligns).

To fully tamper the chain without detection, the attacker must
rewrite **every row from the tampered row to the end**. Tractable
only if they know the seed + can run arbitrary SQL.

### Scenario 2 — delete a row mid-chain

```sql
DELETE FROM audit_events WHERE id = 1234;
```

Row 1233's `row_hash` is now what row 1235's `prev_hash` points
at, but they don't match. Verify catches it.

### Scenario 3 — full truncate

```sql
DELETE FROM audit_events;
```

`audit verify` returns "0 rows verified" (vacuously true). The
journald mirror (see below) still has the evidence — shellboto
writes every audit event as a structured log line too. Cross-
checking:

```bash
shellboto audit replay --file /var/log/journal-audit.jsonl
```

compares journald and DB. Mismatches → listed.

## The journald mirror

Every audit write also emits a `zap.Info` line with the row's
canonical fields through the dedicated `audit` logger:

```json
{
  "level":"info",
  "msg":"audit",
  "ts":"2026-04-20T18:03:11.123456789Z",
  "id":1234,
  "user_id":987654321,
  "kind":"command_run",
  "cmd":"ls -la",
  "exit_code":0,
  "bytes_out":412,
  "duration_ms":23,
  "termination":"completed",
  "output_sha256":"…",
  "prev_hash":"…",
  "row_hash":"…"
}
```

journald captures it. If an attacker wipes the DB but not the
journal, you can rebuild with `shellboto audit replay`. Conversely
if they wipe the journal but not the DB, `audit verify` still
catches row-level tampering.

**The attack vector "compromise both DB and journald"** is
expensive — it requires root on the VPS plus patience to edit
journal binary files. At that point your VPS is owned and the
audit log is the least of your problems.

## Rotating the seed

Don't rotate casually. See [audit-seed.md](audit-seed.md).

## Running verify regularly

As a cron or systemd timer:

```bash
sudo shellboto audit verify
```

If anything except `✅ audit chain OK — N rows verified` comes out,
alert. See [../runbooks/audit-chain-broken.md](../runbooks/audit-chain-broken.md)
for response.

From Telegram (admin+):

```
/audit-verify
```

## Read next

- [audit-seed.md](audit-seed.md) — how to mint, store, rotate.
- [../audit/hash-chain.md](../audit/hash-chain.md) — the same
  topic but covered from the operational rather than security
  angle.
