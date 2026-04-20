# Design decisions

The non-obvious choices, with reasoning. Something looks weird →
start here before "fixing" it.

## Why pty-backed bash instead of one-shot `exec`

**Choice:** every user gets a persistent bash in a pty.

**Why:** `cd /tmp && ls` would work in one-shot `exec`, but
`cd /tmp` then (separate message) `ls` wouldn't — the second `exec`
starts at `$HOME`. Same for env vars, aliases, background jobs.
shellboto is pitched as "a live shell on your VPS" — half-working
stateful shells would break the mental model.

**Cost:** keeping a pty open per user → memory overhead, idle-reap
logic, more edges to handle (bash crashed, pty closed, user spammed
`exec bash`).

**Alternative considered:** ephemeral scripts in a tmp file.
Rejected — state doesn't persist, and forwarding env + cwd across
every command gets its own edge cases.

## Why fd 3 for the control pipe, not stdout parsing

**Choice:** bash writes `done:<exitcode>\n` to fd 3 (set up as an
`ExtraFiles` pipe; dup'd to fd 100 once bash starts so user code
can't close it). Parent reads fd 3 to learn "command finished."

**Why:** parsing bash's prompt out of the pty stream is fragile.
`PS1` can be arbitrary. Commands that clear the screen or rewrite
lines via ANSI would confuse a prompt-scanner. And the user's
command output itself might literally contain "$ " or whatever
we chose as a sentinel.

**The fix:** separate the control channel from the data channel at
the kernel level. fd 1 (stdout) is unmodified user output. fd 3
is ours, inherited by bash on spawn, set readonly, dup'd to fd 100
via a trivial `PROMPT_COMMAND` so user commands can't close it.
Nothing a user runs can break boundary detection.

**Downside:** a custom `PROMPT_COMMAND` is installed. If the user
does `exec bash` or `PROMPT_COMMAND=unset`, the dispatcher dies and
boundary detection silently stops working. The per-command timeout
(`default_timeout`, default 5m) catches it; `/reset` is the manual
recovery. Listed as a known limitation in the README.

## Why hash-chained audit rows

**Choice:** every audit row's `row_hash = sha256(prev_hash ||
canonical_json(row))`. The genesis seed comes from
`SHELLBOTO_AUDIT_SEED`.

**Why:** if an attacker compromises root on the VPS, they can
rewrite `/var/lib/shellboto/state.db` arbitrarily. Hash-chaining
makes a silent edit detectable:

- Rewrite a single row → downstream rows' hashes are now wrong;
  `audit verify` reports the first bad row.
- Truncate the chain (delete rows) → verify reports a gap between
  the genesis seed and the first surviving row.
- Rebuild the chain from scratch with tampered data → the attacker
  would need the seed to compute the first row's hash. The seed is
  in `/etc/shellboto/env`, not in the DB.

**Cost:** an extra ~64 bytes per row (prev_hash + row_hash). One
`sha256` per write. Serialised through a mutex (can't parallel-
insert audit rows). None of these matter at realistic write rates.

**Alternative considered:** Merkle tree / external notary (timestamp
service, or writing to a remote log). Rejected for this project's
scope: single-VPS, single-operator, no external trust root. The
seed + chain is enough of a speedbump for the stated threat.

## Why single binary, no Docker image

**Choice:** `bin/shellboto` is the entire runtime. Goreleaser
ships it as a static binary + .deb + .rpm + Homebrew formula. No
official Dockerfile.

**Why:** the target user has a VPS, not a container platform. The
installer writes a systemd unit because that's what's on the VPS
anyway. Adding Docker as a "supported" distribution path would
double the installer surface area and fragment the runbooks.

A Dockerfile is trivial to write and nothing in the code prevents
running in a container. But we don't promise it'll work with
systemd's `StateDirectory=`, graceful shutdown semantics, or the
installer scripts; those are tied to the host model.

## Why Go

**Choice:** Go 1.26+, no CGO.

**Why:**
- **Single static binary.** Copy + run. No runtime to install.
- **goroutines match the domain.** One goroutine per active shell
  is the obvious concurrency model.
- **Mature ecosystem.** `creack/pty` (real pty, not subprocess
  shell-out), `go-telegram/bot` (actively maintained), GORM +
  `modernc.org/sqlite` (pure-Go DB), `zap` (structured logging).
- **Fast edit-compile-test.** `go test ./...` is seconds.
- **Cross-compile is free.** Linux amd64 + arm64, Darwin amd64 +
  arm64 from one CI runner.

**Alternatives considered:**

| | Why not |
|---|---|
| Python | Runtime install required. Async pty handling harder. |
| Rust | Longer build time, smaller Telegram-bot ecosystem. Steeper ramp for casual contributors. |
| Node | Runtime install. Type-safety story weaker. |
| Bash | Boundary detection, audit hash chain, SQLite — all painful in bash. |

## Why the built-in danger matcher is a regex table

**Choice:** ~26 hand-picked regexes, one per destruction pattern.
Match on first hit. See
[../security/danger-matcher.md](../security/danger-matcher.md) for
every entry with examples.

**Why:**
- Regexes are easy for new contributors to reason about.
- Each pattern is independently auditable: "is this too broad?" →
  look at the examples.
- The matcher is a safety net for admins (typo guard) + a speedbump
  for lazy attackers. The real defense against a compromised admin
  is them not being compromised in the first place + OS-level perms
  for non-admin users.

**What it's not:** a comprehensive malware scanner. A determined
attacker with admin rights can bypass any regex (base64 → sh,
`${IFS}`, variable splitting, eval). Acknowledged in the package
doc comment and the security docs.

## Why auto-resets on role change

**Choice:** when you promote or demote someone, their current pty
is closed. Their next command spawns a new one with the updated
uid/gid/home.

**Why:** avoids a demoted admin's still-open root shell surviving
the demotion. The inverse (a promoted user stuck in their
non-root shell until they manually `/reset`) is less dangerous but
just as surprising. Uniform handling is less surprise.

## Why superadmin is seeded from env, not the DB

**Choice:** `SHELLBOTO_SUPERADMIN_ID` is an env var. Every startup
re-seeds the superadmin row, demoting any other row marked
superadmin to admin.

**Why:**
- No bootstrap problem ("how does the first superadmin exist?").
  The operator who can edit `/etc/shellboto/env` is the superadmin
  by definition.
- Handing off is a config change + restart, not a DB edit. Standard
  ops dance.
- An attacker who can `UPDATE users SET role='superadmin' WHERE…`
  but can't edit the env file gets their elevation wiped on next
  restart.

## Why the hash chain serialises through one mutex

**Choice:** `audit.Log` takes a package-global `logMu` mutex for
the whole read-previous + insert sequence.

**Why:** any concurrent write path would fork the chain. Two writers
reading the same previous row, both computing a next hash, would
produce two distinct rows that both claim to follow the same
parent — the chain is no longer a chain.

**Cost:** audit writes are sequential. At shellboto's write rates
(commands per minute per user, N users), nowhere near contention.
A write-heavy future could shard by user ID with per-user sub-chains,
but that's premature.

## Why flock(LOCK_EX|LOCK_NB) at startup

**Choice:** exactly one shellboto per DB file. `flock` on
`/var/lib/shellboto/shellboto.lock` is `LOCK_EX | LOCK_NB` — fail
fast if another process holds it.

**Why:** two shellbotos writing to the same SQLite file could fight
the audit chain serialisation even with `logMu`, because `logMu`
is per-process. SQLite's own locking prevents corruption but
doesn't prevent the chain fork.

Kernel releases the flock on process exit (including crash), so a
stuck lockfile is not a thing. The same mechanism makes it safe to
`rm` the lockfile if you want to manually reset (but usually you
don't need to).

## Why audit output is gzipped, not compressed with zstd or lzma

**Choice:** gzip (stdlib, level default-6).

**Why:**
- `compress/gzip` is stdlib; no extra dependency.
- Command outputs are mostly short; compression ratio differences
  between zstd and gzip at 5–50 KB are trivial.
- The `zcat` / `gunzip` tools are universally available; an operator
  grabbing a raw SQLite file for forensics can trivially inspect the
  blob.

## Why multi-format config (TOML + YAML + JSON) instead of one

**Choice:** parser chosen by file extension. Same schema across all
three.

**Why:** deployment environments have preferences. Infra-wide YAML
shops would rather not flip one service to TOML; the install.sh
interactive prompt picks the format so the operator doesn't fight
a hardcoded choice.

**Cost:** three parsers instead of one. All three are stdlib-adjacent
(BurntSushi/toml is the canonical Go TOML parser; gopkg.in/yaml.v3
is standard; `encoding/json` is stdlib). Maintenance cost is low.

## Why `strict_ascii_commands` is opt-in

**Choice:** `strict_ascii_commands = false` by default. When true,
any command with non-printable-ASCII bytes is rejected.

**Why:** unicode homoglyphs and control bytes in commands are useful
markers of "something is wrong," but they break legitimate i18n —
accented filenames, non-English text in `echo` payloads, emoji in
notifications. Most operators don't need paranoia mode; those who
do (security-heavy deployments) can opt in.

## Things explicitly **not** changeable via this design

- **One superadmin.** Fundamental to the seeding model. Change it
  by editing the env and restarting — it *is* handoff, and it's
  cheap.
- **Telegram-specific transport.** `telegram/*` packages are
  tightly coupled to the Bot API's update model and inline-keyboard
  semantics. A Slack / Matrix / Discord port is a rewrite.
- **No HA.** One process per DB. Want redundancy? Run a second
  instance on a different VPS with a separate audit chain; they
  don't share state.

## Read next

Back to [README.md](README.md), or jump to
[../configuration/](../configuration/) to learn how to tune this
runtime.
