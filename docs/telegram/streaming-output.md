# Streaming output to Telegram

How command output reaches your phone while bash is still running.

Source: [`internal/stream/stream.go`](../../internal/stream/stream.go).

## The edit loop

On command start, the streamer:

1. Sends an initial placeholder message:
   ```
   <pre>ls -la</pre>
   (running...)
   [ Cancel ]  [ Kill ]
   ```
2. Caches the returned `message_id`.
3. Enters a ticker loop driven by `edit_interval` (default 1s).
4. On each tick, snapshots the Job buffer:
   - If there's new output, renders `<pre>` + body + live-trailer,
     calls `EditMessageText`.
   - If no change since last edit, skips the API call (debounce).
5. Exits when `Job.Done` closes, doing one final flush with the
   exit-code footer.

## Debounce

The text is compared byte-for-byte with the last edit. Identical
text → no API call. Prevents hammering Telegram when output is
idle.

## Heartbeat

If output hasn't changed in `heartbeat` seconds (default 30s),
the trailer becomes `running... still alive (30s)` — visual cue
the bot isn't frozen. The actual text of the trailer IS new, so
the edit fires. This is the only reliable way to distinguish
"shell working silently" from "bot wedged."

## Hitting the 4096-char cap

Telegram's Bot API caps messages at 4096 UTF-16 code units. Counted
post HTML-escape (`<` → `&lt;` is 4 chars not 1). shellboto treats
the effective cap as 4036 to leave room for wrapper tags + footer.

When an edit would exceed the cap:

1. `pickBreak()` finds the largest prefix of pending output that:
   - Fits in the remaining room, and
   - Ends at a `\n` (fallback: space; fallback: hard cut at cap).
2. Current message is finalised with that prefix + a "continued →"
   trailer; keyboard is stripped.
3. `SendMessage` creates a new message with the next prefix.
4. Streaming continues on the new message.

User sees a series of messages, each ≤ 4036 chars of content.

## The `output.txt` spill

If the command produced enough output that the streamer had to
create more than one message, the streamer also uploads the FULL
captured output as an attached file:

```
output-1713632840.txt  (47.3 KB)
```

Named with a unix timestamp so multiple commands don't collide.
Limited by Telegram's bot-API 50 MB cap. If output exceeds 50 MB,
the file upload is skipped; user sees only the streamed messages
(up to `max_output_bytes`, default 50 MiB anyway — tighter than
upload cap).

Single-message outputs don't get the file — no benefit, just
noise.

## HTML escape handling

Output is wrapped in `<pre>...</pre>` so bash's ASCII art (tables,
tree output) renders in monospace on every Telegram client.

Inside `<pre>`, we HTML-escape:
- `&` → `&amp;`
- `<` → `&lt;`
- `>` → `&gt;`

This is why the effective payload cap is below the raw 4096 —
each escape is 4–5 chars instead of 1. An output of 3900 bytes of
`>` characters alone would balloon to 3900 × 4 = 15600 HTML chars
and force rollover. In practice `<` / `>` / `&` density is low
enough that the effective cap handles real output fine.

## What's NOT in the streamed text

- **ANSI escape codes.** Colour output, cursor codes, BEL — all
  stripped by `redact.StripTerminalEscapes` before storage AND
  before streaming. No terminal gibberish reaches the user.
- **Secrets.** `redact.Redact` runs over the full buffer before
  each flush. AWS keys, JWTs, private keys, `TOKEN=...` shapes —
  replaced with placeholder text in-stream, matching what ends up
  in the audit blob.

## Exit-code footer

Final flush appends:

```
✅ exit 0 · 42ms              # success
❌ exit 127 · 1.2s             # non-zero exit
⚠  interrupted · 3.5s          # SIGINT via /cancel
⚠  killed · 8.1s               # SIGKILL via /kill
⚠  timeout · 5m 2s             # default_timeout elapsed
⚠  output capped · 1m 5s       # max_output_bytes exceeded, SIGKILL fired
```

Duration rounded to 10 ms precision.

## "typing" indicator

While the streamer is active, it also sends `sendChatAction(typing)`
on a 4-second ticker (`typingRefresh` in the source). Keeps the
"... is typing" hint visible on the user's screen even when edits
are debouncing.

## Rate limit considerations

Telegram caps bots at ~30 messages/second (`SendMessage` +
`EditMessageText` combined, best-effort). With:

- 1s edit interval × 5 simultaneous users each running a noisy
  command → 5 edits/sec aggregate.

Well below the ceiling. If you expect 10+ concurrent noisy shells,
consider raising `edit_interval` to `2s` — the streaming feels
almost identical to users (commands tend to burst output or be
silent, rarely metered).

## What the user sees

Small command (fits in one message):

```
╭────────────────────────────────╮
│ ls -la /etc                    │
│ total 2108                     │
│ drwxr-xr-x 1 root root    4096 │
│ ...                            │
│ ✅ exit 0 · 14ms               │
│ [ Cancel ]  [ Kill ]           │  ← keyboard stripped on completion
╰────────────────────────────────╯
```

Large command (multi-message spill):

```
message 1 (4036 chars of output)
message 2 (4036 chars)
message 3 (partial + exit footer)
attached: output-<timestamp>.txt
```

## Reading the source

- `stream.go` — the whole thing. `Stream()`, `flush()`,
  `pickBreak()`, `effectiveHTMLLen()`.
- `stream_test.go` — table-driven tests for rollover + HTML
  escape accounting.

## Read next

- [file-transfer.md](file-transfer.md) — the `/get` + upload
  pipeline.
- [supernotify.md](supernotify.md) — the other outbound DM flow.
