# Troubleshooting

Common error symptoms and their fixes. For incident-grade
procedures, go to [../runbooks/](../runbooks/).

| File | Covers |
|------|--------|
| [installer-fails.md](installer-fails.md) | `install.sh` errors |
| [bot-not-responding.md](bot-not-responding.md) | Bot doesn't reply on Telegram |
| [commands-never-complete.md](commands-never-complete.md) | A user's commands hang forever |
| [audit-verify-fails.md](audit-verify-fails.md) | `audit verify` reports broken |
| [common-errors.md](common-errors.md) | Error message → cause → fix table |

## Decision tree

| What's wrong | Start here |
|--------------|-----------|
| Can't install | [installer-fails.md](installer-fails.md) |
| Bot silent | [bot-not-responding.md](bot-not-responding.md) |
| Bot replies but commands hang | [commands-never-complete.md](commands-never-complete.md) |
| `audit verify` fails | [audit-verify-fails.md](audit-verify-fails.md) → escalate to [../runbooks/audit-chain-broken.md](../runbooks/audit-chain-broken.md) |
| Specific error string | [common-errors.md](common-errors.md) |

## When in doubt

```bash
sudo shellboto doctor
sudo systemctl status shellboto
sudo journalctl -u shellboto -n 200
sudo shellboto audit verify
```

These four commands tell you 80% of what you need to know.
