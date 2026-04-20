# Find a Telegram user ID

shellboto's whitelist is keyed by numeric Telegram user IDs (64-bit
integers, typically 9–10 digits for personal accounts). Display
names and @usernames aren't used — they're mutable, they can be
spoofed, and they collide. IDs don't.

## Method 1 — @userinfobot (easiest, 10 seconds)

1. In Telegram, search for `@userinfobot`.
2. Tap **Start**.
3. The bot replies with your profile:

   ```
   Id: 123456789
   First: Alice
   Last: Example
   Username: @alice_example
   ```

4. Copy the `Id` value. That's the number shellboto needs.

To get *someone else's* ID via @userinfobot:

1. They forward a message from themselves to @userinfobot (most
   reliable — works even if they have a privacy-restricted profile).
2. `@userinfobot` replies with "Forwarded from..." and their Id.

## Method 2 — Ask them to message your shellboto bot

This is the operator's path to whitelisting someone new who you're
not sharing a chat with:

1. Tell the candidate the bot's username (`t.me/<yourbot>`).
2. Have them send any message.
3. On the VPS:

   ```bash
   sudo journalctl -u shellboto -n 50 --no-pager | grep auth_reject
   ```

   Every rejected (non-whitelisted) message lands in `audit_events`
   as kind `auth_reject` **and** in the system journal. You'll see
   something like:

   ```
   {"level":"info","msg":"audit","kind":"auth_reject","user_id":987654321,"cmd":"/start"}
   ```

4. Copy `user_id`. That's their Telegram ID.

   Or via the DB:

   ```bash
   sudo shellboto audit search --kind auth_reject --limit 5
   ```

## Method 3 — `@RawDataBot` for group context

If the candidate is in a Telegram group with you but won't DM your
shellboto bot, add `@RawDataBot` to that group temporarily. It
emits raw update payloads including `from.id` for each message.

Remove the bot after you've noted the ID; @RawDataBot sees every
message in any group it's in.

## Method 4 — Forwarding

Ask them to forward one of their own messages to you. Tap the forward
header ("Forwarded from Alice") in a Telegram Desktop client — the
profile opens, and in some clients you can see the ID.

**This is unreliable** because of Telegram's forwarding privacy
settings — users can forward with "hide my account" enabled, which
suppresses the ID. Fall back to method 1 or 2.

## What you should *not* do

- **Don't use `@username` as an identifier.** Usernames are mutable
  (the user can change theirs any day, and someone else can grab
  it). Numeric IDs are immutable for the life of the account.
- **Don't type IDs from memory or transcription.** One digit off is
  a different Telegram account. Copy-paste or scan it; if typing,
  double-check against a fresh @userinfobot query.
- **Don't share IDs you don't need to.** IDs are not themselves
  secrets, but they enumerate account existence. Don't dump the
  whitelist in a public channel.

## Validation shellboto applies

When you `/adduser <id>` or seed `SHELLBOTO_SUPERADMIN_ID`:

- Must parse as a positive 64-bit integer.
- Zero, negative, or non-numeric → rejected with a clear error.
- A 20-digit number is accepted; Telegram IDs will grow past the
  current ~10-digit range as Telegram scales.

Proceed to [installation.md](installation.md).
