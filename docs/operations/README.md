# Operations

Day-2 concerns: the `shellboto` is running; here's how to keep it
running.

| File | What it covers |
|------|----------------|
| [doctor.md](doctor.md) | `shellboto doctor` preflight |
| [logs.md](logs.md) | journald, zap, audit mirror |
| [heartbeat-and-idle-reap.md](heartbeat-and-idle-reap.md) | Per-command heartbeat, idle-shell reaper |
| [user-management.md](user-management.md) | Add / remove / promote / demote |
| [updating.md](updating.md) | Upgrading to a newer release |
| [monitoring.md](monitoring.md) | What to watch, alerts to set |

## Daily / weekly / monthly

| Cadence | Thing |
|---------|-------|
| Daily | Skim `journalctl -u shellboto -n 200`; nothing red = fine |
| Weekly | `shellboto users list` — still only the right people? |
| Monthly | Restore a backup to a test path + `audit verify` against it |
| On incident | [../runbooks/](../runbooks/) |

## When something looks off

Order of operations:

1. `shellboto doctor` — basic health.
2. `systemctl status shellboto` — is it running?
3. `journalctl -u shellboto -n 200` — what's it saying?
4. `shellboto audit verify` — is the chain intact?

If any of those is red, pick the appropriate runbook.

## Read next

- [doctor.md](doctor.md) — the first thing you run.
