# Threat model

What shellboto protects against, what it doesn't, and why.

## Assets

1. **The VPS.** Anything on the filesystem, any processes, any
   outbound network capability.
2. **The bot token.** If leaked, anyone can impersonate the bot.
3. **The whitelist.** Its membership defines the blast radius.
4. **The audit log.** Forensic record of who ran what.
5. **Secrets inside command output** (keys, tokens, passwords
   emitted by commands the user ran).

## Actors

| Actor | Access | Intent |
|-------|--------|--------|
| **Operator (superadmin)** | Full — root shell, all config, DB write | Legitimate |
| **Admin** | Root shell, RBAC-limited user management | Trusted team member |
| **User** | Non-root shell (if `user_shell_user` configured), no user management | Trusted team member, reduced privilege |
| **Stranger on Telegram** | None (not whitelisted) | Unknown — might be a botnet, might be a typo |
| **Local attacker** | On the same VPS, non-shellboto context | Present if the VPS has other compromised services |
| **Network adversary** | Between bot and Telegram Bot API | Passive observation of bot ↔ Telegram traffic |

## What shellboto protects against

### ✅ Accidental destructive commands by admin+

The [danger matcher](danger-matcher.md) trips on ~26 destruction
patterns (`rm -rf /`, `dd of=/dev/sda`, `mkfs.*`, pipe-to-shell,
etc.). Admin must tap ✅ Run to proceed; tapping ❌ Cancel or
letting the 60s TTL elapse drops it.

Bypasses: yes, a sufficiently clever command string (base64 → sh,
`${IFS}` splitting, eval) defeats the regex. Acknowledged — this
layer is a typo guard, not anti-malware.

### ✅ Stranger spam filling the audit DB

Without mitigation, an attacker who discovers your bot's username
can send 30 updates/sec and rapidly grow the `audit_events` table
(every rejected update writes a row). The pre-auth rate limiter
(`auth_reject_burst` / `auth_reject_refill_per_sec`) caps a single
attacker to ~4300 rows/day (~2 MB).

See [rate-limiting.md](rate-limiting.md).

### ✅ Secrets landing in the audit log

The [secret redactor](secret-redaction.md) scrubs:

- Private keys (SSH/TLS)
- JWTs
- Bearer / Basic auth
- GitHub / GitLab / Google / AWS / Stripe / Slack tokens
- `--password=...` / `-pXXXX` flags
- Generic `TOKEN=`, `SECRET=`, `API_KEY=` assignments
- `/etc/shadow` and `/etc/passwd` hashes

Applied to **cmd** and **output** before storage. Both the redacted
cmd and the redacted output flow into the canonical-form hash.

### ✅ Silent edits to audit rows after the fact

The [hash chain](audit-chain.md) means any row tamper breaks
downstream hashes. `/audit-verify` walks the chain and reports the
first mismatched row. Without the `SHELLBOTO_AUDIT_SEED`, an
attacker with full DB access can rebuild the chain post-facto —
the seed closes that hole.

### ✅ The bot token leaking via user shell

`internal/shell/shell.go`'s `sanitizedEnv` strips every
`SHELLBOTO_*` variable before `execve`-ing bash. A user-role
caller running `printenv SHELLBOTO_TOKEN` gets nothing.

The token is still readable by root on the VPS (it's in
`/etc/shellboto/env`, mode 0600). If the VPS is compromised at
root level, the token is gone.

### ✅ Non-admin privilege escalation via regex bypass

If an admin's account is *not* compromised but a
`role=user` account is, the user runs as `user_shell_user` (a
non-root account with no sudoers entries). Every danger pattern's
OS-level impact drops to "user-account-limited" — `rm -rf /`
fails on perms before it can chew through anything important.

### ✅ Accidental admin-to-admin demotion abuse

An admin can demote only admins they themselves promoted
(`promoted_by` column). Alice can't kick Bob out if Charlie
promoted Bob. Only superadmin can override the chain.

### ✅ Two shellboto processes on the same DB

The `flock(LOCK_EX|LOCK_NB)` on `/var/lib/shellboto/shellboto.lock`
rejects a second process trying to open the same DB. No parallel
audit chain forks.

## What shellboto does NOT protect against

### ❌ A compromised Telegram account

If your Telegram is taken over (phishing, SIM swap, session
hijack), shellboto sees no difference. Mitigations are external:

- 2FA on Telegram itself.
- Passcode lock on the device running Telegram.
- Official Telegram client, not a third-party skin.
- Be suspicious of @BotFather DMs from "Telegram Support" that
  aren't the real support channel.

### ❌ A malicious operator (superadmin)

Superadmin can:

- Run any command as root.
- Edit `/etc/shellboto/env` and rotate the audit seed silently —
  though rotating breaks the chain, and any extant journald mirror
  would diverge visibly.
- Directly `sqlite3 /var/lib/shellboto/state.db` and rewrite rows.
  The hash chain catches single-row edits; a sophisticated attacker
  who also knows the seed can rebuild the chain cleanly, but the
  journald mirror would still disagree unless they also had root
  to edit that.

If your superadmin is untrusted, you have bigger problems than
shellboto can solve.

### ❌ Sandbox escape via a legitimate command

`cat /etc/shadow` for a `role=user` fails on perms. Great.
`exploit-of-the-week /proc/self/...` that gets the user kernel-level
code exec is not something shellboto catches — it's an OS-level
problem, and you'd need to keep the kernel patched independently.

### ❌ Supply-chain attacks on dependencies

We run `govulncheck` on every push and ship an SBOM per release,
but that's detection, not prevention. A compromised transitive
dep (e.g. a rogue `gorm.io/gorm` update) is possible. Mitigations:

- Dependabot security PRs stay open for human review (auto-merge
  is disabled for `security`-labelled PRs).
- Release binaries are reproducible from tagged source.

### ❌ Network adversary reading bot traffic

Bot API is TLS-protected. A network adversary can observe *that*
you're talking to `api.telegram.org` and how much, but not what.
If your network is untrusted, route via a VPN you control.

### ❌ Physical access to the VPS

If someone has console access, they've won. shellboto has no FDE,
no offline secrets mechanism.

## Design guarantees (claims we stand by)

- After restart, exactly one row in `users` has `role=superadmin`.
- Two shellboto processes never share a DB file at the same time.
- Every audit row's `row_hash` equals `sha256(prev_hash ||
  canonical_json(row))` where `canonical_json` has fixed field
  order and UTC timestamps.
- Commands matching the danger matcher always require `/confirm`
  before execution (no "skip confirm" bypass).
- `role=user` callers' shells run under the configured unix
  identity if `user_shell_user` is set.

## Bugs welcome

If you find a gap between what this document claims and what
shellboto actually does, that's a security bug. Email
amiwrpremium@gmail.com (per `CONTRIBUTING.md`); don't open a
public issue.

## Read next

- [whitelist-and-rbac.md](whitelist-and-rbac.md) — layer 2+3 in
  detail.
- [danger-matcher.md](danger-matcher.md) — layer 5's full
  rulebook.
