# Environment variables

shellboto reads three environment variables at startup. All come
from `/etc/shellboto/env` (mode 0600, root-owned), loaded by
systemd's `EnvironmentFile=`.

## `SHELLBOTO_TOKEN` (required, secret)

- The bot token from @BotFather — see
  [../getting-started/create-telegram-bot.md](../getting-started/create-telegram-bot.md).
- Looks like `123456789:AAHvZk…` (colon-delimited numeric id + 33+
  base64url chars).
- Validated at startup — empty value is a fatal error.
- **Stripped from spawned shell environments.** The `shell` package
  sanitises `SHELLBOTO_*` vars out of the env passed to bash
  (`internal/shell/shell.go`'s `sanitizedEnv`). Running `printenv
  SHELLBOTO_TOKEN` inside a pty shell returns empty.
- **Not logged.** zap logger's structured fields never include
  the token value.
- Rotate via @BotFather: `/token` → pick the bot → regenerate.
  Old token dies immediately; update the env file + restart.

## `SHELLBOTO_SUPERADMIN_ID` (required)

- Numeric Telegram user ID of the sole superadmin.
- Validated: must parse as positive int64; zero and negatives
  rejected.
- **Seeded on every startup.** `userRepo.SeedSuperadmin(id)` runs
  in a transaction:
  - Demotes any existing `role=superadmin` rows (except the target
    ID) to `role=admin`.
  - Creates or upserts the target ID row as superadmin.
  - Clears `disabled_at` if the target was previously banned.
- **Handoff path.** To change superadmin: edit the env file →
  `systemctl restart shellboto`. The previous superadmin's row is
  auto-demoted; their open shells are auto-reset next message.
- **Not a secret.** The ID itself isn't sensitive — but the
  env file is 0600 because `_TOKEN` and `_AUDIT_SEED` live next
  to it.

## `SHELLBOTO_AUDIT_SEED` (recommended, secret)

- 32 bytes, hex-encoded (64-char string over `[0-9a-f]`).
- **Genesis hash for the audit chain.** The first audit row's
  `prev_hash` comes from this value. Without it, an attacker with
  DB write access can silently rebuild the chain.
- Generate with `openssl rand -hex 32` or `shellboto mint-seed`.
- **Empty → all-zeros fallback.** The process starts with a loud
  warning in the logs. Acceptable for dev; **not for production**.
- **Do not rotate casually.** Rotating breaks the existing chain
  — all pre-rotation audit rows are cryptographically orphaned.
  See [../security/audit-seed.md](../security/audit-seed.md) for
  the correct rotation procedure.

## Credentials-file fallback (systemd-creds)

For **both** `SHELLBOTO_TOKEN` and `SHELLBOTO_AUDIT_SEED`: if the env
var is unset or empty at startup, shellboto falls back to reading
from `$CREDENTIALS_DIRECTORY/<credname>`:

- `SHELLBOTO_TOKEN` → `$CREDENTIALS_DIRECTORY/shellboto-token`
- `SHELLBOTO_AUDIT_SEED` → `$CREDENTIALS_DIRECTORY/shellboto-audit-seed`

`CREDENTIALS_DIRECTORY` is set by systemd (250+) when the unit
declares `LoadCredential=` or `LoadCredentialEncrypted=`. It points
at a read-only memfd-backed directory visible only to the
service's PID — the value never touches disk for the running
process.

Run `./deploy/enable-credentials.sh` to migrate from plaintext env
file to encrypted creds. Deep dive:
[../security/secrets-at-rest.md](../security/secrets-at-rest.md).

## The env file

Installer writes `/etc/shellboto/env` from `deploy/env.example`,
substituting values you provided. Shape:

```
SHELLBOTO_TOKEN=123456789:AAHvZkExampleBase64URLBlob
SHELLBOTO_SUPERADMIN_ID=987654321
SHELLBOTO_AUDIT_SEED=abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789
```

Perms:

```
-rw------- 1 root root /etc/shellboto/env
```

Don't:

- Put it in git.
- Share it. If it's on a second host, the *second* host has root
  on the first host, effectively.
- Leave it world-readable. 0600 is non-negotiable; `shellboto
  doctor` checks.

## Systemd loads it

From `deploy/shellboto.service`:

```
EnvironmentFile=/etc/shellboto/env
```

systemd reads + parses (`KEY=VALUE` pairs, `#` for comments) and
sets those as the process environment for `ExecStart=`.

If systemd can't read the file (wrong perms, bad path): the unit
fails to start with an error in `journalctl -u shellboto`.

## What's NOT an env var

To avoid the "everything is configurable from everywhere"
complexity, shellboto does not support arbitrary env-var overrides
for config keys:

- No `SHELLBOTO_IDLE_REAP=30m` override.
- No `SHELLBOTO_AUDIT_RETENTION=720h`.
- No `SHELLBOTO_LOG_LEVEL=debug`.

Change these in the config file and restart.

The three env vars above exist because they're secrets (token,
seed) or deployment identity (superadmin id) — values that don't
belong in a file that might get copied, backed up, or diffed in git
alongside non-secret config.

## Read next

- [../security/audit-seed.md](../security/audit-seed.md) — seed
  management deep dive.
- [roles.md](roles.md) — capability matrix from superadmin to user.
