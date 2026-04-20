# Non-root shells for `role=user` callers

By default, every pty spawned by shellboto runs as root — including
shells for accounts with `role=user`. That matches the original
project brief (one operator, one VPS) but is **not** what you want
if you're whitelisting anyone other than yourself.

This doc walks through setting it up, *including* the symlink-
attack pitfall that several other setups quietly get wrong.

## Outcome

After this, `role=user` shells:

- Run as an unprivileged system account (no sudoers entries).
- Have their own per-telegram-user `$HOME` under a managed base
  directory.
- Can't read root-owned files (`/etc/shadow`, `/root/.ssh/*`).
- Can't write to system paths (`/etc/*`, `/usr/*`).

`role=admin` and `role=superadmin` shells are unchanged — always
root.

## Setup

### 1. Create the shell user

```bash
sudo useradd --system --shell /bin/bash shellboto-user
sudo sudo -l -U shellboto-user
```

The `sudo -l` check should say `User shellboto-user is not allowed
to run sudo`. If it says anything else, you've got sudoers entries
— remove them before proceeding.

### 2. Create the home-base directory — **carefully**

This is the step people mess up:

```bash
sudo mkdir -p /home/shellboto-user
sudo chown root:shellboto-user /home/shellboto-user
sudo chmod 0750 /home/shellboto-user
```

Ownership is `root:shellboto-user`, mode `0750`.

**Why this specifically and not `shellboto-user:shellboto-user 0700`?**

If `/home/shellboto-user` is writable by `shellboto-user`, they can
pre-plant a symlink:

```
/home/shellboto-user/$TID → /etc
```

When shellboto creates `/home/shellboto-user/<telegram-id>` and
chowns it to `shellboto-user`, the `chown` follows the symlink and
re-owns `/etc` — root privilege escalation.

shellboto's own code mitigates this via:

- `os.Lstat` (not `os.Stat`) on the target path before creating —
  refuses to proceed if it's a symlink.
- `os.Lchown` (not `os.Chown`) on the created dir — TOCTOU defense;
  even if an attacker plants a symlink between `Lstat` and
  `Lchown`, `Lchown` only modifies the symlink inode, not the target.

But **the parent directory must still not be attacker-writable** —
otherwise the attacker can keep planting symlinks at different paths
and race the manager.

`root:shellboto-user 0750` means:

- Root owns the directory; only root can create entries in it.
- `shellboto-user` is in the group, so they can **traverse into
  their own sub-home** (needed for bash's cwd lookup) but can't
  create/rename entries at this level.

### 3. Verify with a sanity check

```bash
ls -la /home/shellboto-user
# drwxr-x--- 2 root shellboto-user ... /home/shellboto-user
```

Directory owner `root`, group `shellboto-user`, mode `drwxr-x---`.

### 4. Point the bot at it

Edit `/etc/shellboto/config.toml`:

```toml
user_shell_user = "shellboto-user"
# user_shell_home is optional; defaults to /home/shellboto-user
```

Then restart the bot:

```bash
sudo systemctl restart shellboto
```

Check the log for confirmation:

```bash
sudo journalctl -u shellboto -n 30 | grep 'user-role shells resolved'
# …user-role shells resolved  uid=998  gid=998  home=/home/shellboto-user
```

If the config is wrong (user doesn't exist, home dir is writable by
shellboto-user, etc.), shellboto logs a fatal error and exits.

## Verification from a user shell

Have a `role=user` Telegram account send:

```
whoami
```

Reply: `shellboto-user`.

```
ls -la /etc/shadow
```

Reply: `ls: cannot access '/etc/shadow': Permission denied`.

```
apt-get install curl
```

Reply: `E: Could not open lock file /var/lib/dpkg/lock - open (13:
Permission denied)`.

```
cat /root/.ssh/authorized_keys
```

Reply: `cat: /root/.ssh/authorized_keys: Permission denied`.

Exactly what we want.

## Existing shells need a /reset

If a `role=user` had an active shell before you flipped the config,
their shell is still running as whatever it was spawned as. Two
ways to rotate:

- Manually: they run `/reset`; the next command spawns under the
  new settings.
- Automatically: shellboto auto-resets their shell on promote/demote.

## How the `$HOME` ends up

Per-telegram-user:

```
/home/shellboto-user/
├── 111111111/              # alice
├── 222222222/              # bob
└── 333333333/              # charlie
```

Each subdir:

```
drwx------ shellboto-user shellboto-user 0700
```

Created on first shell spawn. Persists across restarts. Accessible
only to that `shellboto-user` uid (and root).

**Caveat:** all `role=user` callers share the same unix identity
(`shellboto-user`). They're isolated by cwd + home, but they're
**not** isolated at the kernel level — one user can read another
user's home:

```
# alice from her shell:
cat /home/shellboto-user/222222222/notes.txt  # works
```

If that's a problem for your threat model, you need per-Telegram-
user unix accounts (not supported; would require operator tooling).
Most deployments don't care — the trust is that any `role=user` is
an honest member of the team who won't snoop on peers.

## `user_shell_home` — custom base directory

If `/home/shellboto-user` isn't where you want these homes (e.g.
you prefer `/var/lib/shellboto-user-homes`), set:

```toml
user_shell_user = "shellboto-user"
user_shell_home = "/var/lib/shellboto-user-homes"
```

Same rules: directory exists, root-owned, mode `0750` with group
matching `user_shell_user`, not writable by `user_shell_user`.

## What happens to SHELLBOTO_* env vars inside these shells

Stripped. `internal/shell/shell.go`'s `sanitizedEnv` helper removes
every `SHELLBOTO_*` variable before `execve`-ing bash.

`printenv | grep SHELLBOTO` in a user shell returns nothing.

## Read next

- [../security/root-shell-implications.md](../security/root-shell-implications.md)
  — blast-radius thinking when admin shells run as root.
- [../shell/user-shells.md](../shell/user-shells.md) — the
  implementation side of this (SpawnOpts, Lstat/Lchown, env sanitisation).
