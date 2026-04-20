# Rollback — `deploy/rollback.sh`

Swap between the current and previous binaries.

Source: [`deploy/rollback.sh`](../../deploy/rollback.sh).

## How the `.prev` gets there

On every `./deploy/install.sh` invocation that installs a new
binary, the old binary is saved:

```
Before install:
  /usr/local/bin/shellboto          ← v1.2.0

After install:
  /usr/local/bin/shellboto          ← v1.3.0 (new)
  /usr/local/bin/shellboto.prev     ← v1.2.0 (saved)
```

So `shellboto.prev` is always "whatever was installed just before
the most recent install."

## Usage

```bash
sudo ./deploy/rollback.sh
```

What it does:

1. Stop the service.
2. Atomically swap `/usr/local/bin/shellboto` ↔
   `/usr/local/bin/shellboto.prev` via `mv` (via a temporary
   intermediate name so neither file is ever missing).
3. Start the service.
4. Print the rollback summary.

Reversible: re-run `rollback.sh` to flip back to the newer
version.

## Flags

| Flag | Effect |
|------|--------|
| `-y`, `--yes` | No confirmation prompt. |
| `--prefix DIR` | Operate on `<dir>/usr/local/bin/…`. |
| `--dry-run` | Print actions, change nothing. |
| `-h`, `--help` | Usage. |

## When to use

- **Bad release.** New version has a regression; roll back while
  you fix forward.
- **Canary.** Install a pre-release, `/reset` everyone's shells,
  confirm it works. If it doesn't, `rollback.sh`.
- **Testing upgrade path.** `install.sh` then `rollback.sh` then
  `install.sh` again is safe; atomic swap every step.

## What's NOT a rollback target

- **Config changes.** `rollback.sh` only swaps binaries. If you
  changed `/etc/shellboto/config.toml` and want to revert, you
  need your own VCS / backup. We recommend versioning
  `/etc/shellboto/config.toml` in a private git repo as a common
  practice.
- **DB state.** Audit history + user table are not versioned by
  the rollback. Use [backup.md](../database/backup.md) for that.
- **systemd unit file.** Not versioned; if a release changed the
  unit, the new unit stays after rollback. If you need the old
  unit: check it in to your own config mgmt, or keep a copy.

## When there's no `.prev`

Fresh install or you've already rolled back twice. `rollback.sh`
refuses:

```
❌ nothing to roll back to.
   /usr/local/bin/shellboto.prev does not exist.
   Did you roll back already?
```

The previous previous isn't preserved — we only keep one
generation.

## Interaction with systemd

`rollback.sh` stops the service before the swap, starts it after.
Between the stop and start, the bot is offline; users trying to
send commands see a "bot didn't reply" until the start completes
(typically < 5 seconds).

systemd's `Restart=on-failure` doesn't fire because rollback stops
the service cleanly.

## After rollback

- `systemctl status shellboto` — active (running).
- `shellboto --version` — shows the previous version's git sha.
- `shellboto doctor` — green.
- **Active user shells are all closed.** The service restart
  tears down the pty manager; next message from each user spawns
  a fresh shell on the rolled-back binary.
- **Audit chain intact.** The DB is untouched. `audit verify`
  passes.
- **Release PR / tag situation.** Tag is still on GitHub. Consider
  marking the bad GitHub release as "pre-release" so new
  operators don't pick it up; the fix-forward release then
  supersedes it.

## Release-please / tag flow

Rolled-back binary comes from the `.prev` saved during the last
`install.sh`. If the current production binary came from a
release-please PR merge → tag push → goreleaser publish, the
rolled-back binary is whatever was there before that cycle.

Rolling back the Git tag or un-releasing from release-please is a
separate story; `rollback.sh` only touches the binary on the
host.

## Reading the code

- `deploy/rollback.sh`

## Read next

- [../runbooks/bad-release.md](../runbooks/bad-release.md) — the
  canonical use.
- [installer.md](installer.md) — the other half (how `.prev`
  gets created).
