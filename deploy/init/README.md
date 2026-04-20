# deploy/init/ — non-systemd unit examples

Templates for operators on Alpine, Void, Gentoo, or other distros that
don't use systemd. Pair with `sudo ./deploy/install.sh --skip-systemd`
which places the binary, config, and env file in the right locations
without touching `systemctl`.

All three templates source `/etc/shellboto/env` to get
`SHELLBOTO_TOKEN` / `SHELLBOTO_SUPERADMIN_ID` / `SHELLBOTO_AUDIT_SEED`,
then exec `/usr/local/bin/shellboto -config /etc/shellboto/config.toml`.

## OpenRC (Alpine / Gentoo)

```bash
sudo install -m 0755 deploy/init/openrc/shellboto /etc/init.d/shellboto
sudo rc-update add shellboto default
sudo rc-service shellboto start
```

Logs go to `/var/log/shellboto.log` by default — change `output_log`
in the init script to redirect.

## runit (Void)

```bash
sudo mkdir -p /etc/sv/shellboto /etc/sv/shellboto/log
sudo install -m 0755 deploy/init/runit/shellboto/run     /etc/sv/shellboto/run
sudo install -m 0755 deploy/init/runit/shellboto/log/run /etc/sv/shellboto/log/run
sudo ln -s /etc/sv/shellboto /var/service/shellboto
```

Supervision starts automatically. Logs go to `/var/log/shellboto/` via
`svlogd`. To stop / restart:

```bash
sudo sv down shellboto
sudo sv up shellboto
sudo sv status shellboto
```

## s6 / s6-rc

```bash
sudo mkdir -p /etc/s6-rc/source/shellboto
sudo install -m 0755 deploy/init/s6/shellboto/run  /etc/s6-rc/source/shellboto/run
sudo install -m 0644 deploy/init/s6/shellboto/type /etc/s6-rc/source/shellboto/type
sudo s6-rc-compile /etc/s6-rc/compiled /etc/s6-rc/source
sudo s6-rc-update  /etc/s6-rc/compiled
sudo s6-rc -u change shellboto
```

## Logs

Without journald, `shellboto service logs` from the CLI doesn't apply —
use whichever log viewer your init system ships with (`tail
/var/log/shellboto.log`, `sv check shellboto`, `s6-svstat`).
