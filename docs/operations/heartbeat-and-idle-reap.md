# Heartbeat + idle reaping

Two different timers. Don't confuse them.

For config details: [../configuration/timeouts-and-reaping.md](../configuration/timeouts-and-reaping.md).

## Heartbeat — per-command liveness to Telegram

When a command is running but producing no output, the streamer
would hit its debounce and skip edits. Without intervention, the
Telegram message looks frozen.

Every `heartbeat` seconds (default 30s), the streamer edits the
message anyway, appending a "still alive" trailer:

```
<pre>find / -name '*.log'</pre>
running... still alive (30s)
[ Cancel ]  [ Kill ]
```

On the next tick (or real output arrival), the trailer updates.

Operationally:

- Too short → visible flicker on every edit.
- Too long → user wonders if the bot crashed.
- **0** → disables heartbeat. Long silent commands look frozen.
  Don't do this.

Default 30s is a good balance. Only touch if users complain.

## Idle reap — per-user shell cleanup

Per-user ptys sit in `Shell.Manager.shells` until either:

- The user runs `/reset`.
- The service restarts.
- The reaper closes them for inactivity.

The reaper runs on a 1-minute ticker. For each shell, if
`now - lastAct > idle_reap`, it calls `Shell.Close()` and removes
the entry from the map.

`lastAct` is updated on every byte read from the pty and every
byte written to it, so "idle" means "no I/O at all for `idle_reap`."

Default `idle_reap = 1h`. Tuning:

- Long (`6h` / `24h`) — shells survive most of a work day.
  Memory + process count grow linearly with active users.
- Short (`10m` / `30m`) — aggressive cleanup. Users re-spawn
  shells more often; lose `cd` state across chat-lull pauses.

**0** disables reaping. Shells live until service restart. Memory
grows unbounded; not recommended.

## Seeing what's happening

In journald:

```bash
sudo journalctl -u shellboto | grep 'reap\|idle'
```

Sample lines:

```
{"level":"info","msg":"reaping idle shell","user_id":987654321,"idle_duration":"1h2m15s"}
{"level":"info","msg":"shell closed","user_id":987654321,"reason":"idle_reap"}
```

Audit kind `shell_reaped` fires on reaper close, with `detail`
giving the idle duration.

## `/status` from the user's perspective

```
/status
```

Replies:

```
idle for 42m (last: 2026-04-20 14:22 UTC)
shell uptime: 2h 15m
```

The user can anticipate if they're close to idle-reap.

## When a reaped shell is visible

User sent `cd /tmp` an hour and 5 minutes ago. Reap fires. The
shell is gone.

User sends `pwd` now. The handler sees no shell for them, calls
`GetOrSpawn` → new shell. `pwd` runs in the new shell. Reply:
`/root` (or wherever bash's new default cwd is; not `/tmp`).

User may be confused by the reset. Educate or raise `idle_reap`.

## Interaction with graceful shutdown

On `systemctl stop shellboto`:

1. ctx cancel fires.
2. The reaper's ticker loop sees ctx.Done, returns.
3. `shellMgr.CloseAll()` fires — closes every remaining shell
   (same mechanism as reap, just en masse).
4. `shell_reaped` audits fire for each (with reason
   `manager_closed`).

Restart → users start fresh.

## Reading the code

- `internal/shell/shell.go:Manager.reapLoop`
- `internal/telegram/streamer.go:flush` (heartbeat trailer)

## Read next

- [user-management.md](user-management.md) — the other per-user
  operation.
- [../configuration/timeouts-and-reaping.md](../configuration/timeouts-and-reaping.md)
  — tuning.
