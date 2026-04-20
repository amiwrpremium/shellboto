# Quickstart

End-to-end in ~10 minutes. Get a working bot on a fresh VPS.
Everything below is copy-pasteable.

## 1. Create the Telegram bot

On Telegram, message [@BotFather](https://t.me/BotFather):

```
/newbot
```

BotFather asks for a display name (e.g. `My Shell Bot`) then a
username ending in `bot` (e.g. `myshellbot_bot`). When it succeeds,
it gives you a token that looks like:

```
123456789:AAHvZkSomeReallyLongBase64URLBlob-xxxx
```

**Copy this token. Treat it like a password.** If you paste it into
chat by accident, revoke it immediately (`/token` at @BotFather,
pick the bot, regenerate).

Full walkthrough: [create-telegram-bot.md](create-telegram-bot.md).

## 2. Find your Telegram user ID

Message [@userinfobot](https://t.me/userinfobot) — it replies with a
short profile including your numeric `Id`. That's a 9–10-digit number.
Copy it.

Other methods: [find-user-id.md](find-user-id.md).

## 3. SSH to the VPS, clone, build

```bash
# Prereqs: Go 1.26+, git, make
ssh root@your-vps
git clone https://github.com/amiwrpremium/shellboto.git
cd shellboto
```

## 4. Run the installer

```bash
sudo ./deploy/install.sh
```

The installer:

- Prompts you for the **bot token** (hidden input — nothing echoes).
- Prompts you for your **Telegram user ID** (that's who gets
  superadmin).
- Asks which **config format** you want: TOML / YAML / JSON. Pick
  TOML unless you have a reason.
- Auto-generates a **32-byte audit seed** (for the tamper-evident
  audit chain; see [../security/audit-chain.md](../security/audit-chain.md)).
- Builds `bin/shellboto`, installs it to `/usr/local/bin/shellboto`.
- Writes `/etc/shellboto/env` (mode 0600) and
  `/etc/shellboto/config.toml` (mode 0600).
- Installs the systemd unit, `daemon-reload`s, `enable --now`s.
- Runs `shellboto doctor` before exiting so you see a green
  preflight.

If any step fails, the installer rolls itself back. Re-run after
fixing the problem.

More detail, flags, and non-interactive mode: [installation.md](installation.md).

## 5. Say hi

Open Telegram, find your bot (you set the username in step 1), and
send:

```
/start
```

The bot should reply with a greeting. If it doesn't, check
`sudo journalctl -u shellboto -n 50`.

## 6. Run your first command

```
hostname
```

You should see your VPS hostname stream back as a single edited
message. Try:

```
uname -a
df -h
ps auxf | head -20
```

All of these work. If you try `vim` or `top`, it'll look garbled —
those are interactive terminal UIs and Telegram isn't a terminal.

## 7. Check the audit log

Back on the VPS:

```bash
sudo shellboto audit search --limit 5
```

Every command you sent is there, with its exit code, duration, and
captured output size.

## 8. Add another user (optional)

In Telegram, from your (superadmin) account:

```
/adduser <their_telegram_id>
```

They're added as `role=user` by default. To make them an admin
(can run `/adduser`, see everyone's audit log):

```
/role <their_telegram_id> admin
```

Superadmin only. See [../configuration/roles.md](../configuration/roles.md)
for the full capability matrix.

## 9. Learn the rest

- [first-commands.md](first-commands.md) — the interesting ones:
  `/status`, `/cancel`, `/kill`, `/audit`, `/get`, file uploads.
- [../security/](../security/) — danger-matcher, audit chain,
  threat model. **Read this before inviting anyone else.**
- [../operations/](../operations/) — ongoing ops once it's running.

## Uninstall

```bash
sudo ./deploy/uninstall.sh
```

Keeps config + audit DB by default. Pass `--remove-config` and/or
`--remove-state` to wipe them too (audit DB deletion requires typing
a confirmation phrase).

Rollback the binary to the previous version:

```bash
sudo ./deploy/rollback.sh
```
