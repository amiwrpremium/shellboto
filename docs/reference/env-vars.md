# Environment variables

| Var | Required | Sensitive | Source | Purpose |
|-----|:--------:|:---------:|--------|---------|
| `SHELLBOTO_TOKEN` | yes | **yes** | `/etc/shellboto/env` (loaded by systemd) | Telegram bot token from @BotFather |
| `SHELLBOTO_SUPERADMIN_ID` | yes | no | same | Telegram user ID of the singleton superadmin (positive int64) |
| `SHELLBOTO_AUDIT_SEED` | recommended | yes | same | 32-byte (64 hex chars) genesis seed for the audit hash chain. Empty = all-zeros fallback with startup warning |

These are the **only** env vars shellboto reads. Other config is
in the config file (`/etc/shellboto/config.toml` etc.).

## File-based fallback (systemd-creds)

`SHELLBOTO_TOKEN` and `SHELLBOTO_AUDIT_SEED` each have a second
source: if the env var is empty at startup, shellboto reads
`$CREDENTIALS_DIRECTORY/shellboto-token` and
`$CREDENTIALS_DIRECTORY/shellboto-audit-seed` respectively.
`CREDENTIALS_DIRECTORY` is injected by systemd (250+) when the
unit declares `LoadCredential=` / `LoadCredentialEncrypted=`. Opt
in via `./deploy/enable-credentials.sh`; deep dive in
[../security/secrets-at-rest.md](../security/secrets-at-rest.md).

## Stripped from spawned shells

`internal/shell/shell.go:sanitizedEnv` strips every `SHELLBOTO_*`
variable from the environment passed to bash. So a `role=user`
caller running `printenv SHELLBOTO_TOKEN` gets nothing.

## File perms

```
-rw------- 1 root root /etc/shellboto/env
```

Mode 0600. Owner root. Anything else and `doctor` flags it.

## Generating each

### Token

@BotFather → `/newbot` (first time) or `/token` (rotate).

### Superadmin ID

@userinfobot or any other discovery method (see
[../getting-started/find-user-id.md](../getting-started/find-user-id.md)).

### Audit seed

```bash
shellboto mint-seed
# or
openssl rand -hex 32
```

## Read next

- [../configuration/environment.md](../configuration/environment.md)
  — full prose treatment.
- [../security/audit-seed.md](../security/audit-seed.md) — seed
  rotation deep-dive.
