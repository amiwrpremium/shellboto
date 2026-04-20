# User management

Day-to-day adding, removing, promoting, demoting.

## From Telegram (admin+)

### Add

```
/adduser <telegram_id>
```

Opens a wizard:

1. Bot validates the ID via `GetChat` (catches typos).
2. Prompts for a friendly name. Validation: matches
   `^[A-Za-z]+(?: [A-Za-z]+)*$`. Bad name → auto-reject.
3. Inline Confirm / Cancel.
4. On confirm: row inserted with `role=user`, `added_by=<you>`,
   `added_at=now`.

Supernotify DMs the superadmin. Audit `kind=user_added`.

To add as admin in one step (**superadmin only**):

```
/adduser <telegram_id> admin
```

Same flow; role=admin at end.

### Remove

```
/deluser <telegram_id>
```

Inline Confirm. On confirm:

- Soft-delete: `disabled_at=now`.
- Audit `kind=user_removed`.
- Their active pty shell is auto-closed.
- Supernotify DMs.

Admins can only remove `role=user`. Superadmin can remove any
non-superadmin.

### Promote / Demote

Superadmin only:

```
/role <telegram_id> admin     # user → admin
/role <telegram_id> user      # admin → user
```

Audit `kind=role_changed`. Target shell auto-closed so new privs
kick in on next message.

Admins can demote only admins they promoted (via `promoted_by`).
Expressed through the `/users` browser's Demote button, which
does the RBAC check.

### List

```
/users
```

Opens the inline browser. Tap a user to see details + quick
actions.

## From the host CLI

More powerful / scriptable:

```bash
sudo shellboto users list            # table: ID, role, status, name, username, added_at, added_by, promoted_by
sudo shellboto users tree            # ASCII tree of promotion lineage
```

Output:

```
TELEGRAM_ID  ROLE        STATUS    NAME          USERNAME      ADDED_AT             ADDED_BY  PROMOTED_BY
123456789    superadmin  active    amiwr         @amiwrpremium 2026-03-01T00:00:00Z —         —
987654321    admin       active    Alice         @alice         2026-03-15T10:22:11Z 123456789 123456789
456789123    user        active    Bob           @bob_dev       2026-04-01T08:00:00Z 987654321 —
```

Tree view:

```
👑 amiwr (123456789)
├── 🛡 Alice (987654321)
│   └── 👤 Bob (456789123)  [added by Alice]
└── 👤 Charlie (789012345)  [disabled 2026-04-15]
```

## Raw DB access

```bash
sudo sqlite3 /var/lib/shellboto/state.db
```

```sql
SELECT telegram_id, role, disabled_at FROM users;
```

Useful for ad-hoc queries but don't `UPDATE` directly — you'd
bypass audit events. Use Telegram commands or `shellboto users`
instead.

## Common tasks

### Weekly review

```bash
sudo shellboto users list
```

Remove anyone who shouldn't be there. Ask yourself for each row:

- Do they still need access?
- Have they messaged the bot recently?
- Is their role still appropriate?

Err on the side of `/deluser`. They can always be re-added.

### Find who's been inactive

```bash
sudo sqlite3 /var/lib/shellboto/state.db <<SQL
SELECT u.telegram_id, u.name, u.role,
       MAX(ae.ts) AS last_event
FROM users u
LEFT JOIN audit_events ae ON ae.user_id = u.telegram_id
WHERE u.disabled_at IS NULL
GROUP BY u.telegram_id
ORDER BY last_event ASC;
SQL
```

Rows at the top: oldest activity. Consider removing them.

### Audit recent role changes

```bash
sudo shellboto audit search --kind role_changed --since 720h
```

720h = 30 days. Shows every promote / demote event.

### Confirm someone's still able to act

```bash
sudo shellboto users list | grep <their_id>
```

Or `/users` from a superadmin/admin account.

## Supernotify context

Every change fans out DM notifications:

- The superadmin.
- The target user's immediate promoter (if not the superadmin).

If your team is big enough that supernotify DMs become noisy,
consider whether all those changes are strictly necessary. Noise
reduction: tighter whitelist, stop adding transient users.

## Related

- [../configuration/roles.md](../configuration/roles.md) — full
  RBAC matrix.
- [../security/whitelist-and-rbac.md](../security/whitelist-and-rbac.md)
  — the security lens.
- [../telegram/callbacks-and-flows.md](../telegram/callbacks-and-flows.md)
  — how the inline-keyboard flows work.

## Read next

- [updating.md](updating.md) — when the bot version changes.
