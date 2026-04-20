# systemd unit

`deploy/shellboto.service`, line by line.

## The unit

```ini
[Unit]
Description=shellboto — Telegram shell bot
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
EnvironmentFile=/etc/shellboto/env
ExecStart=/usr/local/bin/shellboto -config /etc/shellboto/config.toml
StateDirectory=shellboto
StateDirectoryMode=0700
Restart=on-failure
RestartSec=3s
StandardOutput=journal
StandardError=journal
NoNewPrivileges=yes
RestrictSUIDSGID=yes
LockPersonality=yes
RestrictRealtime=yes
SystemCallArchitectures=native
TasksMax=500

[Install]
WantedBy=multi-user.target
```

## `[Unit]`

- **`Description=`** — human-readable name shown in `systemctl status`.
- **`After=network-online.target`** — start order: wait for the
  network to be up. Telegram needs outbound HTTPS.
- **`Wants=network-online.target`** — weak dep on bringing up
  network-online before us. `After=` is ordering; `Wants=` is
  activation.

## `[Service]`

### `Type=simple`

Bot runs in the foreground, doesn't fork. systemd tracks the
process directly.

### `User=root`

Needed because shellboto spawns shells, some of which run as
root (admin/superadmin shells). If you configure all shells as
non-root via `user_shell_user`, you could in theory drop the bot
itself to a non-root uid — but the `SpawnOpts.Creds` logic needs
to be able to `setuid/setgid` to different users, which requires
root (or `CAP_SETUID`). Dropping privs here would need a
capabilities-based setup that we haven't shipped.

If you do want to run under a dedicated non-root uid and accept
the limitation that admin shells can't drop to root later, read
the SpawnOpts code and test thoroughly before production.

### `EnvironmentFile=/etc/shellboto/env`

systemd reads this before `ExecStart`. Format: `KEY=VALUE` per
line, `#` comments. Becomes the initial process environment.

See [../configuration/environment.md](../configuration/environment.md).

### `ExecStart=/usr/local/bin/shellboto -config /etc/shellboto/config.toml`

The one and only command. If you installed the config as `.yaml`
or `.json`, change the `-config` path accordingly. `install.sh`
edits this line per your chosen format.

### `StateDirectory=shellboto`

systemd creates `/var/lib/shellboto/` at service start if it
doesn't exist. Owned by `User=root` → root:root. Mode:

### `StateDirectoryMode=0700`

0700 because the DB + lockfile are root-only.

### `Restart=on-failure` + `RestartSec=3s`

Auto-restart on non-zero exit. Wait 3s between attempts.
Handles transient failures (DB briefly locked during vacuum,
etc.) without human touch.

`Restart=on-failure` means clean exits (SIGTERM during `systemctl
stop`) don't trigger restart. Crashes do.

### `StandardOutput=journal` + `StandardError=journal`

All bot output goes to journald. `journalctl -u shellboto` reads
it. zap emits JSON; journald stores + searches structured
fields.

### `NoNewPrivileges=yes`

Kernel-level flag: the process and all children can't gain
privileges via setuid binaries. This is a defense-in-depth
measure against rogue shell commands attempting `sudo`-style
escalation from within the bot process tree.

Note: this doesn't block **us** from dropping privileges via
`SysProcAttr.Credential` — that's a setresuid syscall, which
works under `NoNewPrivileges`. It blocks *gaining* privs via
setuid binaries on exec.

### `RestrictSUIDSGID=yes`

Related: the process can't create files with setuid/setgid bits.
Prevents a compromised shell from planting a setuid backdoor.

### `LockPersonality=yes`

Blocks `personality(2)` syscall (changing execution personality
— rarely legitimate; historically used in exploits).

### `RestrictRealtime=yes`

Blocks real-time scheduling (`SCHED_FIFO`, etc.). Real-time
priority on a bot = no reason; useful to deny.

### `SystemCallArchitectures=native`

Blocks syscalls from non-native architectures (e.g. 32-bit
syscalls on a 64-bit kernel) — reduces the kernel's attack
surface by ~1000 syscalls.

### `TasksMax=500`

Hard fork-bomb containment. The bot's cgroup can have at most 500
processes. Since each command spawns a shell and maybe a few
children, 500 is generous.

If you have very large teams running big build commands, bump
this. Default is fine for solo + small-team deployments.

## What's explicitly NOT set

- **`ProtectSystem=`** — would mount `/usr` + `/boot` read-only.
  Breaks admin shells writing to `/usr/local/bin` (updates).
- **`ProtectHome=`** — would hide `/home`. Breaks user-role
  shells under `/home/shellboto-user/…`.
- **`PrivateTmp=`** — would give the bot its own `/tmp`. Breaks
  users writing there and expecting to see the files from
  other tools.
- **`PrivateDevices=`** — would hide devices. Probably fine for
  most use; didn't bother.

If you want these, you can override in a drop-in:

```
# /etc/systemd/system/shellboto.service.d/hardening.conf
[Service]
PrivateDevices=yes
ProtectKernelModules=yes
```

Test carefully — some break user workflows.

## `[Install]`

### `WantedBy=multi-user.target`

`systemctl enable` creates a symlink
`/etc/systemd/system/multi-user.target.wants/shellboto.service`
→ our unit. Starts at boot once `multi-user.target` is reached.

## Viewing status

```bash
systemctl status shellboto
journalctl -u shellboto -f
journalctl -u shellboto -n 200
```

## Restarting

```bash
sudo systemctl restart shellboto
```

Runs when you've changed config or env. Graceful shutdown + fresh
start; new config takes effect immediately.

## Reading the code

- `deploy/shellboto.service` — the unit file.
- `packaging/postinstall.sh` — what the .deb/.rpm post-install
  does (daemon-reload, etc).

## Read next

- [openrc.md](openrc.md) / [runit.md](runit.md) / [s6.md](s6.md)
  — non-systemd deployment.
- [../operations/logs.md](../operations/logs.md) — how to read the
  journal.
