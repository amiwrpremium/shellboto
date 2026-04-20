# deploy/

Interactive install + uninstall scripts for shellboto, plus the example
config and systemd unit they install.

## Quick start

```bash
# Fresh install
sudo ./deploy/install.sh

# Upgrade (keeps env + config, replaces binary, restarts)
make build && sudo ./deploy/install.sh

# Rollback to the previous binary (the installer kept a copy at
# /usr/local/bin/shellboto.prev on the upgrade above). Reversible:
# re-run to swap back.
sudo ./deploy/rollback.sh

# Uninstall (keeps config + state DB by default)
sudo ./deploy/uninstall.sh

# Full wipe (CI-friendly)
sudo ./deploy/uninstall.sh -y --remove-config --remove-state \
     --i-understand-this-deletes-audit-log
```

## Safety features

- **Token is never echoed or logged.** The installer reads it with
  `read -rs` (stdin silent) and only writes it to `/etc/shellboto/env`
  at mode `0600`. The variable is unset after use so it doesn't linger
  in the script environment.
- **Audit seed is auto-generated.** `openssl rand -hex 32` — never
  prompts, never shown. Pass `--audit-seed HEX` to re-use a specific
  value (for IaC-managed seeds).
- **Idempotent.** Re-running the installer detects existing env + config
  and asks whether to keep them. Backups of replaced files are written
  alongside (`env.bak.YYYYMMDD-HHMMSS`).
- **Typed confirmation** for the only truly destructive action. Running
  `uninstall.sh --remove-state` interactively makes you type
  `DELETE-SHELLBOTO-<size>` verbatim — no muscle-memory yes.
- **Rollback on error.** A mid-install failure unwinds the completed
  steps (service restart, file copies) rather than leaving the system
  in a half-configured state.
- **Colored output auto-disables** on non-TTY / `NO_COLOR=1` / `TERM=dumb`.

## Files

| File                      | Purpose |
|---------------------------|---------|
| `install.sh`              | Interactive installer; also `-y` for CI. |
| `rollback.sh`             | Swap the installed binary with its `.prev` backup. Reversible — re-run to swap back. |
| `uninstall.sh`            | Safe-by-default uninstaller. |
| `lib.sh`                  | Shared helpers (colors, prompts, env IO, rollback). Sourced by both scripts — not executable on its own. |
| `lib_test.sh`             | Unit tests for the pure helpers (`validate_*`, `read_env_value`, `write_env_value`, `human_bytes`). |
| `env.example`             | Template for `/etc/shellboto/env`. |
| `config.example.toml`     | TOML config template. |
| `config.example.yaml`     | YAML config template. |
| `config.example.json`     | JSON config template. |
| `shellboto.service`       | systemd unit, installed to `/etc/systemd/system/`. |

## Install flags

```
sudo ./deploy/install.sh [flags]

  -y, --yes                 non-interactive; env vars must supply secrets
      --superadmin-id N     Telegram user ID of the superadmin
      --config-format FMT   toml | yaml | json (default toml)
      --audit-seed HEX      re-use a specific 32-byte hex seed
      --skip-build          assume bin/shellboto already exists
      --skip-systemd        install files only, don't touch systemctl
      --prefix DIR          install under DIR instead of / (chroots, Docker)
      --dry-run             print every action without running
  -h, --help                show help
```

Non-interactive env vars: `SHELLBOTO_TOKEN` (required in `-y`),
`SHELLBOTO_SUPERADMIN_ID`, `SHELLBOTO_AUDIT_SEED`.

## Rollback flags

```
sudo ./deploy/rollback.sh [flags]

  -y, --yes          non-interactive (skip the swap confirm)
      --dry-run      print every action without making changes
      --prefix DIR   operate under DIR
  -h, --help         show help
```

How it works: `install.sh` copies the current binary to
`/usr/local/bin/shellboto.prev` every time it upgrades.
`rollback.sh` swaps current ↔ previous via an atomic rename dance.
The service is stopped before the swap, started after. Running
rollback twice returns you to the originally-installed version, so
it's safe to use as a toggle.

Fresh installs (no prior upgrade) have no `.prev` yet; rollback will
refuse with a clear message in that case.

## Uninstall flags

```
sudo ./deploy/uninstall.sh [flags]

  -y, --yes                                    non-interactive
      --remove-config                          also delete /etc/shellboto/
      --remove-state                           also delete /var/lib/shellboto/ (audit DB)
      --i-understand-this-deletes-audit-log    required alongside --remove-state in -y mode
      --prefix DIR                             operate under DIR
      --dry-run                                print every action
  -h, --help                                   show help
```
