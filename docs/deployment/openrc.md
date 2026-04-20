# OpenRC

Non-systemd deployment for Alpine / Gentoo / Void.

## Install script

`deploy/init/openrc/shellboto` — ready to drop into `/etc/init.d/`.

```bash
sudo ./deploy/install.sh --skip-systemd
sudo cp deploy/init/openrc/shellboto /etc/init.d/shellboto
sudo chmod 0755 /etc/init.d/shellboto
sudo rc-update add shellboto default
sudo rc-service shellboto start
```

`--skip-systemd` tells the installer to leave systemd alone;
everything else (binary, env, config, state dir) is the same.

## The init file

```bash
#!/sbin/openrc-run
# shellcheck shell=sh
# shellcheck disable=SC2034  # openrc reads these vars via its framework
# shellcheck disable=SC1091  # /etc/shellboto/env materialises at install time

name="shellboto"
description="Telegram shell bot"

command="/usr/local/bin/shellboto"
command_args="-config /etc/shellboto/config.toml"
command_user="root"
command_background="yes"
pidfile="/run/${RC_SVCNAME}.pid"
output_log="/var/log/shellboto.log"
error_log="/var/log/shellboto.log"

depend() {
    need net
    after firewall
}

start_pre() {
    checkpath -d -m 0700 /var/lib/shellboto
    [ -r /etc/shellboto/env ] || eerror "missing /etc/shellboto/env"
    set -a
    . /etc/shellboto/env
    set +a
}
```

## What it does

- **`depend() { need net; }`** — won't start until networking is up.
- **`command_background="yes"`** — OpenRC starts it as a
  background process. PID tracked via `pidfile`.
- **`start_pre()`** — creates `/var/lib/shellboto` if missing
  (OpenRC doesn't have `StateDirectory=`). Sources the env file
  so `SHELLBOTO_TOKEN` etc. reach the process.

## Differences from systemd

| Concern | systemd | OpenRC |
|---------|---------|--------|
| State dir | `StateDirectory=` auto-creates | `start_pre` + `checkpath` manual |
| Env file | `EnvironmentFile=` | `set -a; . ...; set +a` in start_pre |
| Logs | journald (structured) | Plain log files under `/var/log/` |
| Hardening | `NoNewPrivileges=`, etc. | Not directly; use `rc_cgroup_*` sparingly |
| Restart | `Restart=on-failure` | `rc_ulimit` + default supervision |

## Log rotation

OpenRC doesn't rotate logs. Wire up `logrotate`:

```
# /etc/logrotate.d/shellboto
/var/log/shellboto.log {
    weekly
    rotate 12
    compress
    delaycompress
    missingok
    notifempty
    copytruncate
}
```

## Hardening under OpenRC

OpenRC doesn't offer systemd's rich sandboxing. Use:

- Dedicated non-root user (see the note in [systemd.md](systemd.md#userroot)
  about limitations).
- Cgroup v2 resource limits via `rc_cgroup_settings`.
- AppArmor / SELinux profile if your distro supports it.

## Starting / stopping

```bash
sudo rc-service shellboto start
sudo rc-service shellboto stop
sudo rc-service shellboto restart
sudo rc-service shellboto status
```

## Reading the code

- `deploy/init/openrc/shellboto`

## Read next

- [runit.md](runit.md) / [s6.md](s6.md) — other init systems.
- [systemd.md](systemd.md) — the most common path.
