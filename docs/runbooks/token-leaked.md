# Telegram bot token leaked

**Symptom**: you see unexpected chats, messages, or
`audit_events` rows you didn't authorize. The token is in the
wrong hands. Or you accidentally pushed it to a public repo (the
gitleaks pre-commit hook should have caught it; if it didn't,
read this anyway).

## 1. Revoke + rotate + restart — immediately

```bash
# 1. At @BotFather (on Telegram), /token then select your bot —
#    it generates a new token and immediately revokes the old one.

# 2. Update the server env file.
sudo vi /etc/shellboto/env     # replace SHELLBOTO_TOKEN=<new>

# 3. Restart the service.
sudo systemctl restart shellboto
sudo systemctl status shellboto
shellboto doctor               # should stay green
```

Old token is dead the moment BotFather gives you the new one. No
ambiguous half-revoked window.

## 2. Forensics

```bash
# What did the leaked token do? Scan the window between plausible
# leak time and revocation.
shellboto audit search --kind command_run --since 168h | less

# Focus on specific users who acted in the window.
shellboto audit search --user <telegram-id> --since 168h

# Export to JSON for offline analysis.
shellboto audit export --format json --since 168h > /tmp/leak-audit.jsonl
```

What you're looking for:

- Commands you didn't run.
- Sessions from telegram_ids you don't recognise.
- `auth_reject` rows from before-leak times that suddenly become
  successful (someone exfiltrated the whitelist? unlikely via
  token alone, but check).

## 3. Treat the VPS as potentially compromised

If forensics reveals anything you didn't do:

1. Snapshot the DB for chain-of-custody:

   ```bash
   sudo shellboto db backup /var/backups/shellboto/incident-$(date +%s).db
   ```

2. Snapshot the journald entries:

   ```bash
   sudo journalctl -u shellboto --since "30 days ago" > /var/backups/shellboto/incident-journal-$(date +%s).log
   ```

3. Investigate the broader VPS:

   ```bash
   # Recently modified system files
   sudo find /etc /usr -newer /var/backups/shellboto/incident-*.db -type f

   # New cron jobs
   sudo grep -r '^[^#]' /etc/cron.* /var/spool/cron/

   # New systemd units
   sudo find /etc/systemd /lib/systemd -name '*.service' -newer /var/backups/shellboto/incident-*.db

   # Authorized keys changes
   sudo find / -name authorized_keys -newer /var/backups/shellboto/incident-*.db 2>/dev/null
   ```

4. If anything suspicious surfaces, restore the VPS from a
   pre-incident snapshot or rebuild from scratch. The cost of
   trusting a possibly-backdoored host is much higher than the
   cost of a rebuild.

## 4. Find the leak source

How did the token get out?

- **Git commit**: `git log --all -p | grep -i 'SHELLBOTO_TOKEN\|123456789:'`.
- **Backup file accidentally world-readable**:
  `find / -name 'env*' 2>/dev/null | xargs ls -la`.
- **Process listing**: `ps aux | grep shellboto`. Was the token
  ever passed as a flag instead of via env file? (Shouldn't be;
  shellboto only reads from env.)
- **Logs**: any place you may have echoed it.

Fix the source. Audit your git history for the old token even
post-revoke — if it's there, scrub via `git filter-repo`.

## 5. Why the gitleaks hook didn't catch it (if applicable)

`.gitleaks.toml` ships a Telegram-token rule. If a real token
slipped past:

- Was the commit pushed without `--no-verify`? Check.
- Was the regex shape unusual (token with non-standard tail
  characters)? Tighten the regex.
- File the gap as a security issue.

## 6. Tighten going forward

- 2FA on Telegram (your account, every admin's account).
- Rotate the token periodically (every 90 days, or after every
  team change involving admin churn).
- Configure your `.gitignore` so even local dev never accidentally
  commits an env file.
- Don't log the token anywhere — shellboto's zap setup is
  careful about this; downstream tooling might not be.

## What about audit-seed leaks?

Different runbook. The seed alone isn't useful without DB write
access; it's the second factor of the chain-tampering attack. If
both leaked: see
[audit-chain-broken.md](audit-chain-broken.md) and consider
seed rotation per [../security/audit-seed.md](../security/audit-seed.md).

## Read next

- [audit-chain-broken.md](audit-chain-broken.md) — if you suspect
  audit tampering.
- [../security/threat-model.md](../security/threat-model.md) —
  the full posture.
