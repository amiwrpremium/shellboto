# Commands never complete

The bot replies "running..." and never gets to an exit code.

## Common causes

### 1. `exec bash` (or `exec sh`) wedged your shell

You ran `exec bash`. The new bash image doesn't have our
`PROMPT_COMMAND` dispatcher. Boundary detection silently broke.

**Fix:** `/reset`. Spawns a fresh shell with the dispatcher
reinstalled. Lose `cd` / env state.

You'll see this same symptom from `unset PROMPT_COMMAND` — but
PROMPT_COMMAND is `readonly`, so unset returns an error and the
dispatcher survives. `exec bash` is the way to break it.

See [../shell/control-pipe.md](../shell/control-pipe.md).

### 2. Ran a TUI program

`vim`, `top`, `less`, `htop` — they wait for keystrokes that
never arrive. The bot's per-command timeout (default 5m)
eventually reaps.

**Fix:** `/kill` or `/reset`. Use non-interactive equivalents
next time:

| Want | Use instead |
|------|-------------|
| `vim file` | `cat file`, edit elsewhere |
| `nano file` | upload/download via /get + paperclip |
| `less file` | `cat file`, `head -200 file`, `tail -200 file` |
| `top` | `top -b -n 1 \| head -30` |
| `htop` | `htop -n 1 \| head -30` |
| `man cmd` | `cmd --help`, `man --pager=cat cmd` |

### 3. Background process eating the next command's output

You ran `./long-task &` then a normal command. Background
process keeps writing to the pty; subsequent command's output
gets interleaved.

This is technically working as designed. Bash backgrounded jobs
are visible in the same pty. **Mitigation:** redirect background
output explicitly:

```
./long-task > /tmp/long.log 2>&1 &
```

### 4. Command runs longer than `default_timeout`

Default 5m. If your command takes longer (build, big migration),
SIGINT fires at 5m, then SIGKILL at 5m + 5s.

**Fix:**
- Raise `default_timeout` in config + restart.
- Or run inside `nohup ... &` to detach (works but disconnects
  from the streaming).

### 5. Command waiting on stdin

`cat` (no args), `read line`, etc. They block waiting for input
that the bot doesn't provide.

**Fix:** never run a command that reads stdin. Pipe into it:

```
echo "hello" | cat        # works
cat                       # hangs
```

### 6. Network call hanging

`curl http://unreachable` with no `--max-time`, `nslookup
broken.example`, etc. The kernel waits the full default timeout
(can be minutes).

**Fix:** always set explicit timeouts on network commands:

```
curl --max-time 10 http://example.com
nslookup -timeout=5 host.example.com
```

### 7. Ran shellboto inside its own shell (recursion)

You typed `shellboto doctor` from within a bot pty. shellboto
tries to acquire the instance flock; service holds it; doctor
waits or errors.

**Fix:** don't shell-out shellboto from inside its own shell.
Run CLI subcommands from a regular SSH session.

## Diagnose from the host

```bash
# Find the user's shell process
sudo ps -ef --forest | grep -A 5 shellboto

# What's its foreground pid?
sudo ps -o pgid= -p <bash_pid>

# Look for D-state (uninterruptible sleep)
sudo ps -o pid,stat,cmd -e | awk '$2 ~ /D/'
```

D state = stuck I/O. Usually disk-related; SIGKILL queues but
won't fire until the syscall returns.

## Last resort

If nothing else works, restart the service:

```bash
sudo systemctl restart shellboto
```

Heavy hammer; affects everyone.

## Read next

- [../runbooks/shell-stuck.md](../runbooks/shell-stuck.md) — the
  runbook version.
- [../shell/pty-vs-exec.md](../shell/pty-vs-exec.md) — why
  TUI programs don't work.
