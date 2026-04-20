# Telegram integration

How shellboto interacts with the Telegram Bot API. Covers the
user-facing command surface, inline-keyboard flows, streaming
output pipeline, superadmin fan-out, file transfer.

| File | What it covers |
|------|----------------|
| [commands.md](commands.md) | Every slash command by role, arguments, side effects |
| [callbacks-and-flows.md](callbacks-and-flows.md) | Inline-keyboard callbacks, multi-step flows, danger-confirm TTL, callback-data namespaces |
| [streaming-output.md](streaming-output.md) | The edit-loop message pipeline, 4096-char spill to output.txt |
| [supernotify.md](supernotify.md) | Admin fan-out DMs with inline quick-action buttons |
| [file-transfer.md](file-transfer.md) | `/get`, file uploads via attachment, 50 MB cap |

## Mental model

Every Telegram `update` (one message or one callback-button tap)
enters the same dispatch pipeline:

```
update → middleware (auth + ratelimit) → handler (commands or callbacks)
                                                │
                                                ├─ handler may Ask-the-shell → stream reply
                                                ├─ handler may Edit message (multi-step flow state machine)
                                                └─ handler may supernotify (fan-out to superadmin)
```

handlers never talk to the Telegram API directly. They go through
`deps.Deps` which bundles a typed `*bot.Bot`, the shell manager,
the stream helper, the audit repo, etc.

## Inline-keyboard strategy

shellboto uses inline keyboards heavily:

- Cancel / Kill buttons on every running-command message.
- Run / Cancel buttons on danger-confirm prompts.
- Promote / Demote / Ban / Profile buttons on `/users` entries.
- Confirm / Cancel in multi-step add/remove flows.

Every inline button carries `callback_data` starting with a
two-char prefix (`j:` for job, `us:` for users browser, etc.) so
the router can dispatch cheaply on the first 2–3 bytes without
parsing the whole payload.

The prefix registry is in `internal/telegram/namespaces/`.

## Read next

- [commands.md](commands.md) — the full command surface.
- [callbacks-and-flows.md](callbacks-and-flows.md) — how
  interactive flows work.
