# Bot doesn't reply on Telegram

## 1. Is the service running?

```bash
sudo systemctl status shellboto
```

- `active (running)` → continue.
- `inactive` / `failed` → start it: `sudo systemctl start shellboto`.
  Look at journalctl for why it's not running.

## 2. Is it talking to Telegram?

```bash
sudo journalctl -u shellboto -n 50 --no-pager
```

Look for:

- `unauthorized` / `401` → token revoked or wrong. See
  [../runbooks/token-leaked.md](../runbooks/token-leaked.md) for
  rotation; for "wrong token" specifically: re-check
  `/etc/shellboto/env`.
- `network unreachable` / `connection refused` → outbound HTTPS
  blocked. Test: `curl -v https://api.telegram.org`. If that
  fails, fix VPS networking or firewall.
- Nothing recent → bot might be wedged. `sudo systemctl restart
  shellboto`.

## 3. Are you whitelisted?

Pre-auth rejection is silent — the bot doesn't tell you you're
not on the whitelist. Check:

```bash
sudo shellboto users list | grep <your_telegram_id>
```

If you're not there, you weren't whitelisted. Either:

- This is the first install — make sure
  `SHELLBOTO_SUPERADMIN_ID` matches your Telegram ID + restart.
- Someone removed you. Talk to whoever has admin access.

## 4. Are you rate-limited?

Possible if you spammed:

```bash
sudo journalctl -u shellboto --since "5 minutes ago" --output=cat \
    | jq -c 'select(.msg | test("rate"))'
```

Wait a minute, retry.

## 5. Is the bot blocked on your end?

In Telegram Settings → Privacy → Blocked Users, make sure the
bot isn't there.

## 6. Is the bot username right?

Type `t.me/<username>` and confirm it opens the right bot. If
you've created multiple bots at @BotFather, you might be DMing
the wrong one.

## 7. Wait a moment

Cold-start latency: first message after a restart can take a few
seconds (long-poll cycle). Wait 10s, retry.

## 8. Check audit for your message

```bash
sudo shellboto audit search --user <your_id> --since 5m
```

If you see your `auth_reject` rows, the bot saw your message but
blocked on auth (you're not whitelisted). If nothing at all, your
message never reached the bot — Telegram-side issue or network.

## 9. Test from a known-good account

Send `/start` from an account known to be on the whitelist (e.g.
the superadmin's). If that works, the issue is account-specific
(rate limit, removed from whitelist).

If that ALSO doesn't work, the bot is wedged or the network's
broken.

## 10. Restart the service

```bash
sudo systemctl restart shellboto
sudo shellboto doctor
```

If doctor's all green and bot still doesn't talk: dig into
journalctl with `--priority=err` since the restart.

## Read next

- [common-errors.md](common-errors.md) — error-message table.
- [../runbooks/token-leaked.md](../runbooks/token-leaked.md) —
  if the issue is auth-side.
