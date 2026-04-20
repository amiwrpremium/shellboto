# Getting started

Five documents take you from "I have a VPS" to "the bot obeys me."

| Step | Doc | Time |
|------|-----|------|
| 1 | [quickstart.md](quickstart.md) — the compressed end-to-end | ~10 min |
| 2 | [create-telegram-bot.md](create-telegram-bot.md) — register with @BotFather | ~3 min |
| 3 | [find-user-id.md](find-user-id.md) — discover your numeric Telegram ID | ~1 min |
| 4 | [installation.md](installation.md) — interactive + non-interactive installer | ~5 min |
| 5 | [first-commands.md](first-commands.md) — `/start`, `/status`, send a command | ~2 min |

If you know what you're doing, read only [quickstart.md](quickstart.md)
— it's self-contained. If you want detail on each step, open the
others as you go.

## What you need

- A Linux VPS you control (any distro with systemd; Ubuntu 22.04+,
  Debian 12+, Fedora 38+, Arch all tested).
- Root or sudo access.
- Go 1.26+ **if** you're building from source. Installer will build
  for you.
- A Telegram account (yours).
- About 10 minutes.

## What you don't need

- A public IP (bot connects *out* to Telegram, not in).
- A reverse proxy, TLS certificate, or open port.
- A domain name.
- Any cloud-provider secrets.

## Security prerequisites before you start

1. **Enable 2FA on your Telegram account.** Settings → Privacy and
   Security → Two-Step Verification. Without this, anyone who SIM-
   swaps you gets root on your VPS. No exceptions — if you're not
   willing to do this step, do not install shellboto.
2. **Decide whether admins should be you-only or a small team.** The
   whitelist model assumes the superadmin vets every person. A team
   of 3–5 is fine; 20 is asking for incidents.
3. **Pick your `user`-role strategy.** If shells are for you (root)
   only, default config is fine. If non-admins are going on the
   whitelist, read [../configuration/non-root-shells.md](../configuration/non-root-shells.md)
   *before* adding them.

Proceed to [quickstart.md](quickstart.md).
