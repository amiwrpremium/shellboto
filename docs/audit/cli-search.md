# `shellboto audit search`

Filtered, paginated, human-readable view of audit events.

## Usage

```bash
shellboto audit search [flags]
```

Flags:

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `-config <path>` | string | `/etc/shellboto/config.toml` | alt config file |
| `-user <id>` | int | unset | filter by `user_id` (Telegram ID) |
| `-kind <kind>` | string | unset | filter by audit kind (see [kinds.md](kinds.md)) |
| `-since <duration>` | duration | unset | only rows newer than `<duration>` ago |
| `-limit <n>` | int | `50` | maximum rows returned |

## Examples

**Last 20 events, any user, any kind:**

```bash
shellboto audit search --limit 20
```

**Last hour's command runs for user 987654321:**

```bash
shellboto audit search --user 987654321 --kind command_run --since 1h
```

**All auth_reject attempts today:**

```bash
shellboto audit search --kind auth_reject --since 24h --limit 200
```

**Last week's danger-confirm events:**

```bash
shellboto audit search --kind danger_confirmed --since 168h
```

## Output format

Tab-separated, ordered by `ts ASC` so the most recent appears at
the bottom (trails off the bottom of your screen like `tail`):

```
ID      TS                          USER         KIND            EXIT  BYTES  CMD
1234    2026-04-20T15:04:05Z        987654321    command_run     0     2048   ls -la /etc
1235    2026-04-20T15:04:07Z        987654321    command_run     1     128    cat /etc/shadow
1236    2026-04-20T15:04:10Z        123456789    role_changed    —     —      alice(987…): user → admin
```

- `—` dashes for columns that aren't applicable to the kind.
- `CMD` truncated to ~60 chars with `…` suffix when longer.
- `BYTES` is `bytes_out` from the row.

## Output length

- Default `--limit 50`.
- Max is a hard `1000` in the CLI to keep the output tractable;
  use `audit export` for larger pulls.

## Exit codes

- `0` — query succeeded (0 rows is not an error).
- `1` — DB access error / invalid flags.

## Filtering by time

`--since` accepts Go duration syntax:

- `30s`, `15m`, `2h`, `24h`, `168h` (1 week), `2160h` (90 days).
- No support for absolute timestamps (`2026-04-20 15:00:00`) —
  use `audit export` + jq / awk for arbitrary windows.

## Inline filtering by CMD

No `--grep` flag today. Pipe through `grep`:

```bash
shellboto audit search --since 24h --limit 500 | grep 'rm '
```

## From Telegram

Admin+:

```
/audit              # last 20, any user
/audit 50           # last 50
```

The Telegram surface doesn't expose all the CLI flags. Fall back
to `shellboto audit search` on the VPS for granular filters.

## Performance

All three filter columns (`ts`, `user_id`, `kind`) have indexes.
Queries hitting any combination of them are fast even on
million-row tables.

If your table gets huge (years of retention, busy bots), SQLite
still handles it; the bottleneck becomes the terminal rendering.

## Reading the code

- `cmd/shellboto/cmd_audit.go:cmdAuditSearch`

## Read next

- [cli-export.md](cli-export.md) — for larger pulls / machine
  parsing.
- [cli-verify.md](cli-verify.md) — integrity walker.
