# Security

How shellboto is designed defensively and what attack surfaces it
leaves exposed. Read this end-to-end before whitelisting anyone you
don't fully trust.

| File | What it covers |
|------|----------------|
| [threat-model.md](threat-model.md) | What we protect against, what we explicitly don't |
| [whitelist-and-rbac.md](whitelist-and-rbac.md) | Auth flow, RBAC transitions, `auth_reject` audit rows |
| [audit-chain.md](audit-chain.md) | SHA-256 hash chain math, verify walker, tamper detection |
| [audit-seed.md](audit-seed.md) | `SHELLBOTO_AUDIT_SEED` — generation, storage, rotation |
| [danger-matcher.md](danger-matcher.md) | **Every built-in regex with examples + rationale** |
| [secret-redaction.md](secret-redaction.md) | The redactor's pattern list, replacement rules, known limitations |
| [rate-limiting.md](rate-limiting.md) | Post-auth + pre-auth token buckets; why the pre-auth one matters |
| [root-shell-implications.md](root-shell-implications.md) | What root access gets an attacker; mitigations |

## The one-line version

shellboto assumes **one VPS, one operator** (the superadmin), and
treats the Telegram account with 2FA as the primary auth factor.
Inside that trust model, layered defenses limit blast radius from
typos (danger matcher), plaintext leaks (redaction), and local
tampering (hash-chained audit).

## What shellboto is NOT

- **Not a zero-trust RCE platform.** If someone compromises your
  Telegram account, they inherit your shell.
- **Not hardened against a malicious-admin insider.** An admin
  with a root shell can defeat any regex and edit any log. The
  audit seed + journald mirror catch silent edits; loud edits
  succeed.
- **Not a sandbox.** `role=user` shells can still run any command
  their unix identity allows. OS perms do the real containment.
- **Not a secret scanner.** The redactor has ~17 patterns. Your
  novel secret formats probably aren't covered.

## Layered defenses

```
┌─────────────────────────────────────────┐
│ 1. Telegram account + 2FA               │  ← you own this
├─────────────────────────────────────────┤
│ 2. Whitelist (users table)              │
├─────────────────────────────────────────┤
│ 3. RBAC (superadmin / admin / user)     │
├─────────────────────────────────────────┤
│ 4. Rate limiting (per-user + pre-auth)  │
├─────────────────────────────────────────┤
│ 5. Danger matcher (+ /confirm flow)     │
├─────────────────────────────────────────┤
│ 6. OS perms (non-root user shells)      │
├─────────────────────────────────────────┤
│ 7. Secret redaction (audit storage)     │
├─────────────────────────────────────────┤
│ 8. Hash-chained audit log + journald    │
└─────────────────────────────────────────┘
```

Each layer fails differently; the combination is what matters.

## Operator obligations

Run shellboto, accept these responsibilities:

1. **2FA on Telegram.** Non-negotiable.
2. **Keep the whitelist small.** Every user is trust.
3. **Use `role=user` + non-root shells** for anyone who's not you.
4. **Set `SHELLBOTO_AUDIT_SEED`** in production.
5. **Monitor `journalctl -u shellboto`** for `auth_reject` spikes.
6. **Rotate the bot token** if it's ever exposed.
7. **Read [runbooks/](../runbooks/)** before you need them at 3am.

## Read next

- [threat-model.md](threat-model.md) — the full scoping document.
- [danger-matcher.md](danger-matcher.md) — the regex table
  everybody wants to see.
