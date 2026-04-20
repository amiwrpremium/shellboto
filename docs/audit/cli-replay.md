# `shellboto audit replay`

Cross-check an audit JSONL file (typically from journald) against
the live DB. Used in the DB-corruption runbook to confirm the
journald mirror is authoritative.

## Usage

```bash
shellboto audit replay --file <path> [--verbose]
shellboto audit replay < audit.jsonl    # stdin
```

Flags:

| Flag | Default | Meaning |
|------|---------|---------|
| `-config <path>` | standard | alt config |
| `-file <path>` | unset → stdin | JSONL input (one event per line) |
| `-verbose` | `false` | per-entry ✓/✗ lines |

## What it does

For each JSON line in the input:

1. Parse the audit-event fields.
2. Look up the matching row in `audit_events` by `id`.
3. Compare field-by-field.
4. Emit `OK` / `MISSING_IN_DB` / `HASH_MISMATCH` / `FIELD_MISMATCH`.
5. At the end, print summary counts.

## Exit codes

- `0` — all entries matched.
- `3` — at least one mismatch.

## Getting journald JSON

The bot writes every audit event as a zap Info line on a dedicated
`audit` logger → journald. To extract just those for replay:

```bash
sudo journalctl -u shellboto --output=json-pretty \
    | jq -c 'select(.MESSAGE | fromjson | .msg == "audit") \
        | (.MESSAGE | fromjson)' \
    > /tmp/journal-audit.jsonl
```

Or simpler if your zap setup outputs structured JSON directly:

```bash
sudo journalctl -u shellboto --output=cat \
    | jq -c 'select(.msg == "audit")' \
    > /tmp/journal-audit.jsonl
```

Exact jq recipe depends on your log-format config. The bot emits
(among other fields): `msg="audit"`, `kind`, `user_id`, `cmd`,
`exit_code`, `output_sha256`, `prev_hash`, `row_hash`, etc.

## Running replay

```bash
sudo shellboto audit replay --file /tmp/journal-audit.jsonl --verbose
```

Output:

```
id=1234  ✓ OK
id=1235  ✓ OK
id=1236  ✗ MISSING_IN_DB  (in journal but not in audit_events)
id=1237  ✗ HASH_MISMATCH  stored=abc… journal=def…
...

summary: 5678 checked, 5676 OK, 1 missing, 1 hash_mismatch.
```

Without `--verbose`, only the summary is printed.

## When you'd use it

### 1. After DB corruption / restore

See [../runbooks/db-corruption.md](../runbooks/db-corruption.md).
If you had to restore from a backup, replay the post-backup
journald entries against the restored DB to identify gaps.

### 2. Suspected tampering

`audit verify` says the chain is OK, but you suspect the DB has
been quietly rewritten to be internally consistent. Compare
against the journald mirror — the attacker had to compromise both
to stay hidden.

### 3. Drills

Periodic tabletop: export journald → replay → compare. Confirms
your log forwarder is capturing the right fields + your retention
is aligned.

## What replay does NOT do

- **Doesn't reconstruct missing rows.** Reports MISSING_IN_DB;
  doesn't insert.
- **Doesn't fix HASH_MISMATCH.** Reports; doesn't rewrite.
- **Doesn't verify the journal itself.** Assumes journald is the
  source of truth. Validate via systemd-journald's own
  integrity (`journalctl --verify`).

Rebuilding from the journal is a manual step — see
[../runbooks/db-corruption.md](../runbooks/db-corruption.md) for
the reconstruction SQL.

## Reading the code

- `cmd/shellboto/cmd_audit_replay.go`

## Read next

- [hash-chain.md](hash-chain.md) — the `audit verify` sibling.
- [cli-export.md](cli-export.md) — producing exports suitable for
  replay.
