# Deployment

Getting shellboto onto a host and keeping it there.

| File | What it covers |
|------|----------------|
| [installer.md](installer.md) | `deploy/install.sh` deep-dive |
| [systemd.md](systemd.md) | The unit file, field by field |
| [openrc.md](openrc.md) | `deploy/init/openrc/` init script |
| [runit.md](runit.md) | `deploy/init/runit/` service dirs |
| [s6.md](s6.md) | `deploy/init/s6/` — note: execlineb, not sh |
| [uninstall.md](uninstall.md) | `deploy/uninstall.sh`, safe defaults |
| [rollback.md](rollback.md) | `deploy/rollback.sh`, atomic swap |
| [production-checklist.md](production-checklist.md) | Pre-flight list before going live |

## Target host assumptions

- **Linux.** Systemd preferred; OpenRC / runit / s6 supported via
  the init scripts under `deploy/init/`.
- **Root access.** Installer writes to `/etc`, `/var/lib`,
  `/usr/local/bin`.
- **Outbound HTTPS to `api.telegram.org:443`.** No inbound ports.
- **A couple hundred MB disk** for binary + DB + logs. Typical
  install: ~30 MiB binary + < 100 MiB DB initially.

## Paths it owns

```
/usr/local/bin/shellboto            0755 root:root   # the binary
/usr/local/bin/shellboto.prev       0755 root:root   # previous binary (for rollback)
/etc/shellboto/                     0700 root:root   # config dir
/etc/shellboto/env                  0600 root:root   # secrets (token, superadmin, seed)
/etc/shellboto/config.toml          0600 root:root   # runtime config
/etc/systemd/system/shellboto.service 0644 root:root # unit
/var/lib/shellboto/                 0700 root:root   # state dir
/var/lib/shellboto/state.db         0600 root:root   # SQLite
/var/lib/shellboto/shellboto.lock   0600 root:root   # flock
```

Plus whatever the user-role shells' home dirs resolve to (see
[../configuration/non-root-shells.md](../configuration/non-root-shells.md)).

## Supported install methods

- **`deploy/install.sh`** — interactive or `-y`, idempotent.
  Primary path.
- **`.deb` / `.rpm`** — goreleaser-produced packages. No interactive
  prompts; see [packaging/](../packaging/) to know what they do.
- **`brew install amiwrpremium/shellboto/shellboto`** (macOS) —
  CLI only; the bot itself doesn't run on macOS.
- **Manual** — copy binary, write env + config + unit by hand. Use
  for containerised / stripped-down setups.

## Read next

- [installer.md](installer.md) — the most common path.
- [production-checklist.md](production-checklist.md) — before you
  announce the bot to your team.
