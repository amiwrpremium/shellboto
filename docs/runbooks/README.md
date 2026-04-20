# Runbooks

Step-by-step procedures for incidents. Every tool referenced here
already exists — these docs just thread the steps together so a
half-awake operator at 3 AM doesn't have to think.

| File | When to use |
|------|-------------|
| [bad-release.md](bad-release.md) | New version is broken in production |
| [token-leaked.md](token-leaked.md) | The bot token is in the wrong hands |
| [audit-chain-broken.md](audit-chain-broken.md) | `audit verify` reports BROKEN |
| [db-corruption.md](db-corruption.md) | SQLite reports integrity errors |
| [shell-stuck.md](shell-stuck.md) | A user's pty has wedged, won't recover |
| [disk-full.md](disk-full.md) | `/var/lib/shellboto/` (or its filesystem) is full |

## Tooling that shows up across runbooks

- `sudo ./deploy/rollback.sh` — atomic swap to previous binary.
- `shellboto doctor` — preflight / post-change sanity check.
- `shellboto audit verify` — hash-chain integrity walk.
- `shellboto audit replay` — cross-check DB against journald.
- `shellboto audit search` — filter audit events.
- `shellboto db backup <path>` — online SQLite snapshot.
- `systemctl` / `journalctl` — standard systemd ops.

## Order of operations on any incident

1. **Stop bleeding** — service rollback, token revoke, etc.
2. **Snapshot** — `shellboto db backup` before changing anything
   destructive.
3. **Diagnose** — `journalctl -u shellboto -n 500`,
   `shellboto doctor`, `shellboto audit verify`.
4. **Fix** — apply the runbook step.
5. **Verify** — repeat doctor + verify; run a test command.
6. **Document** — add a note to `CHANGELOG.md` or your incident
   log.

## Read next

- The runbook matching your symptom.
