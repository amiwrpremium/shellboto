# Exit codes

Used by every `shellboto <subcommand>`.

| Code | Meaning |
|------|---------|
| `0` | success |
| `1` | error (DB access, parse, I/O, write failure) |
| `2` | usage (bad flag, missing argument, unknown subcommand) |
| `3` | check failed (`doctor`, `audit verify`, `simulate` matched, `audit replay` mismatches) |

## Per-subcommand

| Subcommand | Possible codes |
|------------|----------------|
| no subcommand (bot) | service exits 0 on clean shutdown, 1 on fatal startup error |
| `doctor` | 0 / 3 |
| `config check` | 0 / 1 |
| `audit verify` | 0 / 3 |
| `audit search` | 0 (always; 0 rows is not an error) |
| `audit export` | 0 / 1 |
| `audit replay` | 0 / 3 |
| `db backup` | 0 / 1 |
| `db info` | 0 |
| `db vacuum` | 0 / 1 |
| `users list` / `users tree` | 0 |
| `simulate` | 0 (no match) / 3 (matched) |
| `mint-seed` | 0 / 1 |
| `service` | passes through `systemctl` / `journalctl` exit |
| `completion` | 0 / 2 (bad shell name) |
| `help`, `-version` | 0 |

## In scripts

Cron / systemd-timer alerting:

```bash
shellboto doctor || mail -s "shellboto unhealthy" you@you.net
shellboto audit verify || page-on-call
```

Both rely on the standard "0 = OK, non-zero = anything else"
convention. `3` distinguishes "I checked and the answer was no"
from `1` "I couldn't even check."

## Why three distinct error families

- `1` = couldn't run the check / something blew up.
- `2` = your invocation was wrong; fix the command line.
- `3` = the check ran and reported a problem.

So your alerting can distinguish "system broken" from "check
indicated bad state." Treat both as bad, but the response is
different (1 = look at logs; 3 = look at what was being checked).

## Read next

- [cli.md](cli.md) — full subcommand reference.
- [../operations/monitoring.md](../operations/monitoring.md) —
  wiring exit codes into your alerts.
