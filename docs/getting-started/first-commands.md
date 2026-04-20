# Your first commands

Your bot is running. You're superadmin. You've said `/start` and
gotten a greeting back. Here are the commands worth trying right
away to build intuition.

## Send a shell command

Just type it. No prefix. Anything that bash would execute.

```
hostname
```

You'll see the bot send a reply that ends up saying:

```
hostname
shellboto-dev-01
✅ exit 0 · 12ms
```

The text is inside a single Telegram message that the bot **edits
in place** as output streams in. For short commands it all lands
before the first edit tick.

Try a few more:

```
uname -a
ls -la /etc/shellboto
df -h
date -Is
```

Each runs in your persistent pty. State carries across — env vars,
`cd`, aliases, background jobs. For example:

```
cd /etc/shellboto
```

```
pwd
```

The second reply says `/etc/shellboto`.

## `/status` — what's my shell doing

```
/status
```

Replies with:

- **Idle** → no command currently running. Shows last activity
  timestamp and shell uptime.
- **Running** → a command is executing. Shows the cmd, start time,
  and elapsed. You can `/cancel` or `/kill` it.

Useful when something hangs and you want to confirm it.

## `/cancel` — soft interrupt

```
sleep 30
```

While that's running, send:

```
/cancel
```

That's a SIGINT (Ctrl+C) to the foreground process group. The
running message gets edited to show `⚠ interrupted` and exits
non-zero. You're back to a prompt.

## `/kill` — hard kill

If the foreground process ignores SIGINT:

```
sleep 30 &           # backgrounded (won't actually be cancellable this way)
while true; do :; done
```

Send `/kill`. That's SIGKILL to the foreground process group.
Can't be ignored.

`/kill` does NOT kill bash itself — it refuses to signal the shell's
own PID. Use `/reset` for that.

## `/reset` — respawn the shell

```
/reset
```

Closes your pty, starts a fresh one. Env vars, `cd` state, aliases,
background jobs — all gone. Use when:

- You `exec bash`'d and wedged the boundary detection.
- You want a clean environment after a lot of tinkering.
- A role change happened (promote/demote) — the bot auto-resets
  your shell so your new privileges kick in.

## Danger commands

Send something dangerous:

```
rm -rf /tmp/nonexistent
```

The bot replies with a warning keyboard:

```
⚠  This command matched danger pattern: rm-rf
    rm -rf /tmp/nonexistent

    Tap ✅ Run to execute, or let this prompt expire.
    [ ✅ Run ]  [ ❌ Cancel ]
```

Tap **Run** to proceed, **Cancel** (or let 60 seconds pass) to drop
it. Every danger prompt is audited as `danger_requested`; your choice
is logged as `danger_confirmed` or `danger_expired`.

Full list of danger patterns: [../security/danger-matcher.md](../security/danger-matcher.md).

## `/auditme` — my own audit history

```
/auditme
```

Lists your last 10 audit events: commands, danger prompts, file
ops. Every user has access to their own audit trail.

## Admin+ commands

If you're superadmin (you are after install):

- `/users` — list everyone on the whitelist, their roles, when they
  were added, who added them.
- `/adduser <telegram_id>` — add someone. They're `role=user` by
  default.
- `/deluser <telegram_id>` — remove someone. Their shell is
  auto-closed if they had one.
- `/audit [N]` — last N audit events across all users (default 20).
- `/audit-out <id>` — fetch the captured output for a single audit
  event (gzipped → decompressed → sent as text or `output.txt`).
- `/audit-verify` — walk the hash chain, report any break.

Superadmin-only:

- `/role <telegram_id> admin|user` — promote or demote.

## File ops

### Download a file from the VPS:

```
/get /var/log/syslog
```

Sent to you as a Telegram file upload (max 50 MB per Telegram's
bot API cap).

### Upload a file to the VPS:

- Attach a file to a Telegram message (paperclip → File, **not**
  paperclip → Photo; photos get re-encoded).
- Optional: in the caption, type the destination path. If absent,
  the file lands in your shell's current working directory.

`/get` and uploads are audited as `file_download` and `file_upload`.

## Things to try that *won't* work

These expose the boundaries — understanding them up front saves
confusion later:

- `vim`, `nano`, `less`, `top`, `htop` — interactive full-screen
  programs. Telegram isn't a terminal. Use non-interactive
  equivalents: `cat`, `sed`, `tail`, `ps auxf`, `htop -n 1 | head`.
- `clear` — will send some escape codes to the shell; the bot
  strips them before audit/display. Doesn't break anything, just
  no-ops visually.
- `exec bash` / `exec sh` — re-execs the shell in place, discarding
  our `PROMPT_COMMAND`. Boundary detection dies; the command
  eventually hits `default_timeout` and the reaper kicks in.
  `/reset` recovers immediately.

## Read next

- [../security/](../security/) — before inviting anyone else to the
  whitelist.
- [../operations/](../operations/) — doctor, logs, user management,
  monitoring.
- [../reference/telegram-commands.md](../reference/telegram-commands.md)
  — exhaustive table of every slash command.
