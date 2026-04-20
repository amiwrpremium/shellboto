# runit

Non-systemd deployment for Void Linux (native), Alpine, or any
distro you've added runit to.

## Install

```bash
sudo ./deploy/install.sh --skip-systemd
sudo mkdir -p /etc/sv/shellboto
sudo cp deploy/init/runit/shellboto/run /etc/sv/shellboto/run
sudo chmod 0755 /etc/sv/shellboto/run

# Log service
sudo mkdir -p /etc/sv/shellboto/log
sudo cp deploy/init/runit/shellboto/log/run /etc/sv/shellboto/log/run
sudo chmod 0755 /etc/sv/shellboto/log/run

# Enable
sudo ln -s /etc/sv/shellboto /var/service/
```

## The `run` script

```bash
#!/bin/sh
set -e
exec 2>&1
exec env - \
    $(grep -v '^#' /etc/shellboto/env | xargs) \
    /usr/local/bin/shellboto -config /etc/shellboto/config.toml
```

What's happening:

- **`exec 2>&1`** — merges stderr into stdout so `log/run`
  captures both.
- **`env -`** — blank environment + only the vars read from the
  env file.
- **`exec <binary>`** — replace the shell with shellboto.

runit's `runsv` supervisor sees the binary's PID directly and
restarts on exit.

## The `log/run` script

```bash
#!/bin/sh
exec svlogd -tt /var/log/shellboto
```

`svlogd` is runit's log-tailing daemon. `-tt` prefixes each line
with a TAI64N timestamp. Rotated by size (default 1 MiB).

## Differences from systemd / OpenRC

- **No EnvironmentFile-style mechanism.** Script parses the env
  file manually.
- **No journald.** Logs land as plain text under
  `/var/log/shellboto/`. Rotation handled by svlogd.
- **Supervision is tight.** runsv restarts on any exit (no
  "on-failure" distinction — if you stop via `sv stop`, it
  doesn't restart until you `sv start`).

## Starting / stopping

```bash
sudo sv up shellboto          # start
sudo sv down shellboto        # stop
sudo sv restart shellboto
sudo sv status shellboto
```

## Logs

```bash
sudo tail -f /var/log/shellboto/current
```

TAI64N timestamps at the start of each line. Translate to human
with `tai64nlocal`:

```bash
sudo tail -f /var/log/shellboto/current | tai64nlocal
```

## Note on shellcheck

The `run` file doesn't end in `.sh`; shellcheck hooks need a
special glob to find it (we handle that in `.lefthook.yml`).

## Reading the code

- `deploy/init/runit/shellboto/run`
- `deploy/init/runit/shellboto/log/run`

## Read next

- [s6.md](s6.md) — the related but different s6 family.
- [systemd.md](systemd.md).
