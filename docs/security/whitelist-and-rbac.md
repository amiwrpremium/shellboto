# Whitelist and RBAC

How shellboto decides who is allowed to do what.

## The whitelist

A single SQLite table: `users`. Schema:

```
telegram_id   INTEGER PRIMARY KEY
username      TEXT       -- @handle, refreshed on each message
name          TEXT       -- admin-entered friendly label
role          TEXT NOT NULL  -- "superadmin" | "admin" | "user"
added_at      DATETIME
added_by      INTEGER    -- telegram_id of adder; NULL for seeded superadmin
disabled_at   DATETIME   -- NULL = active; non-NULL = banned
promoted_by   INTEGER    -- telegram_id of promoter; NULL for user/superadmin
```

`telegram_id` is the primary key — immutable for the life of a
Telegram account, and what shellboto compares against incoming
updates.

## Authentication flow

For every incoming message / callback:

```
update arrives
    │
    ▼
middleware.WrapText (or WrapCallback)
    │
    ├─ pre-auth rate-limit check    → drop silently if over-limit
    │                                  (attacker ID, no audit row)
    │
    ├─ lookup users WHERE telegram_id = update.From.ID
    │
    │  ┌── not found, or disabled_at IS NOT NULL:
    │  │       │
    │  │       ├─ write audit kind=auth_reject (rate-limited)
    │  │       │     details: from_id, username, payload
    │  │       └─ drop (sender not told they were rejected)
    │  │
    │  └── found, active:
    │          │
    │          ├─ post-auth rate-limit check (burst=10, refill=1/s)
    │          │     exempt: /cancel, /kill, inline j:c / j:k
    │          │     over-limit: reply "rate limit", no dispatch
    │          │
    │          ├─ metadata touch (username, name, last_seen)
    │          │
    │          ├─ strict_ascii check (if enabled)
    │          │
    │          └─ dispatch to command/callback handler
    ▼
handler runs with access to user + RBAC helpers
```

Silence on reject is deliberate: an attacker probing the bot can't
enumerate whitelist members by spamming IDs and watching response
timing.

## Roles and transitions

Three roles:

- **superadmin** — one per deployment. Seeded from
  `SHELLBOTO_SUPERADMIN_ID` at every startup.
- **admin** — promoted by superadmin.
- **user** — default for new whitelist entries.

The full capability matrix is in
[../configuration/roles.md](../configuration/roles.md).

### Promote

`superadmin` uses `/role <id> admin` (or the Promote button in the
users browser). The target must be `role=user`, active, and
non-banned.

- DB: `UPDATE users SET role='admin', promoted_by=<actor_id>
       WHERE telegram_id=<target>`.
- Audit: `kind=role_changed`, `detail='user→admin (by actor_id)'`.
- Side effect: target's active pty shell is auto-closed. Next
  message spawns a new shell (still root since they're now
  admin, but the reset rotates any lingering state).
- Notification: superadmin gets a DM via supernotify; the actor
  (if not the superadmin themselves) does too.

### Demote

`/role <id> user` (or the Demote button).

- Superadmin can demote any active admin.
- Admin can demote **only admins they promoted** (via
  `promoted_by`).
- DB: `UPDATE users SET role='user', promoted_by=NULL
       WHERE telegram_id=<target>`.
- Audit: `kind=role_changed`, `detail='admin→user (by actor_id)'`.
- Side effect: target's active pty shell is auto-closed. Next
  message spawns a new shell; if `user_shell_user` is configured,
  the new shell runs as that non-root account.
- Notification: supernotify fan-out.

### Add

`admin+` uses `/adduser <id>` (wizard flow in
`telegram/flows/adduser.go`).

- Wizard collects: user ID, friendly name, confirmation.
- Validation: name must match `^[A-Za-z]+(?: [A-Za-z]+)*$`; unicode
  homoglyph attempts are rejected outright.
- ID validation: positive int64, not already on the whitelist.
- DB: `INSERT users (telegram_id, name, role, added_by, added_at)`.
- Audit: `kind=user_added`.
- Notification: supernotify to superadmin + immediate promoter.

### Remove (soft-delete)

`admin+` uses `/deluser <id>`. Actually soft-deletes (sets
`disabled_at`) so audit continuity is preserved.

- Admin can soft-delete only `role=user`.
- Superadmin can soft-delete any non-superadmin (can't delete self;
  would re-seed on next restart anyway).
- DB: `UPDATE users SET disabled_at=NOW() WHERE telegram_id=<target>`.
- Audit: `kind=user_removed`.
- Side effect: target's pty is auto-closed. Subsequent messages
  from them generate `auth_reject` audits.

### Re-add

Just `/adduser <id>` them again. The row is un-disabled
(`disabled_at` cleared) and their role set per the add flow.

### Ban (escalation)

When the bot detects hostile behaviour (e.g. a non-admin attempting
privilege escalation), it writes `kind=user_banned` and soft-
deletes the user. Semantically identical to `/deluser`, but the
audit row marks it as auto-rather-than-operator-initiated.

## The superadmin re-seed dance

At every startup, `userRepo.SeedSuperadmin(cfg.SuperadminID)` runs
inside a transaction:

1. Read all rows with `role='superadmin'`.
2. For each except the target ID, `UPDATE role='admin'`.
3. If the target ID exists: `UPDATE role='superadmin',
   disabled_at=NULL` (re-enable if banned).
4. If not: `INSERT role='superadmin', added_at=NOW(), added_by=NULL`.

Invariants after this runs:

- Exactly one row with `role='superadmin'`.
- That row's `telegram_id` matches `SHELLBOTO_SUPERADMIN_ID`.
- The row is active (`disabled_at IS NULL`).

Changing the env var + restart = handoff. The previous
superadmin's row gets demoted to `admin`; they keep access but
not ownership.

## The `auth_reject` DoS defense

Without rate-limiting the *writing* of `auth_reject` rows, an
attacker spamming the bot could grow the audit DB rapidly.
Telegram's ~30 updates/sec/bot ceiling × 24h × 50 bytes/row ≈
130 MB/day per bot. Multiply by "an attacker who DMs from many
accounts they own" and the DB balloons.

Defense: `auth_reject_burst` / `auth_reject_refill_per_sec`
(defaults 5 + 0.05 = 1 row per 20s steady-state, keyed by
Telegram From-id). A single attacker writes ~4300 rows/day
(~2 MB).

Disable at your peril. The config allows it (`auth_reject_burst=0`)
but anything exposed to the public internet (and Telegram bots
always are, by design) should keep this on.

## Reading audit for the whitelist

```bash
shellboto users list
# or
shellboto users tree
# or
sqlite3 /var/lib/shellboto/state.db 'SELECT telegram_id, role, disabled_at FROM users;'
```

From Telegram (admin+):

```
/users
```

## Read next

- [audit-chain.md](audit-chain.md) — the hash chain that protects
  these decisions after they're written.
- [../configuration/roles.md](../configuration/roles.md) — the
  config-lens version of the same capability matrix.
