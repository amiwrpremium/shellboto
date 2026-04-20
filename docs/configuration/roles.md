# Roles and capabilities

shellboto has three roles. Most people need to understand only two:
superadmin (you) and user (everyone else).

## The three roles

| Role | How you get it | How many |
|------|----------------|----------|
| `superadmin` | `SHELLBOTO_SUPERADMIN_ID` env var + restart | Exactly 1 |
| `admin` | `/role <id> admin` by superadmin, or `/adduser … admin` by superadmin | 0+ |
| `user` | `/adduser <id>` default | 0+ |

## Capability matrix

✅ = allowed, ❌ = not allowed, ✓ = allowed but limited to self,
— = not applicable.

| Action | user | admin | superadmin |
|--------|:----:|:-----:|:----------:|
| Send shell commands | ✅ | ✅ | ✅ |
| `/cancel`, `/kill`, `/reset`, `/status` | ✅ | ✅ | ✅ |
| `/get <path>` (download file) | ✅ | ✅ | ✅ |
| Upload file (attachment) | ✅ | ✅ | ✅ |
| `/auditme` (own audit history) | ✅ | ✅ | ✅ |
| `/help`, `/start` | ✅ | ✅ | ✅ |
| `/users` (list whitelist) | ❌ | ✅ | ✅ |
| `/adduser <id> [user]` | ❌ | ✅ | ✅ |
| `/adduser <id> admin` | ❌ | ❌ | ✅ |
| `/deluser <id>` (users only) | ❌ | ✅ | ✅ |
| `/deluser <id>` (admins) | ❌ | ❌ | ✅ |
| `/audit [N]` (everyone's audit) | ❌ | ✅ | ✅ |
| `/audit-out <id>` (fetch output) | ❌ | ✅ | ✅ |
| `/audit-verify` (run chain walker) | ❌ | ✅ | ✅ |
| `/role <id> admin` (promote user) | ❌ | ❌* | ✅ |
| `/role <id> user` (demote admin) | ❌ | ❌** | ✅ |

\* *Admins cannot promote other users directly. Only superadmin promotes.*
\** *Admins can demote only admins they themselves promoted (promoted_by tracking). Superadmin can demote any admin.*

## Defaults

- New `/adduser <id>` → `role=user`.
- Shell for role=user runs as `user_shell_user` if that config key
  is set; otherwise root (with a startup warning). See
  [non-root-shells.md](non-root-shells.md).
- Shell for role=admin and role=superadmin always runs as root.

## RBAC rules in full

From `internal/telegram/rbac/rbac.go`:

### `CanActOnLifecycle(actor, target)`

Controls whether `/deluser` (and similar lifecycle ops) is allowed.

- Superadmin → can act on anyone except themselves (can't delete
  the superadmin; they'd be auto-reseeded on next restart anyway,
  but the UI rejects it).
- Admin → can act only on `role=user`.
- User → can act on no one.

### `CanPromote(actor, target)`

Controls `/role <id> admin` + the `✅ Promote` button in the
user-management flow.

- Actor must be admin+.
- Target must be `role=user` and active (`disabled_at IS NULL`).

In practice only superadmin exercises this, because the UI for
admin-level promotion is hidden; the RBAC enforcement is there in
case a bug or future feature tries to call it.

### `CanDemote(actor, target)`

- Target must be `role=admin` and active.
- Superadmin → may demote any active admin.
- Admin → may demote **only admins they themselves promoted**
  (checks `users.promoted_by == actor.telegram_id`).

The second rule is why every promotion is recorded with a
`promoted_by` column — it's not decorative.

## Promotion lineage

When admin Alice promotes Bob, the `users` table records:

```
telegram_id = bob
role = admin
promoted_by = alice.telegram_id
```

Alice can later `/role bob user` (demotes). Admin Charlie cannot
demote Bob, because Charlie didn't promote him. Only Alice or
superadmin can.

This is visible in `shellboto users tree` (ASCII tree rendering of
the lineage):

```
👑 superadmin (123)
├── 🛡 admin-by-superadmin (234)
├── 🛡 alice (456)
│   └── 🛡 bob (promoted by alice) (678)
└── 👤 eve (added by alice, role=user) (789)
```

## Banning (soft-delete)

`/deluser <id>` doesn't DROP the row — it sets `disabled_at = now()`.
The row stays for audit continuity (`/auditme` for a deleted user
still works if you query by ID).

- Banned users can still message the bot; every message generates
  a `kind=auth_reject` audit row (rate-limited by
  `auth_reject_burst`).
- Banning does not remove them from historical audit rows.
- Un-banning = another `/adduser <id>` from an admin+. The row is
  re-activated with the new role.

## What happens on promote/demote

After the RBAC transition commits:

1. Audit row `kind=role_changed` with `detail` containing old/new
   roles.
2. Supernotify fires: the superadmin and the immediate promoter
   (if any) get a DM with the event + an inline keyboard with
   quick actions (demote, ban, profile).
3. **The subject's active pty shell is auto-closed.** Their next
   message spawns a new shell with the updated uid/gid/home.
   Prevents a demoted admin from keeping their still-open root
   shell post-demotion.

## What `role_changed` events look like

From the audit log:

```
$ shellboto audit search --kind role_changed --limit 3
ID        TS                         USER         KIND           EXIT    BYTES   CMD
1234      2026-04-20 15:32:11.014Z   alice(456)   role_changed   —       —       bob(678): user → admin
1235      2026-04-21 09:15:42.821Z   alice(456)   role_changed   —       —       bob(678): admin → user
```

## Designing a team

Practical patterns:

- **Solo operator.** You are superadmin. Nobody else. Simplest; no
  capability matrix to think about.
- **You + one trusted ops person.** Promote them to admin. They
  can add/remove `role=user` people but can't demote you (admins
  can only demote admins they promoted).
- **You + small team.** You're superadmin; one admin handles
  day-to-day user management; other admins handle their own
  direct reports. Use the `promoted_by` lineage to map the team.
- **Shared ownership.** Not possible by design. Only one
  superadmin. Change the env + restart to hand off.

## Read next

- [non-root-shells.md](non-root-shells.md) — how to actually make
  `role=user` run as a non-root account.
- [../security/whitelist-and-rbac.md](../security/whitelist-and-rbac.md)
  — security-lens treatment of the same topic.
