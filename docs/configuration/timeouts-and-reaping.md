# Timeouts and reaping

Five knobs control shellboto's time-based behaviour. Tune them
together — they interact.

| Key | Default | Controls |
|-----|---------|----------|
| `default_timeout` | `5m` | Per-command hard cap before SIGINT |
| `kill_grace` | `5s` | Delay from SIGINT to SIGKILL after `default_timeout` |
| `edit_interval` | `1s` | How often the streamer edits the Telegram message |
| `heartbeat` | `30s` | Still-alive heartbeat while output is silent |
| `idle_reap` | `1h` | How long an idle pty lives before being closed |

## Per-command timeout (`default_timeout` + `kill_grace`)

```
command sent
    │
    ▼
bash executes ..........
    │
    │ ....output streams to Telegram
    │
    │                           ← default_timeout elapses
    ▼
SIGINT (Ctrl+C)
    │
    │ ... process may or may not clean up
    │
    │                           ← kill_grace elapses
    ▼
SIGKILL
    │
    ▼
Job.finish(exit=-1)
audit row kind=command_run, termination=timeout
```

**Tuning:**

- **`default_timeout = 5m`** is conservative for operator-shell
  workloads. A build, a migration, a `find /` on a big disk can
  all exceed 5m. Raise to `30m` or `2h` if your team does that
  routinely.
- **`kill_grace = 5s`** is enough for most commands to respond to
  SIGINT (flush buffers, remove tempfiles, etc.). Raise if you run
  commands that need >5s cleanup windows.
- **Don't disable either.** Setting them to `0` effectively
  disables the protection. Do not.

## Edit interval (`edit_interval`)

How often the streamer goroutine edits the Telegram message with
fresh output bytes.

```
t=0ms   command sent, placeholder message created
t=1s    first edit: "running... 0 bytes"
t=2s    second edit: "running... 128 bytes"
t=3s    third edit: "hello\nworld\n"  ← output arrived
...
```

**Tuning:**

- Shorter (`500ms`, `250ms`): snappier UX. More Bot API calls. At
  `250ms`, you're at 4 edits/sec — well under Telegram's per-bot
  rate limit (~30/sec), but expensive across many concurrent
  commands.
- Longer (`3s`, `5s`): smoother; output appears in larger chunks.
  Users may think commands are hanging.
- **Debounce** is automatic: if the output hasn't changed since
  the last edit, no Bot API call is made. So idle shells don't
  burn quota.

## Heartbeat (`heartbeat`)

If a command is producing no output, the message would look frozen.
Without a heartbeat, a user watching a long `find /` with no matches
wouldn't know if the bot crashed.

```
t=0     command sent
t=1s    edit: "running... 0 bytes"
t=2s    (no change, edit skipped by debounce)
...
t=30s   edit: "running... still alive (30s)"
```

**Tuning:**

- `30s` is a reasonable default. Shorter feels twitchy; longer
  feels dead.
- If you disable (`0`), silent commands look frozen to Telegram
  users. Don't.

## Idle reap (`idle_reap`)

Per-user shells are kept alive across messages so your `cd`, env
vars, and bash history persist. But shells not used in a while just
burn memory.

The manager's reap loop wakes every 1 minute and closes shells with
no activity in the last `idle_reap`.

```
2026-04-20 10:00:00  user sends "cd /tmp", shell alive
2026-04-20 10:01:00  reap tick: last_activity was 60s ago → idle_reap=1h, keep
2026-04-20 11:05:00  reap tick: last_activity was 65min ago → close shell
```

When the user messages again at 11:30, a **fresh shell spawns** —
their cwd is back to `$HOME`, env vars gone.

**Tuning:**

- `1h` default is decent for occasional-use accounts.
- Shorter (`10m`, `30m`) for tight-memory hosts or bigger teams.
  Users re-adapt quickly; `/reset` does the same thing.
- Longer (`6h`, `24h`) if you want shells to survive most of a
  work day.
- **`0`** disables reap entirely. Shells live until service
  restart. Not recommended (memory creeps).

## Interaction diagram

```
user input
   │
   ▼
 shell Job starts
   │
   ├─ edit_interval ticks → streamer edits message
   │
   ├─ heartbeat ticks (when output silent) → edit adds "still alive"
   │
   └─ default_timeout elapses → SIGINT
                                 │
                                 └─ kill_grace → SIGKILL

after command finishes, shell stays alive for idle_reap
                                 │
                                 └─ reap closes it
```

## Recommended profiles

### "Operator shell for me" (default)

```toml
default_timeout = "5m"
kill_grace = "5s"
edit_interval = "1s"
heartbeat = "30s"
idle_reap = "1h"
```

Fine for everyday sysadmin tasks.

### "I run long-running builds"

```toml
default_timeout = "2h"
kill_grace = "10s"
edit_interval = "2s"
heartbeat = "1m"
idle_reap = "3h"
```

Edit interval longer (no need for sub-second updates on builds);
`default_timeout` 2h to survive CI-scale builds.

### "Memory-tight VPS, occasional use"

```toml
default_timeout = "2m"
kill_grace = "3s"
edit_interval = "1s"
heartbeat = "20s"
idle_reap = "15m"
```

Kills idle shells quickly, keeps per-command timeouts tight.

## What shellboto doesn't have (on purpose)

- **Per-user timeouts.** One `default_timeout` applies to everyone.
  A future feature could vary by role or by telegram_id; not
  shipped.
- **Soft vs hard timeouts.** SIGINT → SIGKILL is the whole story.
  No user-visible "warn at 80%" step.
- **Max shell lifetime.** No "close this shell after N hours
  regardless of activity." Idle-reap + `/reset` + service restart
  are the available tools.

## Read next

- [audit-output-modes.md](audit-output-modes.md) — the one other
  knob that materially changes runtime behaviour.
- [../operations/heartbeat-and-idle-reap.md](../operations/heartbeat-and-idle-reap.md)
  — operational view of these timers.
