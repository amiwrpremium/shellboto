# Installation

Two supported paths: the **interactive installer** (recommended for
first install on a VPS), and **`-y` / non-interactive mode** (for
CI, Ansible, re-runs).

## Interactive install

```bash
ssh root@your-vps
git clone https://github.com/amiwrpremium/shellboto.git
cd shellboto
sudo ./deploy/install.sh
```

The installer walks through 7 steps. Each prints a clear progress
line and rolls back on failure.

### Step 1 — build the binary

Runs `make build`. Produces `bin/shellboto` with the version
stamp (`--version` will show the git SHA + build timestamp). No
CGO, pure-Go, one static binary.

Skip if you already built: `--skip-build`.

### Step 2 — install the binary

Copies `bin/shellboto` to `/usr/local/bin/shellboto` (mode 0755,
root:root). If `/usr/local/bin/shellboto` already exists, it's
copied to `/usr/local/bin/shellboto.prev` first — that's what
`rollback.sh` flips back to.

Stops the running service if any, then copies, then you'll start
it again in step 7.

### Step 3 — create state directories

```
/etc/shellboto/         0700   root:root   # config + env
/var/lib/shellboto/     0700   root:root   # audit DB lives here (0600 on the DB file itself)
```

`/var/lib/shellboto` is normally created by systemd via the
`StateDirectory=shellboto` directive, but the installer pre-creates
it so `doctor` has somewhere to look.

### Step 4 — configure the environment file

Writes `/etc/shellboto/env` (0600, root:root):

```
SHELLBOTO_TOKEN=...
SHELLBOTO_SUPERADMIN_ID=...
SHELLBOTO_AUDIT_SEED=...
```

- **Token**: prompted with `read -rs` (no echo). Paste the 46-char
  string from @BotFather.
- **Superadmin ID**: numeric Telegram ID you found in the previous
  doc. Validated (positive int).
- **Audit seed**: auto-generated via `openssl rand -hex 32`. You can
  paste an existing seed via `--audit-seed HEX` to preserve chain
  continuity after a reinstall.

On upgrades: existing values are detected and kept unless you
explicitly re-prompt.

### Step 5 — write the config file

You're asked for the format:

- `1) TOML` — recommended unless you have infra-wide YAML/JSON.
- `2) YAML`
- `3) JSON`

The installer copies the matching `deploy/config.example.<fmt>` to
`/etc/shellboto/config.<fmt>` (0600, root:root). Skip customisation —
defaults are sane for most deployments.

Override: `--config-format toml` / `yaml` / `json`.

### Step 6 — install the systemd unit

Copies `deploy/shellboto.service` to `/etc/systemd/system/`
(0644), then `systemctl daemon-reload`.

`--skip-systemd` leaves the unit uninstalled — useful for OpenRC /
runit / s6 deployments (see [../deployment/](../deployment/)).

### Step 7 — start the service

`systemctl enable --now shellboto`. The service should reach
`active (running)` within a second. Installer then runs
`shellboto doctor` — you see green preflight ticks before the
installer exits.

If it doesn't start: `sudo journalctl -u shellboto -n 200`.

## Non-interactive install (`-y` mode)

For CI and Ansible:

```bash
sudo SHELLBOTO_TOKEN='...' \
     SHELLBOTO_AUDIT_SEED="$(openssl rand -hex 32)" \
     ./deploy/install.sh -y \
     --superadmin-id 123456789 \
     --config-format toml
```

All flags:

| Flag | Purpose |
|------|---------|
| `-y`, `--yes` | No prompts. All required values must come from env + flags. |
| `--superadmin-id <n>` | Required in `-y` mode. |
| `--config-format toml\|yaml\|json` | Required in `-y` mode. |
| `--audit-seed <hex>` | Skip generation; reuse existing seed. |
| `--skip-build` | Use existing `bin/shellboto`. |
| `--skip-systemd` | Skip unit install + systemctl. |
| `--prefix <dir>` | Install into `<dir>/etc/...`, `<dir>/usr/local/bin/...`. For chroots / containers. |
| `--dry-run` | Print what would happen; change nothing. |

### Required env in `-y` mode

- `SHELLBOTO_TOKEN` — **required**.
- `SHELLBOTO_AUDIT_SEED` — **recommended** (else installer generates
  one). If you're setting up multiple hosts that share an audit seed
  (not usual), supply it explicitly.

## Re-running / upgrading

Run the installer again after pulling a new version:

```bash
git pull
sudo ./deploy/install.sh
```

It detects:

- Existing `/etc/shellboto/env` → keeps your token / superadmin /
  seed without re-prompting.
- Existing `/etc/shellboto/config.*` → preserves your customisations.
- Existing binary → saves it as `.prev` before installing the new one.

Then restarts the service.

## Rollback

```bash
sudo ./deploy/rollback.sh
```

Swaps `/usr/local/bin/shellboto` ↔ `/usr/local/bin/shellboto.prev`
via atomic rename. Stops the service before the swap, starts it
after. Reversible — re-run to flip back to the newer version.

Fresh installs have nothing to roll back to; `rollback.sh` refuses
with a clear message.

Full detail: [../deployment/rollback.md](../deployment/rollback.md).

## Uninstall

```bash
sudo ./deploy/uninstall.sh              # safe: removes binary + unit, keeps config + DB
sudo ./deploy/uninstall.sh --remove-config
sudo ./deploy/uninstall.sh --remove-config --remove-state --i-understand-this-deletes-audit-log -y
```

Audit DB deletion requires either typing a confirmation phrase
(interactive) or the `--i-understand-...` flag (`-y` mode) — so a
stray `yes | ./uninstall.sh` can't wipe audit history.

Full detail: [../deployment/uninstall.md](../deployment/uninstall.md).

## What the installer **doesn't** do

- Install a firewall. shellboto doesn't open any ports; your firewall
  doesn't need rules for it.
- Create unprivileged user shell accounts. If you want `user`-role
  callers to get non-root shells, read
  [../configuration/non-root-shells.md](../configuration/non-root-shells.md).
- Set up monitoring / alerting. See [../operations/monitoring.md](../operations/monitoring.md).
- Rotate the audit seed. Rotate manually per
  [../security/audit-seed.md](../security/audit-seed.md).

Proceed to [first-commands.md](first-commands.md).
