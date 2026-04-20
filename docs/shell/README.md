# Shell

How the per-user pty-backed bash works. This is the trickiest part
of the code.

| File | What it covers |
|------|----------------|
| [pty-vs-exec.md](pty-vs-exec.md) | Why pty; what that buys us; what won't render |
| [control-pipe.md](control-pipe.md) | fd 3 + PROMPT_COMMAND + `done:<N>` protocol |
| [output-buffer.md](output-buffer.md) | Byte caps + auto-SIGKILL on overflow |
| [signals.md](signals.md) | `/cancel` SIGINT, `/kill` SIGKILL, process groups |
| [user-shells.md](user-shells.md) | Dropping privs, `SpawnOpts`, Lstat/Lchown, env sanitisation |

## Mental model

```
one Go goroutine set per user
    │
    ▼
  fork + pty allocation (creack/pty)
    │
    ▼
  bash process, stdin/stdout/stderr on pty, fd 3 on control pipe
    │
    │  ← byte stream on pty (stdout + stderr interleaved)
    │  ← boundary signal on fd 3: "done:<exit>\n"
    ▼
  ctrl-reader goroutine → finalises Job
  pty-reader goroutine  → appends bytes to Job buffer
```

The pty is allocated once per user, persists across messages. The
bash process inherits it. Each command = write bash input, wait
for boundary signal, collect bytes.

## Why this shape

- **State persistence.** `cd`, env vars, aliases, bash history —
  all per-user.
- **Boundary clarity.** User can't see our control messages;
  bash can't close fd 3; the two channels don't interleave.
- **Signal-correct.** `/cancel` and `/kill` send real signals via
  the pty's tty line discipline (for SIGINT) or ioctl-derived pgid
  (for SIGKILL) — not polite `cancel hints` to a wrapper.

## Read next

- [pty-vs-exec.md](pty-vs-exec.md) — the foundational choice.
- [control-pipe.md](control-pipe.md) — the clever bit.
