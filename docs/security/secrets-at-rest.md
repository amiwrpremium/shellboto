# Secrets at rest

Two modes for delivering the bot token and audit seed to the
service. The **env-file** mode is the default (what `install.sh`
sets up). The **systemd-creds** mode is optional hardening — run
one helper script to enable.

| | env file | systemd-creds |
|---|---|---|
| File on disk | plaintext at `/etc/shellboto/env` (0600 root) | encrypted blob at `/etc/shellboto/credentials/*.cred` (0600 root) |
| Needed at runtime | loaded into process env by systemd | decrypted into `$CREDENTIALS_DIRECTORY`, in-memory only |
| Backup / snapshot / disk-image theft exposure | **plaintext** — attacker on another host reads the token | **ciphertext** — useless without the original host's TPM or `/var/lib/systemd/credential.secret` |
| `printenv` inside a user shell | never (we strip `SHELLBOTO_*`) | never |
| `cat /proc/<pid>/environ` as root on the running VPS | reads token | does NOT contain token (env var isn't set) |
| Hypervisor memory dump of running VPS | reads token from process memory | same — decrypted secret is in memory while the service runs |
| Any other host attempting to read the backup | reads token | **cannot decrypt**, especially on TPM2 hardware |

Neither mode defends against **root on the running, active VPS**.
That's a fundamental trust boundary — nothing short of a
confidential-computing VM (AMD SEV-SNP, Intel TDX) defends
against it. The env-vs-creds choice only affects the **at-rest**
threat: backups, snapshots, disk theft, and hypervisor image
exfiltration.

## What counts as a secret

In shellboto's environment:

- **`SHELLBOTO_TOKEN`** — yes. The bot token from @BotFather.
  If leaked, anyone impersonates the bot. Migrate.
- **`SHELLBOTO_AUDIT_SEED`** — yes. The genesis for the hash
  chain. If leaked AND the attacker has DB-write access, they
  can rewrite history silently. Migrate.
- **`SHELLBOTO_SUPERADMIN_ID`** — no. A public Telegram user
  identifier; appears in audit rows, on @BotFather's end, in
  messages you send from that account. Not sensitive. Stays in
  the env file as the single remaining line.

## Env-file mode (default)

```
/etc/shellboto/env                       0600 root:root
├── SHELLBOTO_TOKEN=…
├── SHELLBOTO_SUPERADMIN_ID=…
└── SHELLBOTO_AUDIT_SEED=…
```

systemd reads this via `EnvironmentFile=` before `ExecStart=`.
The values live in the process environment until shutdown.
`sanitizedEnv` strips every `SHELLBOTO_*` from the environment
passed to spawned bash, so users can't see them via `printenv`.

## systemd-creds mode (opt-in hardening)

After install, run:

```bash
sudo ./deploy/enable-credentials.sh
```

The script:

1. Reads token + seed from `/etc/shellboto/env`.
2. `systemd-creds encrypt --name=shellboto-token …` and
   `--name=shellboto-audit-seed …` — produces ciphertext blobs.
3. Writes blobs to `/etc/shellboto/credentials/*.cred`.
4. Drops
   `/etc/systemd/system/shellboto.service.d/credentials.conf`
   with `LoadCredentialEncrypted=` entries.
5. Strips plaintext `SHELLBOTO_TOKEN` + `SHELLBOTO_AUDIT_SEED`
   from `/etc/shellboto/env` (only `SHELLBOTO_SUPERADMIN_ID`
   remains).
6. `systemctl daemon-reload` + `systemctl restart shellboto`.
7. Runs `shellboto doctor` — reports `source=systemd-creds` for
   both the token and seed.

Result on disk:

```
/etc/shellboto/
├── env                                0600   (only SUPERADMIN_ID now)
├── credentials/
│   ├── shellboto-token.cred           0600   encrypted blob
│   └── shellboto-audit-seed.cred      0600   encrypted blob
└── config.toml                        0600

/etc/systemd/system/shellboto.service.d/
└── credentials.conf                   0644   LoadCredentialEncrypted= lines
```

At runtime, systemd decrypts each `.cred` into a memfd-backed
file at `$CREDENTIALS_DIRECTORY/<name>` (read-only, only visible
to the service PID). The shellboto binary reads that path via
`config.ResolveSecret` — identical behaviour to reading the env
var, just a different source.

### Revert

```bash
sudo ./deploy/enable-credentials.sh --revert
```

Removes the drop-in + the cred directory. You'll need to re-add
the plaintext `SHELLBOTO_TOKEN` and `SHELLBOTO_AUDIT_SEED` lines
to `/etc/shellboto/env` by hand before restarting.

## Requirements

- systemd **250+** (has `LoadCredentialEncrypted=`). Check with
  `systemctl --version`.
  - Debian 12+: ✅ (systemd 252)
  - Ubuntu 22.10+: ✅ (systemd 251)
  - Fedora 37+: ✅ (systemd 252)
  - RHEL 9+: ✅ (systemd 252)
  - Older systems: stay on env-file mode.
- **TPM2** optional but recommended. `systemd-creds` uses it
  automatically when present. Without a TPM, it uses
  `/var/lib/systemd/credential.secret` (0600 root) as the
  encryption key — still machine-bound, just without hardware
  attestation.

Check: `systemd-creds has-tpm2` → returns 0 if TPM usable.

## Key rotation

### Token leak → rotate at @BotFather

```bash
# (a) At @BotFather on Telegram: /token → pick bot → regenerate.
# (b) On the VPS:
NEW_TOKEN=…
printf '%s' "$NEW_TOKEN" | sudo systemd-creds encrypt \
    --name=shellboto-token - /etc/shellboto/credentials/shellboto-token.cred
sudo systemctl restart shellboto
sudo shellboto doctor
```

### Audit-seed rotation

Same pattern but with `--name=shellboto-audit-seed`. See
[audit-seed.md](audit-seed.md) for why seed rotation also breaks
the existing chain — plan the audit-history implications before.

## Migrating to a new host

TPM-sealed creds are **tied to this host**. You cannot copy
`/etc/shellboto/credentials/*.cred` to a different machine and
expect them to decrypt — that's the whole point. To move:

1. On the new host, install shellboto + start in env-file mode
   with fresh token (rotate at @BotFather) + fresh audit seed.
2. Run `enable-credentials.sh` on the new host.

If you want to preserve the audit seed across hosts (to keep
chain continuity), you'll need to decrypt on the old host,
securely transfer, and re-encrypt:

```bash
# On old host:
SEED=$(sudo cat /var/lib/systemd/credential.secret | \
    sudo systemd-creds decrypt /etc/shellboto/credentials/shellboto-audit-seed.cred -)
# (transfer $SEED securely — gpg, signal, age, etc.)

# On new host:
printf '%s' "$SEED" | sudo systemd-creds encrypt \
    --name=shellboto-audit-seed - /etc/shellboto/credentials/shellboto-audit-seed.cred
```

**Don't automate that.** Any automation crossing the host
boundary with plaintext seed defeats the point.

## What remains plaintext no matter what

- **`/etc/shellboto/config.toml`** — not a secret, but not
  public either (has paths, policy knobs). 0600 keeps it
  private to root; this is fine.
- **`SHELLBOTO_SUPERADMIN_ID`** in the env file — public.
- **`/var/lib/shellboto/state.db`** — SQLite file. Contains
  the audit log including (post-redact) command output. **Not
  encrypted at rest.** If your audit blobs might contain
  novel secrets the redactor doesn't catch, set
  `audit_output_mode = never` (see
  [../configuration/audit-output-modes.md](../configuration/audit-output-modes.md))
  so nothing sensitive hits disk in the first place.

## Reading the code

- `internal/config/secret.go` — `ResolveSecret` + `ResolveSecretWithSource`
- `internal/config/config.go:Load` — where the token comes in
- `cmd/shellboto/main.go:resolveAuditSeed` — where the seed comes in
- `deploy/enable-credentials.sh` — the migration helper
- `cmd/shellboto/cmd_doctor.go` — reports which mode is active

## Read next

- [threat-model.md](threat-model.md) — the broader posture.
- [audit-seed.md](audit-seed.md) — seed-specific operational concerns.
- [../deployment/production-checklist.md](../deployment/production-checklist.md)
  — production install flow, including when to flip to creds mode.
