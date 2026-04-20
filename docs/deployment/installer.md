# The installer — `deploy/install.sh`

Interactive + `-y` mode installer. Idempotent: re-run to upgrade
without losing config.

Source: [`deploy/install.sh`](../../deploy/install.sh).

## Flags

| Flag | Purpose |
|------|---------|
| `-y`, `--yes` | No prompts. Required env vars + flags must supply values. |
| `--superadmin-id N` | Telegram ID of superadmin. Required in `-y`. |
| `--config-format toml\|yaml\|json` | Config file format. Required in `-y`. Default in interactive: TOML. |
| `--audit-seed HEX` | Reuse an existing 64-char hex seed instead of generating one. |
| `--skip-build` | Don't run `make build`; use existing `bin/shellboto`. |
| `--skip-systemd` | Don't install unit / run systemctl. Use for OpenRC / runit / s6 / containerised. |
| `--prefix DIR` | Install into `<dir>/etc/…`, `<dir>/usr/local/bin/…`. For chroots. |
| `--dry-run` | Print actions without doing them. |

## Required env in `-y` mode

- `SHELLBOTO_TOKEN` (required)
- `SHELLBOTO_AUDIT_SEED` (recommended; auto-generated if unset)

## Seven steps (interactive walkthrough)

### Step 1 — build the binary

`make build` → `bin/shellboto` with version stamp. Skip with
`--skip-build`.

Needs Go 1.26+ on the host. If not present:

```
error: go 1.26+ not found.
hint: install from https://go.dev/doc/install, or pass --skip-build
      if bin/shellboto already exists.
```

### Step 2 — install the binary

- Stops the service if running (so the old binary isn't in use).
- `install -m 0755 bin/shellboto /usr/local/bin/shellboto.new`
- `mv /usr/local/bin/shellboto /usr/local/bin/shellboto.prev` (if
  present).
- `mv /usr/local/bin/shellboto.new /usr/local/bin/shellboto`.

Atomic replacement; rollback possible via `rollback.sh`.

### Step 3 — create state directories

```
/etc/shellboto/         0700 root:root
/var/lib/shellboto/     0700 root:root
```

Pre-created so `doctor` can see them. systemd's `StateDirectory=`
normally handles `/var/lib/...` but the installer bootstraps it.

### Step 4 — configure env

`/etc/shellboto/env` (0600, root:root):

```
SHELLBOTO_TOKEN=...
SHELLBOTO_SUPERADMIN_ID=...
SHELLBOTO_AUDIT_SEED=...
```

Interactive prompts:

- Token: `read -rs` (no echo). Paste from @BotFather.
- Superadmin ID: plain `read`. Validated as positive int64.
- Audit seed: auto-generated via `openssl rand -hex 32` unless
  you passed `--audit-seed` or the env file already has one from
  a previous install.

Upgrades preserve existing values. You aren't re-prompted unless
you deliberately edit the file aside first.

### Step 5 — write config file

Installer asks:

```
Config format?
  1) TOML (recommended)
  2) YAML
  3) JSON
```

Writes `/etc/shellboto/config.<ext>` (0600). Copies from
`deploy/config.example.<ext>`. Comments preserved (TOML / YAML).

Upgrades preserve your config; not overwritten.

### Step 6 — install systemd unit

```bash
install -m 0644 deploy/shellboto.service /etc/systemd/system/
systemctl daemon-reload
```

`--skip-systemd` skips this step entirely.

### Step 7 — start the service

```bash
systemctl enable --now shellboto
```

Then runs `shellboto doctor`. Installer exits after showing the
green preflight.

## Idempotence

Every step checks current state before acting:

- Dir already at correct perms → skipped.
- Env file present with valid entries → used as-is.
- Config file present → kept.
- Binary same hash as the one being installed → skipped.
- Service already active → restarted (new binary).

So `sudo ./deploy/install.sh` after `git pull && make build` is
the upgrade path. No separate upgrade command.

## Error handling + rollback

`install.sh` installs an `ERR` trap. On any step failing:

- Rollback queued-up changes (undo file writes, undo dir creations
  where safe, undo binary installs).
- Print a clear error with the failing command.
- Exit non-zero.

Rollback doesn't undo changes from prior invocations (e.g. it
won't delete a pre-existing env file). Only partial progress from
the current run.

## Logging

Installer prints to stderr by default:

- Section headers in bold.
- ✓ / ⚠ / ✗ per step.

No JSON output. If you need machine-readable installs, build your
own wrapper that parses exit codes.

## What it doesn't do

- **Doesn't set up `user_shell_user`.** You do that separately;
  see [../configuration/non-root-shells.md](../configuration/non-root-shells.md).
- **Doesn't configure firewall.** No inbound ports needed; skip.
- **Doesn't configure log rotation for journald.** Journald
  handles its own rotation.
- **Doesn't install monitoring.** Bring your own.

## Reading the code

- `deploy/install.sh` — top-level flow.
- `deploy/lib.sh` — shared helpers (color, prompt, trap, etc.).

## Read next

- [systemd.md](systemd.md) — the unit file field-by-field.
- [rollback.md](rollback.md) — when step 2 hands back an `.prev`.
