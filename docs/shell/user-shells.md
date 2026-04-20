# User shells — dropping privileges

The implementation side of [../configuration/non-root-shells.md](../configuration/non-root-shells.md).

## SpawnOpts

```go
type SpawnOpts struct {
    Creds *syscall.Credential  // non-nil = drop to this uid/gid/groups
    Dir   string               // working directory for bash
    Env   []string             // extra env on top of sanitised base
}
```

A zero-value `SpawnOpts{}` means "spawn bash as the current
shellboto process (root) in the default cwd with the default env"
— that's what admin+ shells get.

A populated `SpawnOpts` with `Creds` set means "drop to this
user" — that's what role=user shells get when `user_shell_user`
is configured.

## How Creds lands on the fork

`creack/pty.StartWithSize` ends up calling `os/exec.Cmd.Start`
after wiring the pty. Before start, shellboto sets:

```go
cmd.SysProcAttr = &syscall.SysProcAttr{
    Credential: opts.Creds,  // uid, gid, groups
    Setctty:    true,         // this pty is the controlling tty
    Setsid:     true,          // start a new session
}
cmd.Dir = opts.Dir
cmd.Env  = sanitizedEnv(opts.Env...)  // merge user env after strip
```

The kernel applies credentials at execve time. bash starts with
the new uid/gid and can't re-elevate.

## The per-user home directory

For each role=user caller, the handler calls `ensureUserHome`:

```go
dir := filepath.Join(userShellHome, strconv.FormatInt(telegramID, 10))
```

E.g. `/home/shellboto-user/987654321`.

`ensureUserHome`:

1. `os.Lstat(dir)`:
   - Error (not found) → `os.MkdirAll(dir, 0700)` → `os.Lchown(dir, uid, gid)`.
   - Found and is a regular dir → proceed.
   - Found and is a symlink → refuse with error.
   - Found and is something else → refuse with error.
2. Return dir as Dir for SpawnOpts.

## Why Lstat + Lchown?

**The symlink attack:**

1. The `shellboto-user` unix user has write access to
   `/home/shellboto-user/` (bad setup).
2. Attacker-as-shellboto-user creates
   `/home/shellboto-user/987654321 → /etc`.
3. shellboto spawns role=user shell for telegram id 987654321.
4. `ensureUserHome` calls `os.Stat` (follows symlinks) → sees a
   real dir. Proceeds.
5. `os.Chown(dir, uid, gid)` — follows the symlink — re-owns `/etc`
   to `shellboto-user`.
6. Attacker now owns /etc; profit.

**Defense in depth:**

- `os.Lstat` doesn't follow symlinks; sees the link and refuses.
- Even if a TOCTOU race plants the symlink between `Lstat` and
  `Lchown`, `Lchown` modifies the symlink itself (not the target)
  — no privilege escalation.
- Plus: the parent dir (`/home/shellboto-user`) should be root-
  owned 0750 so shellboto-user can't plant symlinks there in the
  first place. That's documented setup; `ensureUserHome` is the
  belt.

The code comments in `internal/shell/shell.go` spell this out.

## Environment sanitisation

`sanitizedEnv` runs before execve. It builds a bash environment
consisting of:

```
HOME=/home/shellboto-user/987654321   (or cwd = Dir)
USER=shellboto-user
LOGNAME=shellboto-user
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
PS1=  (bash's default)
PS2=  (continuation)
```

Explicitly NOT inherited:

- `SHELLBOTO_TOKEN`, `SHELLBOTO_SUPERADMIN_ID`, `SHELLBOTO_AUDIT_SEED`
  — stripped. No `SHELLBOTO_*` var reaches bash.
- Most other parent-process env vars — most of them are systemd
  service overrides anyway; we pick what's needed explicitly.

A user-role caller running `env | grep SHELLBOTO` gets nothing.
Running `printenv` gets the sanitised set.

## What bash does with these

- **Login-shell behaviour.** bash is invoked as `/bin/bash -i`
  (interactive). It reads `.bashrc` from `HOME` if it exists.
- **PATH** — standard Linux path; no `.` or PWD on it (no relative
  lookups).
- **HOME** — per-telegram-id dir. Files the user creates land
  there.

If the user has customised their `.bashrc` via a previous shell
session, it lives at `/home/shellboto-user/<tid>/.bashrc` and runs
on the next shell. Persistence of user customisations is a
deliberate feature.

## What `Creds` cannot do

`syscall.Credential` sets uid/gid/groups. It does not:

- Set namespaces (pid, net, mount, user). shellboto doesn't use
  namespace isolation.
- Apply cgroup limits. systemd's `TasksMax=500` caps fork count
  at the service level; within that, a user-role shell can
  spawn as many processes as the uid is allowed.
- Apply seccomp filters. No syscall denylist.

If you need stronger isolation than "unprivileged uid with a home
dir," use a container per user (not a shellboto feature; bring
your own).

## Why we don't spawn a container per command

Considered and rejected:

- **Latency.** Spinning up a container per command = 1–5 seconds
  just in container start. Interactive shell UX dies.
- **State.** Stateful shells are half the value of shellboto
  (see [pty-vs-exec.md](pty-vs-exec.md)). Containers break
  persistence unless you reattach, which is its own mess.
- **Complexity.** Pulling in Docker / podman / systemd-nspawn as a
  runtime dep triples the ops surface. shellboto is a single
  static binary; we keep that.

If "root shell for admins, per-user unix account for users" isn't
enough isolation, shellboto isn't the right tool — use a proper
sandboxed interactive-shell platform.

## Reading the code

- `internal/shell/shell.go:SpawnOpts`
- `internal/shell/shell.go:sanitizedEnv`
- `internal/telegram/commands/exec.go:ShellOptsFor` — the
  role-based SpawnOpts builder.
- `internal/telegram/commands/exec.go:ensureUserHome` — Lstat /
  Lchown logic.

## Read next

- [../configuration/non-root-shells.md](../configuration/non-root-shells.md)
  — the operator-facing setup doc.
- [../security/root-shell-implications.md](../security/root-shell-implications.md)
  — the threat-model lens on admin-is-root.
