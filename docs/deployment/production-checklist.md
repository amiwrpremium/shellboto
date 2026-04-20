# Production checklist

Run through before announcing the bot to your team.

## Before the first install

- [ ] VPS is one you trust + control (not a free-tier shared box).
- [ ] Backups + snapshots are configured at the hypervisor level.
- [ ] SSH access is restricted (keys only, fail2ban, no root
      login by password).
- [ ] Disk is ≥ 10 GB free; audit + output storage will consume
      some over time.

## Telegram side

- [ ] Bot created with @BotFather under a username you control.
- [ ] **2FA enabled on your Telegram account.** Mandatory.
- [ ] `/setcommands` run at BotFather (see
      [../getting-started/create-telegram-bot.md](../getting-started/create-telegram-bot.md))
      — purely UX, but helpful.
- [ ] Bot's privacy mode matches your deployment (private-DM use
      ⇒ privacy ON; group use ⇒ OFF).

## Installer run

- [ ] `./deploy/install.sh` completes without errors.
- [ ] `shellboto doctor` is green at the end.
- [ ] `systemctl status shellboto` → `active (running)`.
- [ ] Bot replies to `/start` on your Telegram account.

## Security configuration

- [ ] `SHELLBOTO_AUDIT_SEED` is set (not the all-zeros fallback).
      Check with `shellboto doctor`.
- [ ] `/etc/shellboto/env` has mode 0600 and owner root:root.
- [ ] `/etc/shellboto/config.toml` has mode 0600.
- [ ] If any `role=user` callers will be on the whitelist:
  - [ ] `user_shell_user` is set in config.
  - [ ] The unprivileged unix account exists and has no sudoers
        entries.
  - [ ] `/home/shellboto-user/` parent dir is `root:<group> 0750`
        (not `user:user 0700`).
  - [ ] Spot-check: non-admin shell gets Permission denied on
        `/etc/shadow`, `/root/.ssh/`, etc.

## Operational hygiene

- [ ] `shellboto audit verify` scheduled (cron or systemd timer,
      ≥ every 6h).
- [ ] Alert path wired for verify failures (email / Slack / page).
- [ ] `shellboto db backup` scheduled daily.
- [ ] Backup destination is **offsite** (not just
      `/var/backups/` on the same disk).
- [ ] Backup retention matches your policy (default ~30 days is
      reasonable for solo; longer for compliance).
- [ ] `journalctl -u shellboto` retention matches needs
      (journald's default is ~2 weeks of log; raise via
      `/etc/systemd/journald.conf` if you need longer).

## Documentation

- [ ] You've read [../security/threat-model.md](../security/threat-model.md).
- [ ] You've read [../security/danger-matcher.md](../security/danger-matcher.md).
- [ ] You've read [../security/root-shell-implications.md](../security/root-shell-implications.md).
- [ ] You've skimmed [../runbooks/](../runbooks/) so you know
      where to look at 3 AM.
- [ ] You have a password-manager entry for the bot token (not
      for logging in — for rotating via @BotFather if it leaks).

## People

- [ ] Whitelist is minimal — just you, or you + 1–2 trusted
      co-admins.
- [ ] Your co-admins have 2FA on Telegram too.
- [ ] There's a note / document somewhere explaining the service
      to whoever inherits the VPS after you (successor doc).

## Final pre-flight

- [ ] Run a danger command deliberately (`rm -rf /tmp/foo`);
      confirm the danger-confirm flow works.
- [ ] Send a big command (`journalctl -n 1000`); confirm
      streaming + output.txt spill work.
- [ ] `/get /etc/shellboto/env` (as admin) → replies with file.
- [ ] Trigger a `/cancel` mid-command; confirm audit row reflects
      `termination=canceled`.
- [ ] Upload a tiny file via caption-path; confirm it lands at
      the right location.
- [ ] Verify audit chain one more time.

## Go-live

Announce to your team, add them via `/adduser`, send them the
bot's `@username`. Done.

## Post-deploy — monitor for a week

- Daily: skim journalctl for anything new.
- Weekly: review `shellboto users list` and
  `shellboto audit search --kind role_changed --since 168h`.
- Monthly: restore a backup into a test location and run
  `shellboto audit verify` against it.

## Read next

- [../operations/](../operations/) — the ongoing ops stuff.
- [../runbooks/](../runbooks/) — when something breaks.
