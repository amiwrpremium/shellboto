# Configuration

How to configure shellboto: file formats, every key, every env var,
role matrix, non-root-shell setup, timeouts, audit output modes.

| File | What it covers |
|------|----------------|
| [formats.md](formats.md) | TOML vs YAML vs JSON — same schema, pick by file extension |
| [schema.md](schema.md) | Every config key: type, default, meaning, example |
| [environment.md](environment.md) | `SHELLBOTO_TOKEN`, `_SUPERADMIN_ID`, `_AUDIT_SEED` |
| [roles.md](roles.md) | superadmin / admin / user capability matrix |
| [non-root-shells.md](non-root-shells.md) | Setting up unprivileged user shells — the symlink-attack pitfall |
| [timeouts-and-reaping.md](timeouts-and-reaping.md) | `default_timeout`, `idle_reap`, `heartbeat`, `edit_interval`, `kill_grace` |
| [audit-output-modes.md](audit-output-modes.md) | `audit_output_mode = always \| errors_only \| never` trade-offs |

## Mental model

shellboto reads config from **three sources**, merged in this order
(later wins):

1. **Defaults in code.** Every config field has a sensible default.
   You can run with an essentially empty config file.
2. **Config file.** One of `/etc/shellboto/config.{toml,yaml,json}`
   — pick one, installer picks it for you. Same schema regardless
   of format.
3. **Environment variables.** Only for three values, all secrets /
   identity: `SHELLBOTO_TOKEN` (required), `SHELLBOTO_SUPERADMIN_ID`
   (required), `SHELLBOTO_AUDIT_SEED` (recommended). Env wins over
   file.

**Not supported:** arbitrary env-var overrides for every config key.
Keep it simple — if you want to change `idle_reap`, edit the file.

## Where the file lives

Default: `/etc/shellboto/config.toml` (or `.yaml` / `.json`).
Override with `-config <path>` on the CLI:

```bash
shellboto -config /tmp/alt.toml doctor
```

systemd's `ExecStart=` hardcodes the default path; if you've
changed it, update the unit too.

## Permissions

```
/etc/shellboto/                0700 root:root
/etc/shellboto/env             0600 root:root   # secrets
/etc/shellboto/config.toml     0600 root:root   # non-secret but private
```

The config file contains no secrets directly — token, superadmin
ID, and audit seed are in the env file. But it does contain paths,
patterns, and policy knobs that would leak deployment details to a
reader, so 0600 is correct.

## Reading the current effective config

```bash
shellboto config check /etc/shellboto/config.toml
```

Prints the parsed + validated view. Use this to sanity-check after
an edit but before restart.

## Reloading

Config is read **once at startup**. Changes require a restart:

```bash
sudo systemctl restart shellboto
```

No signal-based reload, no config-watch. Deliberate: the audit
chain treats the config (specifically `audit_output_mode`) as part
of the deployment's posture — swapping it mid-stream without a
restart would muddy the forensic record.

## Read next

- [formats.md](formats.md) — pick a format.
- [schema.md](schema.md) — the exhaustive key reference.
