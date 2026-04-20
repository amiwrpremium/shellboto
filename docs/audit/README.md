# Audit

The operational side of the audit log: schema, kinds, the hash
chain from an operator's perspective, output storage, retention,
and every `shellboto audit …` CLI subcommand.

For the security angle on hash-chain math, see
[../security/audit-chain.md](../security/audit-chain.md) and
[../security/audit-seed.md](../security/audit-seed.md). Those docs
are the "why." These docs are the "how to use it day-to-day."

| File | What it covers |
|------|----------------|
| [schema.md](schema.md) | `audit_events` and `audit_outputs` table DDL |
| [kinds.md](kinds.md) | Every audit-event `kind` constant, when it fires |
| [hash-chain.md](hash-chain.md) | Operational view of chain + verify output reading |
| [output-storage.md](output-storage.md) | Gzipped blob, redaction, SHA-256, modes |
| [retention.md](retention.md) | 90-day pruner, how to change, manual prune |
| [cli-verify.md](cli-verify.md) | `shellboto audit verify` |
| [cli-search.md](cli-search.md) | `shellboto audit search` with filters |
| [cli-export.md](cli-export.md) | `shellboto audit export` JSONL / CSV |
| [cli-replay.md](cli-replay.md) | `shellboto audit replay` (journald cross-check) |

## Quick examples

```bash
# What happened last hour?
shellboto audit search --since 1h --limit 50

# What did alice do this week?
shellboto audit search --user 987654321 --since 168h

# Only danger-confirm events, any user, any time:
shellboto audit search --kind danger_requested

# Dump everything to a JSONL file for offline analysis:
shellboto audit export --format json > audit.jsonl

# Verify the chain is intact:
shellboto audit verify

# Fetch the captured output for event 1234:
shellboto audit search --limit 1 --id 1234   # see the row
# then from Telegram (admin+): /audit-out 1234
```

## Read next

- [schema.md](schema.md) — the tables.
- [kinds.md](kinds.md) — the event catalog.
