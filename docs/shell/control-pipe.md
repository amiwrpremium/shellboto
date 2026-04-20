# The fd 3 control pipe

How shellboto knows a command finished without parsing bash's
prompt out of the user-visible output stream.

## Setup

At pty allocation, shellboto creates an OS pipe:

```go
ctrlR, ctrlW, _ := os.Pipe()
cmd.ExtraFiles = []*os.File{ctrlW}    // fd 3 on bash
```

Go's `ExtraFiles[0]` assigns to fd 3 in the child. After `pty.Start`:

- Parent (us) holds the read end (`ctrlR`). We store it as
  `shell.ctrl`.
- Bash (child) has the write end on fd 3. ExtraFiles[0] → fd 3.

Parent closes its copy of `ctrlW` after spawn, so bash sees EOF on
fd 3 only when bash exits.

## Installing the `PROMPT_COMMAND` dispatcher

Right after pty.Start, we write this to bash's stdin:

```bash
exec 100>&3                              # dup fd 3 to fd 100
readonly PROMPT_COMMAND='printf "done:%d\n" "$?" >&100'
```

What this does:

1. `exec 100>&3` — duplicate fd 3 onto fd 100. Now bash has two
   fds pointing at the same pipe. Once we've done this, user
   commands *can* close fd 3 without breaking our channel; fd 100
   still points at the pipe.
2. `readonly PROMPT_COMMAND=...` — bash fires whatever's in
   `PROMPT_COMMAND` every time it's about to display a prompt.
   The `readonly` attribute makes it unchangeable by the user:
   `PROMPT_COMMAND=` returns "readonly variable" without clearing.
3. `printf "done:%d\n" "$?" >&100` — the command body. After each
   user command, bash prints `done:<exit>\n` to fd 100 (which
   goes to fd 3 → our pipe).

## The protocol

shellboto reads line-delimited from the control pipe. Format:

```
done:<integer>\n
```

Where `<integer>` is `$?` from the most recent foreground command.

First `done:N` after setup = "bash is ready." We wait on
`Shell.ready` before serving the first user command; prevents
race between "shell spawned" and "first command dispatched."

Subsequent `done:N` lines = "command finished."

## Why readonly?

Without `readonly`:

```
user> PROMPT_COMMAND=
user> ls      # no done:0 signal emitted
bot waits... and waits... timeout eventually.
```

With `readonly`:

```
user> PROMPT_COMMAND=
bash: PROMPT_COMMAND: readonly variable
user> ls
<output>
done:0                    # emitted as expected
```

## Why fd 100 (not just fd 3)?

If we left user commands with fd 3 exposed, they could:

```
user> exec 3>&-                # close fd 3
user> ls                       # boundary signal lost
```

By duplicating fd 3 → fd 100 before running any user code, we
reserve fd 100 for our use. User commands operate on fd 3 → fd 9
for their own redirections; fd 100 is out of reach for ordinary
usage.

Not bulletproof: `exec 100>&-` from the user still breaks it. But
that's an intentional act — not a stray typo.

## The 30 ms drain window

After `done:N\n` arrives on fd 3, we don't immediately finalise
the Job. The pty (fd 1) and the control pipe (fd 3) are
independent kernel file descriptors; the last few bytes of
command output may still be in flight on the pty when the
control message arrives.

If we finalised instantly:

- Last chunk of `ls`'s output = "total 42\n"
- Appears on pty **after** `done:0\n` appears on fd 3
- Goes into the Job buffer AFTER we've already handed the Job
  off to the audit writer
- Next user command sees those stale bytes as its first output

Fix: 30 ms sleep (`ctrlDrainWindow` in source) before finalising.
Comfortably above same-host kernel scheduling latency.

After the drain, we:

1. Stop accepting new bytes for this Job.
2. Set Job.finishedAt.
3. Close Job.Done.

## What user commands see

From the user's perspective, `ls` works normally:

```
user> ls
foo  bar  baz
user> 
```

They never see `done:0\n` in their output — it went to fd 3, not
fd 1. shellboto's ctrl-reader goroutine consumed it. Their stdout
is clean.

## Edge cases

### `exec bash` (re-exec in place)

User runs `exec bash`. The new bash image starts with fresh env,
fresh shell options. Our `PROMPT_COMMAND` is discarded. No
`done:<N>` is emitted on subsequent commands.

shellboto's readPty keeps collecting output forever. Eventually
`default_timeout` fires; SIGINT, then SIGKILL. User sees
`⚠ timeout`. `/reset` spawns a fresh shell with the dispatcher
reinstalled.

### `exec > /tmp/out`

User redirects stdout to a file. Now `done:N` goes to... wait,
`done:N` goes to fd 100, which is unaffected by stdout
redirection. Still lands on the control pipe.

So this case works. Nice.

### `trap 'exit 0' DEBUG`

User installs a DEBUG trap. Might race with `PROMPT_COMMAND`
depending on ordering. Edge case; if it breaks, `/reset`.

## Reading the code

- `internal/shell/shell.go` — look for `ExtraFiles`,
  `PROMPT_COMMAND`, `ctrlR`, `readCtrl`, `ctrlDrainWindow`.

## Read next

- [output-buffer.md](output-buffer.md) — what happens to the
  pty bytes after readPty accumulates them.
- [signals.md](signals.md) — the other reason we wanted a pty.
