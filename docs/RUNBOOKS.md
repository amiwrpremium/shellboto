# Runbooks

Step-by-step procedures for incidents. Every tool referenced here
already exists — these docs just thread the steps together so a
half-awake operator at 3 a.m. doesn't have to think.

Tooling used across runbooks:

- `sudo ./deploy/rollback.sh` — atomic swap back to the previous binary.
- `shellboto doctor` — preflight / post-change sanity check.
- `shellboto audit verify` — hash-chain integrity walk.
- `shellboto audit replay` — cross-check DB against journald.
- `shellboto audit search` — filter audit events.
- `shellboto db backup <path>` — online SQLite snapshot.
- `systemctl` / `journalctl` — standard systemd ops.

---

## 1. A release is broken in production

**Symptom**: the service was healthy before the last release; now
it's crashing, looping, or misbehaving after the goreleaser-published
update.

### Roll back, then investigate

```bash
# 1. Confirm the bad state.
systemctl status shellboto
journalctl -u shellboto -n 100 --no-pager

# 2. Swap back to the previous binary. Atomic; reversible.
sudo ./deploy/rollback.sh

# 3. Confirm the service is healthy on the previous version.
systemctl status shellboto
shellboto doctor
```

### Yank the bad release on GitHub

Optional but helpful so new installers don't pick the broken
artifact:

1. github.com/amiwrpremium/shellboto/releases → edit the bad release.
2. Tick **"Set as a pre-release"** (or mark draft).
3. Leave a short note explaining why.

The tag itself isn't deleted — future `v*.*.*-prev-was-bad` fix
commits land normally; release-please opens the next release PR
once fixes are merged.

### Fix forward

1. Open a PR that reverts (or fixes) the offending commit.
2. Land it on `master`. release-please will open a new release PR.
3. Merge the release PR → new tag → goreleaser ships the fix.

`sudo ./deploy/rollback.sh` is reversible — re-run it on the host to
flip back to the new binary once you've verified the fix.

---

## 2. Telegram bot token leaked

**Symptom**: you see unexpected chats, messages, or `audit_events`
rows you didn't authorize. The token is in the wrong hands.

### Revoke + rotate + restart

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

### Forensics

```bash
# What did the leaked token do? Scan the window between plausible
# leak time and revocation.
shellboto audit search --kind command_run --since 168h \
  | less

# Focus on specific users who acted in the window.
shellboto audit search --user <telegram-id> --since 168h

# Export to JSON for offline analysis.
shellboto audit export --format json --since 168h > /tmp/leak-audit.jsonl
```

Anything unexpected → treat the VPS as compromised; snapshot the
DB (`shellboto db backup`) for forensic chain-of-custody, then
consider rebuilding from a known-good image.

---

## 3. Audit chain reports BROKEN

**Symptom**: `shellboto audit verify` prints `❌ audit chain BROKEN`.
This means a row's stored `row_hash` or `prev_hash` doesn't match
what would be computed from the canonical form — either legitimate
post-prune state (rare false positive) or genuine tampering.

### Triage

```bash
# 1. Read the verify output carefully — it names the first bad id
#    and the mismatch kind.
shellboto audit verify

# 2. Cross-check against journald's mirror. The audit journal
#    (C-3 in commit history) writes every event to syslog as well,
#    independently of the SQLite row. Any tamper has to corrupt
#    BOTH to hide.
journalctl -u shellboto -o cat --since "<approx-cutoff>" \
  | shellboto audit replay
```

The replay output shows per-id OK / MISSING / HASH_MISMATCH lines.
If the journald mirror matches expected hashes but DB doesn't → the
DB was tampered. If both disagree with each other in a way that
points to pruning → legitimate (retention pruner dropped old rows
and verify flagged the now-first row's seed binding).

### If tampered

```bash
# 1. Preserve state.
sudo systemctl stop shellboto
shellboto db backup /var/backups/shellboto-tamper-$(date -u +%Y%m%d-%H%M%S).db
cp /var/log/journal/<machine-id>/system.journal /var/backups/   # immutable copy

# 2. Investigate who had OS-level access to /var/lib/shellboto.
sudo ls -la /var/lib/shellboto/
sudo last
sudo aureport -au 2>/dev/null || true

# 3. After investigation, restart from last-known-good backup.
```

### If legit post-prune

Verify's output includes `PostPrune: true` when it skipped the
seed-binding check. That's expected — legitimate pruning
deliberately breaks the genesis binding. The chain between
surviving rows is still verified.

---

## 4. Database corruption

**Symptom**: `shellboto` fails to start with SQLite "database disk
image is malformed" or similar. Or `shellboto doctor` fails the
db-path check.

### Stop, preserve, restore

```bash
# 1. Stop the service so nothing else writes.
sudo systemctl stop shellboto

# 2. Snapshot the bad DB for forensics — don't nuke it.
shellboto db backup /tmp/bad-$(date -u +%Y%m%d-%H%M%S).db
#    (If even backup fails: `cp /var/lib/shellboto/state.db* /tmp/` raw.)

# 3. Try SQLite recovery.
sqlite3 /var/lib/shellboto/state.db ".recover" \
  | sqlite3 /tmp/recovered.db
#    Inspect /tmp/recovered.db; if it opens and has your users +
#    recent audit rows, you're in decent shape.

# 4. Restore from your last-good backup (the ops-owned one you
#    keep outside /var/lib/shellboto). Or promote /tmp/recovered.db.
sudo install -m 0600 /tmp/recovered.db /var/lib/shellboto/state.db
sudo chown root:root /var/lib/shellboto/state.db

# 5. Preflight + restart.
sudo shellboto doctor
sudo systemctl start shellboto
sudo systemctl status shellboto
shellboto audit verify
```

### If you have no backup

- SQLite's `.recover` usually salvages 95%+ of rows from a
  corrupted file. Accept the loss of whatever didn't.
- If even `.recover` fails, start fresh — `rm /var/lib/shellboto/state.db*`,
  restart, the bot will re-seed the superadmin row on startup.
  You lose all user records and audit history; accept the reset or
  roll back to a host-level snapshot if you have one.

---

## Prevention — worth the effort

- Host-level daily snapshots of `/var/lib/shellboto/` (LVM snapshot,
  Btrfs, BackupPC, Restic, whatever). Anything is better than
  nothing.
- Separate off-box copy of the audit journal — `journalctl` already
  writes durably, but having it replicated to another host means the
  mirror survives even a full-disk event.
- Run `shellboto audit verify` periodically (cron weekly) — catches
  silent tampering faster than waiting for an incident.
