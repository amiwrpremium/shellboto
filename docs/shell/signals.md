# Signals — `/cancel` and `/kill`

How the bot sends interrupts to the running command.

## `/cancel` → SIGINT (Ctrl+C)

Implementation (from `shell.go`):

```go
func (s *Shell) SigInt() error {
    s.lastAct.Store(time.Now().UnixNano())
    _, err := s.pty.Write([]byte{0x03})
    return err
}
```

That's it — write `\x03` (byte 3, Ctrl+C) to the pty master. The
pty's line discipline handles the rest:

1. Kernel sees the special character on the pty.
2. Sends SIGINT to the **foreground process group** of the tty.
3. Foreground process decides what to do (default: die;
   `trap 'echo caught' INT` intercepts).

Same semantics as typing Ctrl+C in a real terminal.

If the foreground process ignores SIGINT (e.g. `while true; do :;
done` in bash), the signal is delivered but has no effect.
`/cancel` returns success regardless — it sent the signal; what
the process does with it is its business.

## `/kill` → SIGKILL

More aggressive: SIGKILL cannot be trapped or ignored.

Implementation:

```go
func (s *Shell) SigKill() error {
    s.lastAct.Store(time.Now().UnixNano())
    fg, err := unix.IoctlGetInt(int(s.pty.Fd()), unix.TIOCGPGRP)
    if err != nil {
        return err
    }
    if fg == s.cmd.Process.Pid {
        return errors.New("no foreground child to kill")
    }
    return syscall.Kill(-fg, syscall.SIGKILL)
}
```

Steps:

1. `ioctl(TIOCGPGRP)` on the pty master → returns the foreground
   process group ID.
2. If the foreground is **bash itself** (no child running), refuse
   — killing bash would leave the shell headless; `/reset` is the
   correct tool for that.
3. Otherwise, `kill(-fg, SIGKILL)` — the negative pgid sends to
   the whole group.

SIGKILL propagates to all processes in the group. Children of the
foreground command (if the command forked) die too.

## Why process **groups**, not single PIDs?

Commands often fork:

```
find / -exec grep -l 'x' {} \;
```

`find` itself is the foreground process, but `grep` is spawned
per-match. Both are in the same process group. SIGKILL to the
group kills all of them; SIGKILL to just `find`'s pid would leave
orphaned `grep`s.

## Refusing to kill bash

If the user `/kill`s when no command is running:

```
fg == s.cmd.Process.Pid
```

(bash is its own foreground process group in an idle pty.)
`SigKill` returns "no foreground child to kill" and the bot
replies with that.

If the user wants to kill bash (maybe it's wedged):

```
/reset
```

`Shell.Close()` sends SIGTERM to bash's pgid, waits 2s, then
SIGKILL. The shell is replaced with a fresh one on the next
message.

## Timeout → SIGINT → (kill_grace) → SIGKILL

When `default_timeout` elapses without a command finish:

1. SIGINT sent (same mechanism as `/cancel`).
2. Wait `kill_grace` (default 5s).
3. If the Job still isn't finished: SIGKILL sent (same mechanism
   as `/kill`).

Implementation lives in the per-Job watchdog spawned when the
Job starts.

## What the user sees

`/cancel`:

```
(streaming output, then interrupted)
^C
bash: ...
⚠  interrupted · 3.2s
```

`/kill`:

```
(streaming output, then abruptly)
⚠  killed · 8.1s
```

`default_timeout` elapsed:

```
(streaming output, then)
⚠  timeout · 5m 2s
```

`max_output_bytes` cap hit:

```
(big pile of output, up to the cap)
⚠  output capped · 12s
```

## Audit termination field

Every completed Job writes `audit_events.termination`:

- `completed` — exit happened naturally.
- `canceled` — `/cancel` sent SIGINT and the process died.
- `killed` — `/kill` sent SIGKILL.
- `timeout` — `default_timeout` fired; SIGINT→SIGKILL escalation.
- `truncated` — `max_output_bytes` overflow → auto-SIGKILL.

First-write-wins (`termSet atomic.Pointer[string]`) — so if a user
spams `/cancel` followed by `/kill` 100 ms apart, only the first
wins. Prevents confusing audit records like `killed` when the
process had already exited on SIGINT.

## Can a user refuse to die?

Yes, briefly:

- SIGINT can be trapped (`trap 'echo no' INT`). shellboto's
  `/cancel` delivers it; the process ignores it.
- SIGKILL **cannot** be trapped. It's terminal. If a process
  survives SIGKILL, you've got kernel issues (process stuck in
  uninterruptible sleep — usually a bad storage I/O).

For "process stuck in D state" scenarios: SIGKILL is queued but
doesn't fire until the I/O completes. shellboto will look like
`/kill` hung. Eventually resolves when the kernel unblocks the
syscall.

## What about signal masks?

User commands can `trap "" TERM`, `trap "" INT` — ignore specific
signals. We don't reset the signal mask between commands (bash's
`PROMPT_COMMAND` doesn't).

If a user traps everything:

```
trap "" ALL      # not a real syntax but close enough
```

They've jammed their own shell. `/cancel` / `/kill` still fire;
SIGKILL still kills. `/reset` always recovers since `Shell.Close`
sends SIGTERM to bash's pgid (which bash has no trap for by
default).

## Reading the code

- `internal/shell/shell.go:Shell.SigInt`
- `internal/shell/shell.go:Shell.SigKill`
- `internal/shell/shell.go:Shell.Close` — the full teardown
  sequence.

## Read next

- [user-shells.md](user-shells.md) — the credential-drop story
  for non-admin shells.
- [../telegram/commands.md](../telegram/commands.md) — user-facing
  `/cancel` + `/kill`.
