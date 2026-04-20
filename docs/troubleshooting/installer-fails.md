# Installer failures

`install.sh` is idempotent + has an `ERR` trap that rolls back
partial progress. Common failure modes:

## "go 1.26+ not found"

You're on Go 1.25 or older. Install 1.26+:

```bash
# https://go.dev/doc/install
wget https://go.dev/dl/go1.26.2.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.26.2.linux-amd64.tar.gz
export PATH="/usr/local/go/bin:$PATH"
```

Or pass `--skip-build` and provide a pre-built `bin/shellboto`.

## "make: No such file"

Install make:

```bash
sudo apt install make
# or
sudo dnf install make
```

## "permission denied" on /etc/shellboto

You forgot `sudo`. The installer needs root.

```bash
sudo ./deploy/install.sh
```

## "another shellboto is running"

You ran the installer (or a CLI subcommand) while the service was
mid-restart. Wait a few seconds, retry.

If that doesn't help, find the holder:

```bash
sudo lsof /var/lib/shellboto/shellboto.lock
```

Investigate that PID. Probably a stale shellboto somewhere — do
NOT just delete the lockfile (the kernel still holds the lock on
the inode until the holding process exits). Kill the PID
gracefully if confirmed stale.

## "EnvironmentFile=/etc/shellboto/env: No such file"

Systemd can't read the env. Check it exists + has correct perms:

```bash
sudo ls -la /etc/shellboto/env
# -rw------- 1 root root ... /etc/shellboto/env
```

If missing, the installer didn't run step 4 to completion. Re-run.

## "SHELLBOTO_TOKEN required" at startup

Env file present but the variable is empty / missing. Check:

```bash
sudo grep SHELLBOTO_TOKEN /etc/shellboto/env
```

Should be `SHELLBOTO_TOKEN=123456789:...`. If empty: paste a real
token. If shaped wrong: re-paste from @BotFather.

## "Bad config: …" at startup

Your `config.toml` (or .yaml / .json) failed validation. The
error message tells you which key. Common:

- Bad duration string (`"5 minutes"` instead of `"5m"`).
- Bad enum value (`"loud"` instead of `"json"` for `log_format`).
- Path that doesn't exist (`db_path = "/nonexistent/state.db"`).

Fix + re-validate without restarting:

```bash
sudo shellboto config check /etc/shellboto/config.toml
```

## "user_shell_user X: no such user"

`user_shell_user` in config but the unix account doesn't exist.
Either:

```bash
# create the account
sudo useradd --system --shell /bin/bash shellboto-user
```

or remove the config key (revert to dev-mode root shells for
role=user).

## Installer hangs at one step

Most likely the build step taking longer than expected on a
slow CPU. `make build` shouldn't exceed ~30 seconds. If it
seems frozen:

```bash
# Are go processes running?
ps aux | grep go
```

If yes, wait. If no, the script has a bug — file an issue with
the output of `bash -x ./deploy/install.sh`.

## Read next

- [bot-not-responding.md](bot-not-responding.md) — once installed,
  if it doesn't talk.
- [../deployment/installer.md](../deployment/installer.md) —
  installer reference.
