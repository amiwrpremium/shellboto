# FAQ

## Is this safe to run on my VPS?

**Only as safe as your whitelist and your Telegram account.** The
whitelist is a single table in SQLite; anyone on it can execute
arbitrary commands as whatever user you configured (root by default).
If a whitelisted Telegram account is compromised — phishing, SIM swap,
session hijack — the attacker inherits that shell. Enable 2FA on
Telegram. Keep the whitelist tight. Use [non-root `user`
shells](configuration/non-root-shells.md) for anyone who isn't you.

## Why a pty instead of just running commands?

A pty gives you a persistent bash per user — `cd`, env vars, aliases,
job control, shell history all work. Running one-shot `exec` would
kill every command's environment the moment it finished. See
[shell/pty-vs-exec.md](shell/pty-vs-exec.md).

## Why does `vim` or `top` not work?

Telegram isn't a terminal. It can't render cursor addressing, color
codes (we strip them anyway), or redraw-on-keystroke UIs. Use
`cat`, `tail`, `ps`, `htop -n 1`, `top -b -n 1 | head`, etc.

## My command hangs and never returns a prompt. What happened?

You probably ran `exec bash` or `exec sh`. That replaces the shell
image in place, which discards our `PROMPT_COMMAND` dispatcher.
Boundary detection dies silently. The per-command timeout
(`default_timeout`, default 5m) eventually fires and reaps it.
`/reset` recovers immediately. See
[troubleshooting/commands-never-complete.md](troubleshooting/commands-never-complete.md).

## Can I run this in Docker?

Nothing stops you, but shellboto assumes a systemd-managed VPS. The
installer writes a systemd unit, the installer + uninstaller drive
`systemctl`, the audit DB wants to live in `StateDirectory`. For a
container: skip `install.sh`, mount config + state as volumes, run
`/usr/local/bin/shellboto -config /etc/shellboto/config.toml` as
PID 1. Stop/restart signals become container lifecycle. No official
Dockerfile is shipped.

## How big does the audit DB get?

Bounded by retention. Default `audit_retention = 2160h` (90 days)
with the hourly pruner means steady-state size ≈
`avg_rows_per_day × 90`. Each row is small metadata; the captured
output blob (gzipped) dominates. Enable
`audit_output_mode = errors_only` or `never` if you don't need
forensic replay of successful commands.

## Does shellboto see my bot token?

Yes — it's in `/etc/shellboto/env` (mode 0600, root-owned) and in
the process's environment. It's **stripped from the spawned bash's
environment** before any user sees a shell, so `printenv` / `env`
can't leak it from inside a pty. But the `env` file on disk is
protected only by filesystem perms; root (and shellboto) can read it.

## Can I have two superadmins?

No. Exactly one, seeded from `SHELLBOTO_SUPERADMIN_ID`. Any other
row found with `role=superadmin` at startup is automatically demoted
to `admin`. Change the env var + restart to hand off.

## Does `/audit-verify` mean the output wasn't tampered with?

It means the hash chain of audit *metadata* is intact. The output
blob's SHA-256 is included in the canonical hash, so if anyone
rewrites the blob without recomputing the whole downstream chain,
verify catches it. Without the `SHELLBOTO_AUDIT_SEED`, an attacker
with full DB write access can rebuild the chain silently. With the
seed, they can't. See [security/audit-chain.md](security/audit-chain.md).

## Why does the installer ask for the bot token with hidden input?

So it doesn't end up in your shell history (`~/.bash_history`) or
in the terminal scrollback. The prompt uses `read -rs`; nothing
echoes.

## Can a `user`-role caller escalate to admin?

Not through any built-in code path. They'd need to: (1) compromise
an admin's Telegram account, (2) find a shell-escape in a non-root
context AND local privilege escalation to root, AND (3) rewrite
the user-role's DB row. The danger matcher explicitly blocks
patterns that mutate auth files (`/etc/shadow`, `/etc/sudoers.d/*`,
`/root/.ssh/authorized_keys`) so typos don't accidentally open that
path. See [security/root-shell-implications.md](security/root-shell-implications.md).

## Can I add custom danger patterns?

Yes. `extra_danger_patterns` in config is a list of regex strings
that get merged with the built-ins. See
[security/danger-matcher.md#adding-patterns](security/danger-matcher.md#adding-your-own-patterns).

## Where does the bot's output go if I'm not looking at Telegram?

Every command (kind, exit code, duration, size, full output blob)
is persisted to SQLite. Query offline with:

```bash
shellboto audit search --user $TELEGRAM_ID --since 24h
shellboto audit export --since 7d --format json > audit.jsonl
```

Also mirrored as structured JSON log lines to the `audit` zap logger
→ journald. If someone wipes the DB, the journal survives. See
[audit/](audit/).

## Why Go and not $OTHER_LANGUAGE?

Static binary, no CGO, no runtime to install on the target VPS, good
concurrency primitives for "one goroutine per pty" and "fan-out
notification", ecosystem has mature pty (creack/pty), Telegram Bot
API client (go-telegram/bot), SQLite driver (GORM + modernc.org/sqlite
pure Go). Python or Node could do it — the binary size would be
worse and dependencies on the host would grow.

## How do I hand off to someone else as operator?

1. Put their Telegram ID in `SHELLBOTO_SUPERADMIN_ID`.
2. `sudo systemctl restart shellboto`.
3. Old superadmin row auto-demotes to `admin` (or you can
   `/deluser` them once they've confirmed they're in).

## Is there a hosted version?

No. Self-host only. The threat model assumes you control the VPS.

## What Telegram clients work?

All of them — the bot sends and receives standard Bot API messages
and inline keyboards. On mobile, inline keyboards for `/promote` or
`/danger` confirm are the same as desktop.

## What about Slack / Discord / Matrix?

Not supported. The streaming message-edit pattern is Telegram-specific
(most chat platforms don't support per-message edits at that rate or
cap output the same way). A port is a rewrite, not a flag.
