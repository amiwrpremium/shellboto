# Supernotify — admin fan-out DMs

When a whitelist change happens, shellboto DMs the superadmin (and
sometimes the immediate promoter) with the event + quick-action
buttons.

Source: [`internal/telegram/supernotify/supernotify.go`](../../internal/telegram/supernotify/supernotify.go).

## What triggers a supernotify

Audit kinds that fan out:

- `user_added` — new whitelist entry.
- `user_removed` — soft-delete.
- `user_banned` — auto-ban from escalation or strict-ASCII.
- `role_changed` — promote or demote.

Not fanned out:

- Ordinary `command_run` — too noisy. Use `/audit` instead.
- `shell_spawn`, `shell_reset`, `shell_reaped` — not operator-
  relevant.
- `danger_*` — the confirming admin already knows; the
  superadmin can see these via `/audit` if they want.

## Who receives

For every fan-out event:

1. **The superadmin** — always DMed.
2. **The actor's immediate promoter** (if any, and not the
   superadmin themselves) — DMed.

So if Alice (promoted by Bob, who was promoted by the superadmin)
promotes Charlie to admin:

- Superadmin gets a DM.
- Alice (the actor) does not get a DM about her own action.
- Bob (Alice's promoter) does not get a DM — he's not the
  superadmin and he's one hop removed.

If the superadmin themselves performs an action: they don't DM
themselves.

## What the DM looks like

For `role_changed` from user to admin:

```
👑 superadmin-promoted alice (456) to admin

added at 2026-03-15
promoted_by: superadmin (123)
total users: 4 (1 superadmin, 2 admins, 1 user)

[ 👤 Demote ]  [ 🚫 Ban ]  [ ✨ Profile ]  [ ❌ Close ]
    dm:456       us:ban:456  us:profile:456  us:close
```

The inline buttons route to the same handlers as the `/users`
browser. Handy when you're reading the DM weeks later and want
to reverse a decision.

## Action TTL

Inline buttons on supernotify DMs have a TTL:
`super_notify_action_ttl` (default 10m).

After the TTL:

- A sweeper goroutine collects expired (message_id, chat_id)
  pairs.
- Calls `EditMessageReplyMarkup(reply_markup=nil)` on each to
  strip the keyboard.
- The event text remains — historical record preserved.

This prevents tapping a "Demote Alice" button from a DM that's
been sitting in your chat scroll for a week — by which point your
understanding of the team has changed and tapping was probably
unintentional.

Disable with `super_notify_action_ttl = 0`. Buttons persist
forever (until you tap or the server restart eventually discards
the sweeper state).

## Silent vs. unsilent

DMs are sent with `disable_notification = false` by default —
you'll get a sound / badge on your phone. shellboto doesn't
currently expose a quieter-supernotify config; if you're worried
about midnight pings, either:

- Set Telegram's per-chat mute on the bot, or
- Schedule a regular audit review and turn off supernotify
  entirely (would require a code change; not a config flag today).

## What supernotify doesn't do

- **No email / SMS / PagerDuty.** Telegram DM only.
- **No grouping.** Five back-to-back user changes produce five
  DMs, not a digest.
- **No retry on failure.** If sending fails (e.g. the operator
  has blocked the bot), the error is logged and the event is
  dropped. Audit row in the DB is the source of truth.

## Superadmin blocked the bot?

Unusual, but if the superadmin accidentally blocks the bot on
Telegram, supernotify DMs silently fail (Bot API returns 403).
Shellboto logs the error but keeps working — the event is still
audited.

Fix: unblock the bot in Telegram (Settings → Privacy → Blocked
Users → remove the bot).

## Code structure

- `Emitter.Emit(event)` — called from handlers after their own
  DB write + audit.
- `Emitter.pendingTTL map[int64]time.Time` — (message_id, expiry)
  pairs.
- `Emitter.sweepLoop(ctx)` — runs on a ticker, expires TTLs.

## Writing new supernotify events

1. Add the trigger in the handler that performs the action.
2. Call `deps.Super.Emit(...)` with a typed event struct.
3. Add a renderer in `supernotify/views.go` for the new event
   shape.
4. Add a test in `supernotify_test.go` asserting recipient list +
   rendered text + action-button TTL.

## Read next

- [callbacks-and-flows.md](callbacks-and-flows.md) — the action
  buttons use the same namespace router.
- [../operations/user-management.md](../operations/user-management.md)
  — the operator lens on these events.
