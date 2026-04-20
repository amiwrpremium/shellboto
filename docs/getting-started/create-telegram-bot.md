# Create the Telegram bot (step-by-step)

shellboto talks to Telegram through the Bot API. You don't run a
server — Telegram's infrastructure does. You just register a bot and
get a token.

## 1. Open a chat with @BotFather

- Open Telegram.
- In the search box, type `@BotFather`. Pick the **verified** one
  (blue check mark). There are copycats; only the verified
  @BotFather can issue real tokens.
- Tap **Start** (or send `/start`) if it's your first time.

## 2. Create the bot

Send `/newbot`.

BotFather will ask for:

1. **A display name** — shown in chat headers, e.g. `My Shell Bot`.
   Anything you like; not user-visible as an identifier.
2. **A username** — must end in `bot` (case-insensitive) and be
   globally unique across Telegram, e.g. `amiwr_shell_bot` or
   `my_shellboto_bot`. This is how users find your bot:
   `t.me/<username>`.

If the username is taken, BotFather will tell you and ask again.

## 3. The token

On success BotFather sends a message that contains a line like:

```
Use this token to access the HTTP API:
123456789:AAHvZkExampleBase64URLtoken-xxxxxxxxxxxxxxxxx
```

That 46-character string **is the bot**. Anyone with it can
impersonate the bot: read every message sent to it, reply on its
behalf, add admins, run any `/command` it exposes.

**Do not:**
- Paste it into group chats, commit it, log it, or screenshot it.
- Embed it in front-end code, Docker image layers, or public
  tarballs.

**Do:**
- Let the shellboto installer prompt for it (hidden input).
- Keep `/etc/shellboto/env` at mode 0600, root-owned.
- Rotate it if you suspect exposure: `/token` at BotFather → pick
  the bot → regenerate. Old token dies immediately.

## 4. Set the description (optional, cosmetic)

From the @BotFather chat:

- `/setdescription` — text shown above the **Start** button on first
  interaction.
- `/setabouttext` — text shown in the bot's profile.
- `/setuserpic` — upload an avatar.

None of this is required for shellboto to work.

## 5. Disable privacy mode if you want the bot to read group messages

By default, a bot added to a group only sees messages addressed to
it (`/cmd@yourbot`, replies, mentions). If you plan to use shellboto
**only in private chat** (recommended), leave this alone. If you plan
to use it in a group of admins, disable privacy mode:

- `/setprivacy` → pick the bot → **Disable**.

*Still* only whitelisted users can do anything; privacy mode is just
about whether messages arrive at the bot at all.

## 6. Configure allowed commands (cosmetic, useful)

So Telegram shows tab-completion + the `/` keyboard on mobile:

- `/setcommands` → pick the bot → paste:

```
start - start or re-register with the bot
help - list available commands
status - show current shell status
cancel - send Ctrl+C to the current command
kill - SIGKILL the current command
reset - respawn the shell
auditme - show my recent audit events
get - download a file from the VPS
users - admin+ — list whitelisted users
adduser - admin+ — whitelist a user
deluser - admin+ — remove a user from the whitelist
role - superadmin — change a user's role
audit - admin+ — show recent audit events
audit-verify - admin+ — walk the audit hash chain
```

This is purely UX; the bot accepts whatever commands its code knows
regardless of what you set here.

## 7. Secure the BotFather chat itself

Your Telegram account with 2FA on is the root of trust. If someone
gets into your Telegram, they can ask BotFather to regenerate the
token. Defense:

- Telegram → Settings → Privacy and Security → Two-Step Verification
  → set a password + a recovery email you actually control.
- Enable passcode lock on the device running Telegram.
- Use the official Telegram app (or the official Telegram Web) —
  third-party clients are a supply-chain risk for credentials.

## Tokens look like this

```
<numeric-bot-id>:AA<33+ base64url chars>
```

- `<numeric-bot-id>` — 6–12 digit number, public (visible in bot
  profiles). Not a secret.
- `AA…` — the secret. 33+ chars of `[A-Za-z0-9_-]`.

shellboto's danger-matcher and gitleaks config know this shape and
will flag real tokens in commits automatically.

## What shellboto does with the token

1. Reads it from the `SHELLBOTO_TOKEN` environment variable at
   startup.
2. Uses it only to authenticate with Telegram's Bot API over HTTPS.
3. **Strips it from the spawned bash's environment** so
   `printenv SHELLBOTO_TOKEN` from inside a user-role shell returns
   nothing. See [../shell/user-shells.md](../shell/user-shells.md).
4. Never logs it (zap's field names are controlled; the token never
   appears in structured log fields).

Proceed to [find-user-id.md](find-user-id.md).
