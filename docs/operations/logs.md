# Logs

shellboto emits structured JSON log lines through zap, captured
by journald (via the systemd unit's `StandardOutput=journal`).

## Reading live

```bash
sudo journalctl -u shellboto -f
```

Tail the last N:

```bash
sudo journalctl -u shellboto -n 200 --no-pager
```

Filter by log level:

```bash
sudo journalctl -u shellboto --priority=warning -n 100
```

Filter by field (JSON-aware):

```bash
sudo journalctl -u shellboto --output=json | jq 'select(.MESSAGE | fromjson? | .kind == "command_run")'
```

## What goes in the log

Every log line is a JSON object. Fields commonly present:

- `level` — `debug` / `info` / `warn` / `error`.
- `msg` — human-ish event name (`startup`, `command_run`, `audit`,
  `shell closed`, etc).
- `ts` — ISO 8601 UTC nano.
- `logger` — zap logger name. `"audit"` for the audit mirror;
  other loggers (`"telegram"`, `"shell"`) for other subsystems.

## Two separate logger streams

- **Main logger** — everything non-audit. Debug-level events like
  "long-poll round-trip" (at `debug`), info-level events like
  "shell spawned," warns like "rate limited," errors on failures.
- **`audit` child logger** — every audit-event row gets a
  mirrored Info-level log line. Includes `prev_hash` + `row_hash`
  + full canonical fields.

The mirror is what makes cross-checking with the DB possible via
`shellboto audit replay`.

## Log levels

Controlled by `log_level` in config. Default: `info`.

- `debug` — fine-grained; use temporarily to chase a specific
  issue.
- `info` — normal operation.
- `warn` — recoverable anomaly (rate-limit hit, single-row DB
  constraint violation, etc).
- `error` — failure that propagated; something didn't happen that
  should have.
- `fatal` — process is about to exit. Usually only at startup
  (config bad, DB won't open).

Set via config:

```toml
log_level = "debug"
```

Restart to pick up.

## Log format

`log_format = "json"` (default) — JSON. Best for journald.
`log_format = "console"` — coloured text. Best for dev, where you
run `bin/shellboto` in a terminal directly.

## Searching the audit mirror

```bash
# All audit rows from the last hour:
sudo journalctl -u shellboto --since "1 hour ago" --output=cat | \
    jq -c 'select(.msg == "audit")'
```

Same information as `shellboto audit search`, but you can't
filter by `--user` without jq expressions.

## Retention

journald defaults to:

- Keep logs until the journal files exceed 10% of the filesystem
  or 4 GB (whichever smaller).
- Oldest logs rotated out first.

For shellboto-scale traffic, this is typically weeks to months.

Raise it if you need longer journald retention (most scenarios you
want offsite audit exports instead):

```
# /etc/systemd/journald.conf.d/shellboto.conf
[Journal]
SystemMaxUse=4G
MaxRetentionSec=6month
```

`systemctl restart systemd-journald`.

## Shipping logs elsewhere

Common patterns:

- **Vector / Fluent Bit.** Install the shipper; point it at
  journald or a file. Ship to Loki / Elastic / Splunk / CloudWatch.
- **systemd-journal-remote.** Stream journal entries to another
  host.
- **journalbeat / Datadog Agent.** Pre-packaged commercial
  solutions.

shellboto itself doesn't care where the logs end up — any shipper
that captures journald works.

## Log-only workflows

If you don't want the DB at all (weird but OK) and only want
journald audit trails:

- Set `audit_output_mode = never` (no blobs in DB).
- Set `audit_retention = 24h` (DB only keeps 1 day).
- Ship journald offsite.

The DB is still the authoritative chain. journald is the forensic
trail. Losing both = losing forensics.

## Debugging specific issues

Audit chain failures:

```bash
sudo journalctl -u shellboto --since "24 hours ago" --output=cat | \
    jq -c 'select(.msg == "audit" and .kind == "command_run" and .exit_code != 0)'
```

Only errors:

```bash
sudo journalctl -u shellboto --priority=err --since "1 day ago"
```

Rate-limit hits:

```bash
sudo journalctl -u shellboto --output=cat --since "1 day ago" | \
    jq -c 'select(.msg | test("rate"))'
```

## Read next

- [monitoring.md](monitoring.md) — alert on these patterns.
- [../audit/cli-replay.md](../audit/cli-replay.md) — journald
  ↔ DB cross-check.
