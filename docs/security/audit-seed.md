# The audit seed

`SHELLBOTO_AUDIT_SEED` is the genesis value for the audit hash
chain. 32 bytes, hex-encoded (64 chars from `[0-9a-f]`).

## Why it matters

Without the seed, `prev_hash` of the first row is 32 zero bytes,
which anyone can compute. An attacker with full DB access can
rewrite every row, compute fresh hashes starting from zero, and
produce a cosmetically clean audit chain — you'd have no way to
tell.

With the seed:

- The first row's hash depends on the seed.
- The attacker needs to read the seed (which lives in
  `/etc/shellboto/env`, mode 0600, root-only) to forge anything.
- If they have root, they already have everything — but the seed
  adds one more authentication factor they had to acquire.

It's not perfect security. It's cheap, correct, and raises the bar.

## Generating

### `shellboto mint-seed`

```bash
shellboto mint-seed
# prints a fresh 32-byte hex string
```

`--env` variant emits in env-file format:

```bash
shellboto mint-seed --env
# SHELLBOTO_AUDIT_SEED=abc123...
```

Paste the second form straight into `/etc/shellboto/env`.

### openssl (equivalent)

```bash
openssl rand -hex 32
```

Same result, works without shellboto installed.

### getting 32 bytes from /dev/urandom

```bash
head -c 32 /dev/urandom | xxd -p -c 64
```

Functionally equivalent. Avoid anything that truncates or re-seeds
from a weaker source.

## Storage

Two modes. Pick one.

### Env-file mode (default)

```
/etc/shellboto/env:
SHELLBOTO_AUDIT_SEED=<64-char hex>
```

File perms: `-rw------- 1 root root`. systemd loads this into the
bot's env before ExecStart via `EnvironmentFile=`.

### systemd-creds mode (encrypted at rest)

After running `./deploy/enable-credentials.sh`:

```
/etc/shellboto/credentials/shellboto-audit-seed.cred   # ciphertext, 0600
```

Decrypted at service start into `$CREDENTIALS_DIRECTORY/shellboto-audit-seed`
(memfd-backed, never on disk). `shellboto doctor` reports
`SHELLBOTO_AUDIT_SEED valid  32 bytes, source=systemd-creds`.

Deep dive: [secrets-at-rest.md](secrets-at-rest.md).

Do **not**:

- Commit the seed to git.
- Back it up anywhere the VPS itself doesn't protect (e.g. it's
  not a thing to store in a password manager; it's VPS-local).
- Share it between multiple VPSes. Each shellboto deployment's seed
  is independent — sharing means cross-compromise.
- Transmit it in clear text over chat or email. If you have to send
  it somewhere (rare), use an E2E-encrypted channel, and only to
  yourself on the new VPS.

## The "no seed set" warning

At startup, if `SHELLBOTO_AUDIT_SEED` is empty, the bot logs:

```
WARN  audit seed not set — audit chain uses all-zeros genesis.
      Acceptable for development; not for production.
```

and uses 32 zero bytes. You'll see this in `journalctl -u shellboto`
right after startup. `shellboto doctor` also flags it.

Fix: mint a seed + add it to the env file + restart.

Do not suppress the warning without setting a real seed.

## Rotation

**Rotating the seed breaks the existing chain.** All pre-rotation
rows are cryptographically orphaned — they were hashed against a
seed you no longer have, so verify starts failing at the rotation
point.

Rotate only when:

1. You believe the seed has been compromised.
2. You're migrating to a new VPS + starting a fresh deployment
   anyway.
3. You want to cut a hard "start fresh" point in the audit log
   (e.g. annual compliance window).

Procedure:

```bash
# 1. Archive the current DB with its current seed.
sudo shellboto db backup /var/lib/shellboto/backups/state.db-$(date -I).pre-rotation

# 2. Generate the new seed.
NEW_SEED=$(shellboto mint-seed)

# 3. Write it atomically so there's no half-file state.
sudo install -m 0600 -o root -g root /dev/stdin /etc/shellboto/env <<EOF
SHELLBOTO_TOKEN=$(grep ^SHELLBOTO_TOKEN= /etc/shellboto/env | cut -d= -f2-)
SHELLBOTO_SUPERADMIN_ID=$(grep ^SHELLBOTO_SUPERADMIN_ID= /etc/shellboto/env | cut -d= -f2-)
SHELLBOTO_AUDIT_SEED=${NEW_SEED}
EOF

# 4. Restart.
sudo systemctl restart shellboto

# 5. Verify — expect a break at the oldest row.
sudo shellboto audit verify
# Chain BROKEN at row 1 — expected after seed rotation.

# 6. If the current DB matters, either:
#    a) keep running with the "break" at the rotation point (rows from
#       before verify as a sub-chain against the old seed, if you kept
#       it), or
#    b) delete pre-rotation rows so verify is clean from this point:
sudo sqlite3 /var/lib/shellboto/state.db \
    "DELETE FROM audit_events WHERE id < (SELECT MIN(id) FROM audit_events WHERE ts > datetime('now', '-1 minute'));"
sudo systemctl restart shellboto
sudo shellboto audit verify
# Chain OK — N rows verified (post-rotation only).
```

**Deleting pre-rotation rows is a policy decision.** Compliance
regimes that require N-year retention won't allow it. Talk to
whoever owns that policy.

## Recovery when the seed is lost

If `/etc/shellboto/env` is deleted / overwritten and you have no
backup:

- You cannot re-run verify against the existing chain.
- But the chain itself is still internally consistent — every
  `row_hash` still matches `sha256(prev_hash || canonical(row))`
  where `prev_hash` is whatever is stored; verify without the
  original seed just can't attest that row 1's prev_hash is
  correct. It still detects all downstream tampering.

Practical recovery:

1. Mint a new seed (or leave blank for all-zeros).
2. Bot starts; writes new rows chained from the current tip.
3. Verify reports row 1 as "genesis mismatch" (expected) but
   everything from current-tip forward is valid.

## Backup strategy

The seed is small (64 chars). Two pragmatic options:

- **VPS-local only.** Accept that seed loss means chain discontinuity
  but not catastrophe. For solo deployments this is fine.
- **Encrypted in your VPS backup.** If you already encrypt VPS
  backups (`restic`, `borg`, etc.), the seed is included
  automatically. Make sure the decryption key is offsite.

**Do not** store the seed in a passwords manager, cloud notes,
git, or paper alongside "this is the audit seed for server X" —
that metadata is half of what makes it a credential.

## Doctor check

```bash
shellboto doctor | grep -i seed
# SHELLBOTO_AUDIT_SEED   set (64 hex chars)  ✅
```

If you see `not set — using zero-seed fallback`, fix before prod.

## Read next

- [audit-chain.md](audit-chain.md) — how the seed is used in the
  chain math.
- [../runbooks/audit-chain-broken.md](../runbooks/audit-chain-broken.md)
  — what to do when `audit verify` fails in anger.
