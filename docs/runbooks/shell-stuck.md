# A user's shell is stuck

**Symptom**: a user reports their commands never finish. `/status`
says "running" forever. `/cancel` and `/kill` "didn't help" /
"didn't seem to do anything."

## Quick checks the user can do

Tell them to:

1. **`/kill`** — `/cancel` may have failed if the foreground
   process traps SIGINT (`trap '' INT` in their shell). `/kill`
   is SIGKILL; can't be ignored except by stuck I/O.
2. **`/reset`** — closes their pty. Spawns a fresh one on next
   message. Loses cwd, env, aliases — they lose state but get a
   working shell.

## What to check on the host

```bash
# Find the user's shellboto-spawned bash process.
sudo ps -ef --forest | grep -A 5 shellboto
```

Look for a `bash` process under shellboto's pid with a child that
might be stuck. Common culprits:

- **`exec bash`** — they ran this in their shell. The new bash
  doesn't have our `PROMPT_COMMAND`; boundary detection broke.
  `default_timeout` (default 5m) eventually cleans up. `/reset`
  recovers immediately. See [../shell/control-pipe.md](../shell/control-pipe.md).
- **Stuck I/O** (`D` state in `ps`). Process is in
  uninterruptible sleep waiting on disk. SIGKILL is queued but
  doesn't fire until the syscall returns. Reboot might be the
  only fix.
- **Vim / nano / less waiting for input.** They expected input
  via Telegram messages. That's not how it works. `/kill` should
  end them.
- **Background job piped to foreground** (`./long-task &`). The
  background job is fine, but they can't tell. `/status` reports
  the foreground state.

## Force-kill from the host

If `/kill` from Telegram isn't working:

```bash
# Find the bash group leader for the user's shell.
sudo pgrep -f 'bash' -a       # find the right one by parent PID

# Kill the whole process group.
sudo kill -KILL -<pgid>
```

The bot's pty reader sees EOF, closes the shell, audits
`shell_reaped` with detail noting the unclean close.

## Manager-level reset

Restart the bot. Any active shell dies cleanly:

```bash
sudo systemctl restart shellboto
```

Heavy hammer. Affects every user, not just the stuck one.

## If many users are stuck simultaneously

Suggests a host-level issue: disk full, kernel hung, network DNS
failure causing every command to hang on a name lookup.

```bash
sudo systemctl status shellboto
df -h
dmesg | tail -50
```

## Long-term mitigations

- **Educate users on `exec bash`.** Tell them not to. Document
  in your team's runbook that `/reset` is the recovery.
- **Tighten `default_timeout`.** From `5m` to `2m` if your team
  runs short commands and patience is thin. Catches stuck shells
  faster.
- **Tighten `idle_reap`.** Closes shells that go quiet —
  including stuck ones — earlier. Trade-off: reduces session
  state preservation.

## Read next

- [../shell/pty-vs-exec.md](../shell/pty-vs-exec.md) — why some
  things will always be hard.
- [../shell/signals.md](../shell/signals.md) — what `/cancel` and
  `/kill` actually do.
