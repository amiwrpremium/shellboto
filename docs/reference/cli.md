# CLI subcommand reference

Running `shellboto` with no subcommand starts the bot service.
With a subcommand, it runs a one-shot ops utility and exits.

All subcommands accept `-config <path>` (default
`/etc/shellboto/config.toml`). Env vars
(`SHELLBOTO_TOKEN`, `SHELLBOTO_SUPERADMIN_ID`,
`SHELLBOTO_AUDIT_SEED`) must be available the same way as for the
service.

## `doctor`

```
shellboto doctor
```

Preflight check: config, env, paths, permissions, DB access.

| Flag | Default | Purpose |
|------|---------|---------|
| `-config <path>` | `/etc/shellboto/config.toml` | alt config |

Exit: 0 all pass / 3 fail.

See [../operations/doctor.md](../operations/doctor.md).

## `config check [path]`

```
shellboto config check
shellboto config check /tmp/alt.toml
```

Parse + validate a config file; print the effective values.

Exit: 0 OK / 1 parse or validation error.

## `audit verify`

```
shellboto audit verify
```

Walk the SHA-256 hash chain from the seed / post-prune baseline
to the tip.

Exit: 0 OK / 3 BROKEN.

See [../audit/cli-verify.md](../audit/cli-verify.md).

## `audit search`

```
shellboto audit search [flags]
```

| Flag | Default | Purpose |
|------|---------|---------|
| `-user <id>` | unset | filter by Telegram user ID |
| `-kind <kind>` | unset | filter by audit kind |
| `-since <duration>` | unset | `1h` / `24h` / etc. |
| `-limit <n>` | `50` | max rows (cap 1000) |

Exit: 0 always (0 rows not an error).

See [../audit/cli-search.md](../audit/cli-search.md).

## `audit export`

```
shellboto audit export [flags]
```

| Flag | Default | Purpose |
|------|---------|---------|
| `-user <id>` | unset | filter |
| `-kind <kind>` | unset | filter |
| `-since <duration>` | unset | filter |
| `-limit <n>` | `10000` | max rows |
| `-format json\|csv` | `json` | output format |

Writes to stdout. Exit: 0 OK / 1 error.

See [../audit/cli-export.md](../audit/cli-export.md).

## `audit replay`

```
shellboto audit replay --file <path>
```

Or stdin:

```
cat journal-audit.jsonl | shellboto audit replay
```

| Flag | Default | Purpose |
|------|---------|---------|
| `-file <path>` | stdin | JSONL input |
| `-verbose` | `false` | per-entry status lines |

Exit: 0 all match / 3 mismatches.

See [../audit/cli-replay.md](../audit/cli-replay.md).

## `db backup <path>`

```
shellboto db backup /var/backups/shellboto/state-$(date -I).db
```

Online-safe SQLite snapshot via `VACUUM INTO`.

Exit: 0 OK / 1 error.

## `db info`

```
shellboto db info
```

Tab-aligned output: file size, mod time, table row counts,
oldest/newest audit ts, pragma settings.

Exit: 0.

## `db vacuum`

```
shellboto db vacuum
```

Reclaim freelist. Requires the service stopped (takes the
instance flock).

Exit: 0 OK / 1 error (including "service is running").

See [../database/vacuum.md](../database/vacuum.md).

## `users list`

```
shellboto users list
```

Tab-aligned table of every user (active + disabled).

Exit: 0.

## `users tree`

```
shellboto users tree
```

ASCII tree of promotion lineage (👑 superadmin → 🛡 admins →
users).

Exit: 0.

## `simulate <cmd>`

```
shellboto simulate 'rm -rf /tmp/build'
```

Dry-run the danger matcher. Reports the matched pattern (if any).

Exit: 0 no match / 3 match.

## `mint-seed`

```
shellboto mint-seed
shellboto mint-seed --env
```

Prints a fresh 32-byte hex seed. `--env` emits in env-file form
(`SHELLBOTO_AUDIT_SEED=<hex>`).

Exit: 0 OK.

## `service <verb>`

```
shellboto service status|start|stop|restart|enable|disable|logs
```

Passthrough to `systemctl` / `journalctl`. `logs` adds `-n 200 -f`
defaults.

Exit: the systemctl / journalctl return value.

## `completion <bash|zsh|fish>`

```
shellboto completion bash | sudo tee /etc/bash_completion.d/shellboto > /dev/null
source /etc/bash_completion.d/shellboto
```

Prints a shell completion function.

Exit: 0 / 2 (bad shell name).

## `help`

```
shellboto help
shellboto -h
shellboto --help
```

List subcommands with short descriptions.

Exit: 0.

## `-version`

```
shellboto -version
shellboto --version
```

Print: `shellboto version=X.Y.Z gitSHA=<short> built=<iso-8601>`.

Exit: 0.

## No-subcommand default

Start the bot. Long-polls Telegram until SIGTERM.

Under systemd: `ExecStart=/usr/local/bin/shellboto -config /etc/shellboto/config.toml`.

## Exit codes (summary)

| Code | Meaning |
|------|---------|
| 0 | success |
| 1 | error (DB access, parse, I/O) |
| 2 | usage (bad flag, missing argument) |
| 3 | check failed (doctor, audit verify, simulate match) |

See [exit-codes.md](exit-codes.md).
