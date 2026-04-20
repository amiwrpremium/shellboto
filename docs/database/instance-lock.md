# Instance lock

shellboto enforces exactly-one-instance via an `flock` on
`/var/lib/shellboto/shellboto.lock`.

## Why

Two shellboto processes against the same DB would:

- Race on audit-chain writes. `audit.Log`'s mutex is **per-process**
  — two processes can each hold their own instance of the mutex
  simultaneously. Both read the same "previous row," both
  compute a next hash, both insert. The chain forks.
- Duplicate handler dispatch. Same Telegram update processed
  twice.
- Corrupt Job state in surprising ways (each process manages its
  own shell map; a user's shells would split across processes).

SQLite's own `busy_timeout` saves us from hard data corruption,
but application-level invariants fail.

## How

At startup (`main.go`), right after opening the DB:

```go
lockPath := filepath.Join(stateDir, "shellboto.lock")
f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0600)
if err != nil {
    logger.Fatal("instance lock open", zap.Error(err))
}
if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
    logger.Fatal("another shellboto is running?", zap.Error(err))
}
// Keep f alive for the lifetime of the process.
```

`LOCK_EX|LOCK_NB`:

- **LOCK_EX** — exclusive. Only one holder.
- **LOCK_NB** — non-blocking. If already held by another process,
  fail immediately with EAGAIN/EWOULDBLOCK.

## Release

The kernel releases the flock automatically when the holding
process exits (clean shutdown or crash). No "stuck lockfile"
scenario.

If you manually delete the lockfile while shellboto is running,
the kernel-level lock is held on the file's *inode*, not its
path. Deleting the path doesn't release the lock until the
holding process exits.

## Implications

- **You cannot run `shellboto` with the service already up.** CLI
  commands that want the DB (e.g. `audit verify`) don't take the
  lock — they're read-only-via-WAL, so they work.
- **`shellboto db vacuum` DOES require the service to be stopped.**
  It needs an exclusive DB connection. See [vacuum.md](vacuum.md).
- **Two `shellboto doctor` calls at once = fine.** Neither takes
  the lock.

## Can I bypass for dev?

You can set `SHELLBOTO_DB_PATH` (or use `-config`) to point at a
different DB file — its own separate lockfile. Useful for local
testing:

```bash
sudo ./bin/shellboto -config /tmp/test-config.toml
```

Where the test config points at `/tmp/test-state.db`. You can run
this alongside the production instance; different lockfiles,
different DBs.

## Error messages

If a second shellboto tries to start:

```
{"level":"fatal","msg":"instance lock","error":"resource temporarily unavailable"}
```

Or via `doctor`:

```
❌  instance lock: /var/lib/shellboto/shellboto.lock is held
```

Find the offending PID:

```bash
sudo lsof /var/lib/shellboto/shellboto.lock
```

Kill it if it's stale (shouldn't be — kernel auto-releases):

```bash
sudo kill -TERM <pid>
```

## Lockfile contents

Empty file. 0 bytes. Only its inode matters. shellboto doesn't
write PID / hostname into it; you can't use its contents to
diagnose who's holding.

Use `lsof` or `fuser`:

```bash
sudo fuser /var/lib/shellboto/shellboto.lock
```

## Reading the code

- `internal/db/lock.go:AcquireInstanceLock`
- `cmd/shellboto/main.go` — where it's called at startup.

## Read next

- [vacuum.md](vacuum.md) — the other command that needs
  exclusive access.
