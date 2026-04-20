# Pty vs. one-shot exec

shellboto uses a pty-backed persistent bash per user, not a
per-command `os/exec`. This doc explains the choice and its
consequences.

## What a pty is

A pseudo-terminal: a pair of file descriptors (master/slave) that
behaves to the child process like a real terminal, with line
discipline, window-size ioctls, job control, everything.

`creack/pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})`
allocates one; bash's stdin/stdout/stderr are wired to the slave
side. The parent (us) reads from the master.

## Why not just `os/exec`

```go
cmd := exec.Command("bash", "-c", userInput)
output, err := cmd.CombinedOutput()
// done. No state carried to next command.
```

Problems:

- **`cd` doesn't persist.** Next `cmd.Run()` starts at `$HOME`.
  To mimic state, you'd threadpool + thread through env + cwd +
  shell options + alias map per user. Lots of edges.
- **No job control.** `sleep 10 &` detaches, but you have no
  pid-group handle to send signals into.
- **No line discipline.** `less` / `vim` / `top` expect a tty;
  without one they either refuse to start or misbehave silently.
- **No `PROMPT_COMMAND` hook.** Can't install a boundary-signal
  dispatcher on a one-shot bash.

## What the pty gives us

- **Stateful shell.** Everything a user types persists until the
  pty is closed: cwd, env vars, exported functions, aliases, bash
  history, shell options (`shopt`), job control.
- **Signal-correct interrupts.** `/cancel` writes `\x03` (Ctrl+C)
  to the master; line discipline forwards SIGINT to the
  foreground process group. Same thing that happens when you
  type Ctrl+C in a real terminal.
- **Window size control.** We allocate at 40×120; some commands
  honour the reported size for output formatting (`column -t`,
  `ls` when output goes to a tty, etc.).
- **Job control.** `bg`, `fg`, `jobs` all work. Background
  processes persist; their stdout gets interleaved with future
  commands (documented limitation).

## The trade-offs (downsides)

### Can't run interactive TUIs

`vim`, `nano`, `less`, `top`, `htop`, `man` — these programs
assume:

- The terminal has a cursor you can address.
- You can clear the screen.
- Keyboard input is delivered key-by-key.

Telegram has none of that. Each "message" is a blob sent on Enter.
There's no key-by-key input channel and no cursor position
reports. These programs will either:

- Refuse to start (`less` checks `isatty`, then falls through).
- Emit cursor-addressing codes that we strip (so the user sees
  garbage).
- Wait for input forever (`vim` in insert mode).

Workarounds exist for each: `cat`, `tail`, `ps auxf`, `htop -n 1`,
`man --pager=cat`.

### Boundary detection is fragile

Running the pty with bash means we need to know when a command
is "done" (so we can finalise the streaming message + audit
write). Options:

1. **Parse the prompt.** Fragile — user's PS1 could be anything.
2. **Use `set -e` and non-zero exit.** Doesn't work — bash doesn't
   signal "done" on a successful exit either.
3. **Install a `PROMPT_COMMAND` hook that writes to fd 3.** What
   we do. See [control-pipe.md](control-pipe.md).

Option 3 is the clever bit but has its own edge cases. If the
user `exec bash` or `PROMPT_COMMAND=''`, the dispatcher dies and
boundary detection silently stops working. The per-command
timeout eventually reaps; `/reset` recovers instantly.

### Idle shells consume resources

Each pty:

- ~40 KB kernel memory for the pty struct.
- A bash process (RSS ~3–5 MB).
- Two goroutines (reader + ctrl reader), a few KB each.
- An entry in the manager's `shells` map.

For a single user = negligible. For 100 idle users = 0.5 GB. That's
why `idle_reap` (default 1h) exists: per-user shells go away when
the user stops typing.

### Bash state is `kill -9`-able

Users can do `exec bash` (re-exec in place), `kill -9 $$` (kill
self), `exit` (clean close) — all of these leave shellboto with a
dead pty on its hands. The readPty goroutine sees EOF, calls
`Shell.Close()`, the next user message spawns a new pty.

Visible to the user as `⚠ shell died` on the message they sent
that was interrupted by the kill. Just retry.

## What we do NOT do

- **No `script(1)` wrapper.** Would add dep + a subprocess for no
  gain — we control the pty directly.
- **No terminal emulator** (alacritty, kitty protocol) — we want
  raw bytes, no rendering.
- **No SSH tunnelling.** The pty is local to the shellboto
  process.

## Reading the code

- `internal/shell/shell.go:Shell` — the struct.
- `shell.go:Shell.readPty` — bytes → Job buffer.
- `shell.go:Shell.readCtrl` — control pipe → boundary signals.
- `shell.go:Manager.GetOrSpawn` — lookup + pty allocation.

## Read next

- [control-pipe.md](control-pipe.md) — how we tell "done" from
  "still outputting."
- [signals.md](signals.md) — `/cancel` + `/kill` mechanics.
