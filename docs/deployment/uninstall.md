# Uninstall — `deploy/uninstall.sh`

Safe removal. Keeps config + audit DB by default.

Source: [`deploy/uninstall.sh`](../../deploy/uninstall.sh).

## Defaults

```bash
sudo ./deploy/uninstall.sh
```

- Stops the service.
- Disables the unit.
- Removes `/usr/local/bin/shellboto` (and `.prev`).
- Removes `/etc/systemd/system/shellboto.service`.
- `daemon-reload`.
- **Keeps** `/etc/shellboto/` (env + config).
- **Keeps** `/var/lib/shellboto/` (audit DB).

So a default uninstall is reversible: reinstall and the audit
history, whitelist, and config are all back.

## Flags

| Flag | Effect |
|------|--------|
| `-y`, `--yes` | No prompts. |
| `--remove-config` | Also `rm -rf /etc/shellboto/`. |
| `--remove-state` | Also `rm -rf /var/lib/shellboto/`. **Deletes audit DB.** |
| `--i-understand-this-deletes-audit-log` | Required alongside `--remove-state` in `-y` mode. |
| `--prefix DIR` | Operate on `<dir>/etc/...` etc. (chroots). |
| `--dry-run` | Print actions, change nothing. |

## The audit-DB guard

Without `--remove-state`, your audit history is safe.

With `--remove-state` interactively, you're prompted:

```
About to delete /var/lib/shellboto/ including state.db.
This will permanently destroy the audit log.
Type DELETE AUDIT LOG to confirm: _
```

You must type the exact phrase. Otherwise it bails.

In `-y` mode, the phrase is replaced by the
`--i-understand-this-deletes-audit-log` flag:

```bash
sudo ./deploy/uninstall.sh -y --remove-state --i-understand-this-deletes-audit-log
```

A stray `yes | ./uninstall.sh` without the flag does **not**
delete state.

## Why this much friction

The audit DB is unreproducible state. Backup before uninstall:

```bash
sudo shellboto db backup /var/backups/shellboto/final-$(date -I).db
# then uninstall
```

Or re-run the installer on the same machine later; without the
`--remove-state`, your DB survives.

## Full-wipe uninstall

For CI / test flows that genuinely want everything gone:

```bash
sudo ./deploy/uninstall.sh -y \
    --remove-config \
    --remove-state \
    --i-understand-this-deletes-audit-log
```

Leaves no trace under `/etc/shellboto`, `/var/lib/shellboto`,
`/usr/local/bin/shellboto*`, or the systemd unit.

## Reading the code

- `deploy/uninstall.sh` — the flow.
- `deploy/lib.sh` — prompt + die helpers.

## Read next

- [rollback.md](rollback.md) — for binary swaps, not full uninstalls.
- [../runbooks/db-corruption.md](../runbooks/db-corruption.md) —
  if you're considering `--remove-state` as a recovery strategy,
  read the runbook first.
