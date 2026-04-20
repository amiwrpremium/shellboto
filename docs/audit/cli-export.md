# `shellboto audit export`

Stream audit events as JSONL (one JSON object per line) or CSV.
For larger dumps than `audit search`, or for feeding downstream
pipelines.

## Usage

```bash
shellboto audit export [flags] > audit.jsonl
```

Flags:

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `-config <path>` | string | standard | alt config |
| `-user <id>` | int | unset | filter |
| `-kind <kind>` | string | unset | filter |
| `-since <duration>` | duration | unset | filter |
| `-limit <n>` | int | `10000` | max rows |
| `-format json\|csv` | string | `json` | output format |

## JSONL example

```bash
shellboto audit export --since 24h > today.jsonl
head -1 today.jsonl | jq
```

```json
{
  "id": 12345,
  "ts": "2026-04-20T15:04:05.123456789Z",
  "user_id": 987654321,
  "kind": "command_run",
  "cmd": "ls -la /etc",
  "exit_code": 0,
  "bytes_out": 2048,
  "duration_ms": 14,
  "termination": "completed",
  "danger_pattern": null,
  "detail": null,
  "output_sha256": "8d969eef6ecad3c29a3a629280e686cf0c3f5d5a86aff3ca12020c923adc6c92",
  "prev_hash": "a1b2c3...",
  "row_hash": "d4e5f6..."
}
```

The JSONL form includes `prev_hash` + `row_hash` columns (hex-
encoded) so an offline consumer can verify the chain themselves
against a known seed.

## CSV example

```bash
shellboto audit export --format csv --since 24h > today.csv
```

```
id,ts,user_id,kind,cmd,exit_code,bytes_out,duration_ms,termination,danger_pattern,detail,output_sha256
12345,2026-04-20T15:04:05Z,987654321,command_run,"ls -la /etc",0,2048,14,completed,,,8d96...
```

CSV quoting follows RFC 4180: fields with `,` / `"` / `\n` are
quoted; embedded quotes doubled.

CSV **omits** `prev_hash` / `row_hash` (wider rows get unwieldy).
Use JSON format for chain-verification pipelines.

## Does NOT include output blobs

Neither format includes the `audit_outputs.blob`. For that:

```bash
# Single blob via CLI
sudo sqlite3 /var/lib/shellboto/state.db \
    "SELECT blob FROM audit_outputs WHERE audit_event_id = 1234;" | zcat

# Or from Telegram (admin+):
/audit-out 1234
```

Dumping all blobs en masse is a bad idea (privacy + size). If you
need it, write your own SQL query to avoid the shell bot's own
blob-aware commands.

## Pagination

The `--limit` default is 10000. Larger pulls: issue multiple runs
with `--since` windows.

```bash
shellboto audit export --since 24h > today.jsonl
shellboto audit export --since 48h --limit 10000 > day-before.jsonl
# dedupe if needed: awk '!seen[$0]++'
```

Or use SQL for arbitrary windows:

```bash
sudo sqlite3 -separator $'\t' /var/lib/shellboto/state.db \
    "SELECT id, ts, user_id, kind, cmd, exit_code
     FROM audit_events
     WHERE ts >= '2026-04-01' AND ts < '2026-05-01'
     ORDER BY id;" > april-2026.tsv
```

## Scheduled daily export

```
# /etc/cron.daily/shellboto-audit-backup
#!/bin/bash
set -eu
out=/var/backups/shellboto/audit-$(date +%F).jsonl.gz
/usr/local/bin/shellboto audit export --since 24h | gzip > "$out"
chmod 0600 "$out"
# Then rsync / scp / s3 "$out" offsite.
```

Ship the `.jsonl.gz` offsite to the backup of your choice. With the
audit seed also safely stored, you can run verify on a restored
DB anywhere.

## Formats shellboto does not support

- **Protobuf.** Not shipped.
- **Avro / Parquet.** Not shipped.
- **Syslog RFC 5424.** The journald mirror already emits
  structured zap JSON which your log forwarder can translate.

## Reading the code

- `cmd/shellboto/cmd_audit.go:cmdAuditExport`

## Read next

- [cli-replay.md](cli-replay.md) — cross-check an exported JSONL
  against the live DB.
- [../operations/logs.md](../operations/logs.md) — the journald
  mirror, which is effectively a rolling audit export.
