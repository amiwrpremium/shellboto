# Concurrency model

Goroutines, channels, mutexes — what the rules are and why they
matter. Read alongside [runtime-model.md](runtime-model.md) and
[data-flow.md](data-flow.md).

## The rules

1. **Package-level mutability is forbidden** except for init-time
   config. Anything mutated after startup lives on a struct, and
   that struct carries its own `sync.Mutex` / `sync.RWMutex` /
   atomic.
2. **Channel direction is annotated.** Producers write to
   `chan<-`, consumers read from `<-chan`. Makes the data-flow
   direction obvious at call sites.
3. **One writer per channel.** If the design calls for multiple
   producers, they go through a mutex-protected helper that feeds
   the channel.
4. **Close the channel at the producer side.** Consumers never
   close; they just range until the channel drains.
5. **`context.Context` threads through every handler + every
   blocking call.** Cancellation propagates naturally; no
   per-goroutine kill switches.
6. **No unbounded goroutines.** Every `go func(){…}()` has a
   visible lifecycle: tied to a `ctx`, a manager, or a `Job`.

## Goroutine inventory

### Always-on (lives process-long)

| Goroutine | Started in | Exit condition |
|-----------|-----------|----------------|
| Telegram long-poll | `go-telegram/bot` internals (called from `bot.Start`) | ctx cancel |
| Audit pruner | `auditRepo.PruneLoop` | ctx cancel |
| Idle-reap | `shell.Manager.reapLoop` | ctx cancel |
| Supernotify TTL sweeper | `supernotify.Emitter` | ctx cancel |
| Graceful-shutdown trap | `main.go signal.Notify(...)` | any signal fires |

### Per-active-shell (lives between `GetOrSpawn` and `Close`)

| Goroutine | Purpose | Exit condition |
|-----------|---------|----------------|
| `readPty` | Copy pty bytes to active Job buffer | pty EOF / shell close |
| `readCtrl` | Parse `done:<exitcode>` from fd 3 | fd 3 EOF / shell close |

### Per-running-command (lives during streaming)

| Goroutine | Purpose | Exit condition |
|-----------|---------|----------------|
| `stream.Stream` | Edit loop + spill to file | Job.Done closed + final flush returns |

Every per-command goroutine is tied to a `Job`, which has a
buffered `Done chan int` of capacity 1 — producers `select` to
write (fall through if already closed), consumers just `<-j.Done`.

## Mutexes + atomics, by struct

### `shell.Shell`

- **`currentMu sync.Mutex`** — guards the store/swap of the
  current Job and the pty-write operation. `Run` holds this for
  the duration of writing the command to the pty.
- **`current atomic.Pointer[Job]`** — readable without the mutex
  for fast `IsIdle()` checks; writable only while holding the
  mutex.
- **`dead atomic.Bool`** — set once on Close; gate used by
  `CompareAndSwap` so Close is idempotent + race-free.
- **`lastAct atomic.Int64`** — unix nanos timestamp, updated on
  every input/output event for idle-reap accounting.
- **`deathCh chan struct{}`** — closed at end of `Close()`;
  lets callers block on `<-Shell.DeathCh()` for shutdown notice.

### `shell.Job`

- **`mu sync.Mutex`** — guards the bytes buffer + version counter.
- **`done atomic.Bool`** — prevents double-finish.
- **`exitCode, finishedAt atomic.Int64`** — readable without a
  mutex.
- **`termSet atomic.Pointer[string]`** — first-write-wins on the
  termination reason (`canceled`, `killed`, `timeout`,
  `completed`, `truncated`); subsequent writes no-op.
- **`Done chan int`** — buffered capacity 1; `finish()` sends on
  it then closes.

### `shell.Manager`

- **`shells map[int64]*Shell` + `mu sync.Mutex`** — guards the
  map. Lookups, inserts, removals all take the mutex. The mutex
  is NOT held during pty I/O; that happens on the returned
  `*Shell`.

### `db/repo.AuditRepo`

- **`logMu sync.Mutex`** — serialises the read-previous-row →
  compute-hash → insert sequence. Every audit write holds this;
  chain linearity depends on it.

### `telegram/ratelimit.Limiter`

- **`buckets map[int64]*bucket` + `mu sync.Mutex`** — guards the
  map. Each bucket has its own internal atomic counters, so the
  main mutex is only held briefly for the lookup.

### `supernotify.Emitter`

- **`pendingTTL map[int64]time.Time` + `mu sync.Mutex`** — guards
  the map. Sweeper takes the mutex, collects expired entries,
  releases the mutex, then does the EditMessageReplyMarkup calls
  outside the lock (so slow API calls don't stall event fan-out).

## Channel inventory

Small enough to enumerate:

| Channel | Buffered? | Producer → Consumer |
|---------|-----------|---------------------|
| `Job.Done chan int` | 1 | `Job.finish()` → `stream.Stream` |
| `Shell.ready chan struct{}` | unbuf | pty-reader sees first `done:0` → `Run`'s first-use wait |
| `Shell.deathCh chan struct{}` | unbuf | `Shell.Close()` → `Manager.Remove` and shutdown watchers |
| Telegram update chan | buffered | `go-telegram/bot` internals → dispatcher loop |

## Lock ordering

Two-level rule to prevent deadlocks:

1. **Manager-level locks never reach down into Shell-level locks
   while held.** `Manager.Get` takes the map mutex, does the
   lookup, releases, then returns the `*Shell` to the caller.
2. **Shell-level locks never reach up into Manager-level locks
   while held.** If a shell needs to remove itself from the
   manager (on Close), it does so after releasing its own locks.

The only reachable pair is `Shell.currentMu` → `Job.mu` (inside
`Run`, when we build and install the Job while writing the pty).
That pair is always taken in the same order.

`audit.logMu` is independent of the other locks — audit writes
happen outside any shell/manager mutex.

## Cancellation propagation

`main.go` creates one `ctx, cancel := signal.NotifyContext(ctx,
os.Interrupt, syscall.SIGTERM)`. This context is passed to:

- `bot.Start(ctx)` — long-poll uses it.
- `auditRepo.PruneLoop(ctx)`.
- `shellMgr.Start(ctx)` — kicks off the reap goroutine.
- `supernotify.Start(ctx)`.

Every handler receives a derived child context. Blocking calls
(DB, Bot API, pty waits with explicit timeouts) take it.

Shutdown sequence:

```
systemd sends SIGTERM
        │
        ▼
signal.Notify trips           ← cancel() fires
        │
        ├── long-poll returns (after current getUpdates)
        ├── pruner sees ctx.Done(), exits
        ├── reap loop sees ctx.Done(), exits
        └── supernotify sees ctx.Done(), exits
                │
                ▼
main.go: shellMgr.CloseAll()   ← serially close every live shell
                │
                ▼
         audit kind=shutdown written
                │
                ▼
         db.Close(gormDB), process exits
```

systemd gives us up to `TimeoutStopSec=90s` (systemd default);
shutdown normally completes in under a second.

## Testing concurrency

- **No time.Sleep in tests.** Tests use channels + timeouts
  (`t.Fatal` if a `select` hits its `<-time.After` branch).
- **Real pty, real bash.** `shell_test.go` does actual fork+exec,
  not mocks. Concurrency bugs in our code show up deterministically.
- **Real SQLite.** `newTestRepo(t)` uses `t.TempDir()` for an
  isolated database. No mocked transactions; we'd miss real race
  bugs otherwise.
- **Race detector in CI.** `go test -race` runs on every PR.

## Known subtle points

### The 30 ms drain window

After bash writes `done:<exit>\n` on fd 3, the reader goroutine
doesn't immediately finalise the Job — the pty and fd 3 are
independent file descriptors, and the last few bytes of command
output may still be in flight through the kernel. If we finalise
too fast, those trailing bytes land in the next command's buffer.

Fix: 30 ms sleep before finalising. That's comfortably above same-
host kernel scheduling latency.

### Idle-reap + Close race

If a user sends a command at exactly the moment idle-reap is
closing their shell, `GetOrSpawn` could get a `*Shell` pointer
that's one instruction away from being torn down.

Mitigation:
1. Reap takes the map mutex, removes the entry, releases.
2. `Close()` uses `CompareAndSwap` on `dead` so if a new
   command is racing in, it notices and returns an error.
3. The caller retries `GetOrSpawn`, which now sees no entry and
   spawns fresh.

### The `output_sha256` is computed once

Audit blob's SHA-256 is part of the canonical hash. Computed at
the moment of write; stored in a column so `Verify` doesn't
re-decompress to recompute. Legacy rows (pre-column) fall back to
decompress-and-hash; the migration left old rows intact.

### Stream restart after bot restart

If shellboto restarts mid-command, the streamer goroutine dies
with the process. The in-flight bash process on a pty is killed
by systemd because systemd's cgroup contains it — so there's no
orphan to reconcile. The user sees the half-edited message in
Telegram until they run another command; the bot doesn't reattach
to "resume" the edit (Telegram doesn't have that concept).

Read next: [design-decisions.md](design-decisions.md).
