# Callbacks and multi-step flows

Inline keyboards make shellboto feel like a desktop app. This doc
covers how those work.

## Callback-data namespaces

Every inline button carries a `callback_data` string ≤ 64 bytes.
shellboto prefixes each with a two-char namespace so the router
can dispatch cheaply:

| Prefix | Namespace | Handler |
|--------|-----------|---------|
| `j:` | **Job** controls on a running command | Cancel (`j:c`) / Kill (`j:k`) |
| `us:` | **Users** browser + per-user actions | Open profile, page through list |
| `pr:` | **Promote** flow | Confirm promotion of a specific user |
| `dm:` | **Demote** flow | Confirm demotion |
| `ad:` | **Add-user** wizard | Confirm / Cancel steps |
| `dg:` | **Danger-confirm** | Run / Cancel a matched-pattern command |
| `rg:` | **Register** flow | Confirmation during superadmin registration |

Defined in `internal/telegram/namespaces/namespaces.go`. Prefix
sets are kept narrow + well-documented so new prefixes don't
collide silently.

## Example: the running-command keyboard

When a command starts, `stream.Stream` attaches an inline keyboard
with the payload:

```
[ Cancel ]  [ Kill ]
  j:c         j:k
```

On tap, Telegram sends a `callback_query` update. The router
sees `j:c` → dispatches to `callbacks/job.go`'s cancel handler,
which is the same code path as the `/cancel` text command.

Result: tapping Cancel is exactly equivalent to sending `/cancel`
— same audit kind, same rate-limit exemption, same shell action.

## Multi-step flows

Some user actions span multiple back-and-forth messages. The
add-user wizard is the canonical example:

```
user: /adduser 123456789
bot:  Looks up user 123456789 via GetChat...
      → does it resolve? Y/N.
      [reply]

user: <text reply: friendly name>
bot:  Validates name regex, stores pending state.
      Asks for confirmation with inline buttons.
      [ ✅ Confirm ]  [ ❌ Cancel ]

user: taps ✅ Confirm (callback ad:confirm)
bot:  Inserts into users table, audit kind=user_added, supernotify.
      Keyboard stripped; message edited to "✅ added".
```

Between steps, the bot stores pending state in a `flows.Registry`
keyed by `(telegram_id, flow_name)`. Regular text messages from
that user are intercepted by the flow first; only if the flow
returns "not my message" does exec-handler take over.

## Flow state storage

**In-memory only.** The flow registry is a map with a mutex; it
doesn't persist to SQLite.

Consequence: if you restart the bot mid-wizard, any in-progress
flow state is lost. The user will see their next message treated
as an exec command instead of a wizard step. No big deal — they
restart the flow.

## Danger-confirm

Triggered when `danger.Match(cmd)` hits (see
[../security/danger-matcher.md](../security/danger-matcher.md)).

```
user: rm -rf /tmp/stale

bot:  ⚠ This command matched danger pattern:
        \brm\s+(-[a-zA-Z]*r[a-zA-Z]*f|-[a-zA-Z]*f[a-zA-Z]*r)\b

        rm -rf /tmp/stale

        Tap ✅ Run to execute, or let this prompt expire.
        [ ✅ Run ]  [ ❌ Cancel ]
                (dg:run:<token>)  (dg:cancel:<token>)
```

- `<token>` is an opaque short-lived identifier stored in the
  flow registry, mapped to the pending command string. Gives us
  two-factor protection: even if an attacker races to tap the
  button on a message that wasn't theirs, the token validates
  ownership.
- `confirm_ttl` (default 60s) caps validity. After TTL, the
  token is garbage-collected and tapping the button triggers
  `kind=danger_expired`.
- Audit events:
  - On match: `kind=danger_requested`.
  - On Run tap: `kind=danger_confirmed`, then normal
    `kind=command_run` as usual.
  - On Cancel tap or TTL expiry: `kind=danger_expired`.

## Users browser

`/users` opens:

```
shellboto users
----------------
👑 superadmin_name  (123)
🛡 admin_alice     (456)
🛡 admin_bob       (678)     ← tappable
👤 user_charlie    (789)
```

Each entry's inline button has `callback_data = us:open:<id>`.
Tapping shows a per-user detail panel:

```
profile: admin_bob (678)
role: admin
promoted_by: admin_alice (456)
added_at: 2026-03-15
last_seen: 2026-04-20 14:22 UTC

[ 👤 Demote ]  [ 🚫 Ban ]  [ ✨ Profile ]  [ ❌ Close ]
   dm:<id>      us:ban:<id>   us:profile:<id>  us:close
```

Buttons respect RBAC — e.g. an admin sees the `Ban` button only
for `role=user` targets, not for other admins.

## Supernotify action buttons

Event notifications to superadmin/promoter carry quick-action
buttons too (see [supernotify.md](supernotify.md)). These have the
same prefixes as the `/users` browser buttons — tapping "Demote"
on a notification DMed last week works exactly the same as using
the browser today, provided the token hasn't expired.

Supernotify action TTL is configurable via
`super_notify_action_ttl` (default 10m). After that, a sweeper
strips the keyboard from the old notification message so a stale
button can't be tapped — the message text stays as a historical
record.

## What callbacks can and can't do

Can:

- Run any DB write or pty action the equivalent text command can.
- Take the same RBAC checks as the text command.
- Fire audit events.
- Edit the message they were tapped on (common — the keyboard
  gets updated or removed).

Can't:

- Message arbitrary chats (they can only reply into the chat
  where the callback fired).
- Bypass rate limits (callbacks go through the same middleware).
- Access Telegram's full user profile without a corresponding
  API call.

## Writing a new callback

1. Pick / register a namespace prefix in `internal/telegram/namespaces/`.
2. Add a handler in `internal/telegram/callbacks/<thing>.go` with
   signature `func Handle(ctx, b, update, deps)`.
3. Register it in `internal/telegram/bot.go`'s
   `RegisterCallbacks(...)` call.
4. Add a builder in `internal/telegram/keyboards/` so other code
   can construct the button.
5. Add tests covering RBAC + flow state + happy path.

See `callbacks/adduser.go` + `flows/adduser.go` +
`commands/adduser.go` as a complete worked example.

## Read next

- [streaming-output.md](streaming-output.md) — how replies are
  rendered.
- [supernotify.md](supernotify.md) — admin fan-out.
