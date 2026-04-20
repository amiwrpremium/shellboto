# Runtime model

How the process is organised at runtime. What starts up, in what
order. Who's running when nothing's happening. What happens on
shutdown.

## One process, long-lived

Started by systemd (`shellboto.service`) as PID 1-under-the-unit.
`Type=simple`, no fork. Runs until it crashes or systemd stops it.

`main.go` does the startup sequence:

1. **Parse flags + env.** `-config <path>` plus env vars
   `SHELLBOTO_TOKEN`, `SHELLBOTO_SUPERADMIN_ID`, `SHELLBOTO_AUDIT_SEED`.
2. **Load + validate config.** `config.Load()`. Fatal on malformed
   config, unreadable file, or failed validation (see
   [../configuration/schema.md](../configuration/schema.md)).
3. **Construct logger.** `logging.New(cfg)`. A derived
   `logger.Named("audit")` is set aside for the audit mirror.
4. **Resolve audit seed.** Parse `SHELLBOTO_AUDIT_SEED` hex; if
   missing, fall back to all-zeros with a loud warning.
5. **Resolve `user_shell_user`.** If set, `user.Lookup()` → cache
   the uid/gid/groups. If empty, log a dev-mode warning
   ("user-role shells will run as root").
6. **Take the instance lock.** `flock(LOCK_EX|LOCK_NB)` on
   `/var/lib/shellboto/shellboto.lock`. Kernel releases on process
   exit. If another shellboto is already running, exit 1.
7. **Open the DB.** `db.Open(cfg.DBPath)` — GORM + modernc SQLite,
   chmod 0600 on the file.
8. **Run migrations.** `db.AutoMigrate()` — additive only (see
   [../database/migrations.md](../database/migrations.md)).
9. **Construct audit repo.** `repo.NewAuditRepo(gormDB, seed,
   auditJournal, mode, maxBlob)`.
10. **Start the audit pruner goroutine.** Ticks hourly, deletes rows
    older than `audit_retention`, wrapped in `recover()` so a
    driver panic doesn't tear down the process.
11. **Seed the superadmin.** `userRepo.SeedSuperadmin(cfg.SuperadminID)`
    — idempotent: creates/upserts the row, demotes any other
    superadmin to admin.
12. **Build dep bundle.** `deps.New(cfg, logger, auditRepo, userRepo,
    shellMgr, streamer, danger, …)`.
13. **Construct shell manager.** `shell.NewManager(cfg, auditRepo, …)`.
    Starts the idle-reap goroutine on a 1-minute ticker.
14. **Start the Telegram bot.** `telegram.New(cfg.Token, deps)` →
    registers handlers → `bot.Start(ctx)`. Long-poll begins.
15. **Log startup audit event.** `kind=startup`, metadata includes
    the git SHA + version.
16. **Block on `ctx.Done()`.** A SIGINT/SIGTERM from systemd cancels
    the context; every goroutine drains and exits.

## Long-lived goroutines

Once startup completes, a steady-state shellboto has these
goroutines running:

| Goroutine | Owner | Purpose |
|-----------|-------|---------|
| Telegram long-poll | `go-telegram/bot` internals | `getUpdates` loop; dispatches to handlers |
| Audit pruner | `auditRepo.PruneLoop` | Hourly ticker; deletes rows older than retention |
| Idle-reap | `shell.Manager.reapLoop` | 1-min ticker; closes shells idle > `idle_reap` |
| Supernotify TTL sweeper | `supernotify.Emitter` | Strips inline keyboards from old admin DMs after `super_notify_action_ttl` |
| Per-active-shell: pty reader | `shell.Shell.readPty` | Reads bytes from pty → appends to current Job buffer |
| Per-active-shell: ctrl reader | `shell.Shell.readCtrl` | Reads `done:<N>` messages from fd 3 → signals Job done |
| Per-running-command: streamer | `stream.Streamer.Stream` | Edits Telegram message on `edit_interval`; spills to file at 4096-char cap |

Per-user shell goroutines are spawned lazily on first command from
that user, torn down on `/reset`, idle-reap, or service stop.

## State (in-memory)

- **`shell.Manager.shells`** — `map[int64]*Shell`, keyed by Telegram
  user ID. Guarded by a mutex. Populated on first command from a
  user, removed on close.
- **`telegram/ratelimit.Limiter.buckets`** — `map[int64]*bucket`
  keyed by Telegram user ID. Guarded by a mutex. Never freed during
  process lifetime (small; a few dozen bytes per whitelisted user).
- **`telegram/ratelimit.AuthRejectLimiter.buckets`** — same shape,
  keyed by pre-auth sender ID. Swept periodically to bound growth
  from attackers.
- **`supernotify.Emitter.pendingTTL`** — map of (chat_id, message_id)
  → expiry. Sweeper removes entries and strips keyboards on expiry.

## State (on disk)

- **`/var/lib/shellboto/state.db`** — SQLite file. Written on every
  audit event and every RBAC change. Read at startup (config check)
  and on every handler invocation (user lookup).
- **`/var/lib/shellboto/shellboto.lock`** — opaque empty file;
  exists to be `flock`-ed. Content unused.

## Graceful shutdown

systemd sends SIGTERM when `systemctl stop shellboto` or
`systemctl restart shellboto` fires. `main.go` installed a
`signal.Notify(ctx, SIGINT, SIGTERM)` trap.

Shutdown sequence:

1. Cancel the top-level context.
2. `bot.Stop(ctx)` — closes the long-poll; returns after the
   current `getUpdates` round-trip.
3. `shellMgr.CloseAll()` — for every live shell: send SIGTERM to
   bash's process group, wait 2s, then SIGKILL. Close pty + ctrl
   pipe. Finalise in-flight Jobs with exit -1.
4. Audit row `kind=shutdown` written.
5. `auditRepo.Stop()` — stops the pruner; waits for it to return.
6. DB close.
7. Instance-flock release (implicit on process exit).

systemd's default `TimeoutStopSec=90s` is plenty. If shellboto takes
longer (shouldn't), systemd escalates to SIGKILL.

## Startup invariants

- **The DB file exists before Open.** systemd's
  `StateDirectory=shellboto` creates `/var/lib/shellboto/` with mode
  0700 before ExecStart.
- **Config is read before network.** If the config is bad, we fail
  fast without opening a connection to Telegram or the DB. No
  partial-state leaks.
- **Exactly one superadmin at startup.** `SeedSuperadmin` runs in a
  transaction that demotes any extra superadmins. The DB on disk
  can never have two superadmins after startup completes.
- **First audit row is `kind=startup`.** If you see a new
  `kind=command_run` before `kind=startup`, the process crashed and
  its restart wrote `startup` later — investigate the gap.

## Failure modes at runtime

| Failure | What happens | Recovery |
|---------|--------------|----------|
| Telegram API 5xx / network blip | go-telegram/bot retries with backoff | Self-heals; check `journalctl` for spikes |
| SQLite write error | Audit write returns error; caller's command still completes (the audit is best-effort for resilience) | Investigate DB corruption; see [../runbooks/db-corruption.md](../runbooks/db-corruption.md) |
| Audit-chain panic (verify walker) | `recover()` in pruner catches it; process keeps running | Run `shellboto audit verify` manually; see [../runbooks/audit-chain-broken.md](../runbooks/audit-chain-broken.md) |
| Shell pty disappears (bash crashed) | Reader goroutine returns err; `Shell.Close` fires; state cleared | User sees `⚠ shell died`; next message spawns a new shell |
| Bot token invalid (revoked) | Long-poll returns 401; bot logs and exits | systemd restarts; fix `SHELLBOTO_TOKEN`; restart |

## Memory + CPU footprint

Steady-state, no active shells:

- **RAM:** ~30 MB RSS (Go runtime + dep modules).
- **CPU:** <1% of a core. Long-poll interval default.

With 5 active shells each running `find /`:

- **RAM:** ~50–80 MB RSS (per-shell buffers bounded by
  `max_output_bytes`, default 50 MB).
- **CPU:** dominated by the shells themselves, not the bot. The
  reader goroutines are I/O-blocked.

## Scaling limits

- **One process per VPS.** The `flock` invariant prevents parallel
  instances. If you need HA, run on a fresh VPS with a fresh audit
  chain.
- **SQLite concurrency.** Audit writes serialise through a single
  mutex. Sustained >1000 commands/sec would contend; real workloads
  don't approach that.
- **Telegram Bot API rate limits.** ~30 messages/sec per bot
  overall. Streamer's debounce + edit-skip keeps us well under.

Read next: [data-flow.md](data-flow.md).
