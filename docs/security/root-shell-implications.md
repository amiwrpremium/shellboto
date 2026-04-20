# Root-shell implications

`role=admin` and `role=superadmin` shells always run as root. This
is by design (they're trusted operators) but it pays to understand
the blast radius.

## What root gets you

A root shell on the VPS means:

- Read every file, including `/etc/shadow`, every user's home,
  every SSH key.
- Write anywhere: replace the kernel with a malicious one at next
  reboot (`/boot/vmlinuz-*`), stage a persistent backdoor in
  `systemd`, edit audit DB + journald archives.
- Network: open any port, connect anywhere, intercept via iptables,
  terminate TLS, hijack sessions.
- System control: shut down, reboot, kill any process, install
  packages, add users, escalate anyone to sudo.
- Pivot: SSH onwards to anything the host can reach. If the host
  is in a private network with other services, lateral movement
  is trivial.

If your threat model includes "my admin's Telegram account gets
stolen," the attacker inherits all of the above. The shellboto
process itself doesn't make this worse — it just gives them a nice
UI over the standard root-shell experience.

## What root does NOT give you via shellboto specifically

- **No extra access to other VPSes.** shellboto has no
  configuration for remote hosts. `ssh` to elsewhere from the bot
  shell is whatever the VPS's SSH credentials allow — which is an
  OS-level question, not a shellboto one.
- **No bypass of external rate limits.** If your VPS is rate-
  limited by a cloud provider (AWS account throttles, GitHub API
  limits), the bot shell has the same limits as any other SSH
  session would.

## Mitigations you can apply

### 1. Don't give anyone admin you wouldn't trust with root SSH

Reasoning: that's precisely the access you're granting.

If someone needs "enough access to run commands" but not "root-
level trust," keep them as `role=user` with `user_shell_user`
configured to a non-root account.

### 2. Separate prod and dev bots

One shellboto per VPS. If you operate multiple VPSes:

- Separate bot tokens (distinct @BotFather bots).
- Separate whitelists (different superadmin IDs per host if you
  want per-host lockout granularity).
- Separate audit chains (separate `SHELLBOTO_AUDIT_SEED`).
- Same set of admins, but cross-host they're separate policy
  decisions.

Compromise of one bot is then scoped to one host.

### 3. Keep the whitelist tight

- Add users only when they have an active, ongoing need.
- Remove them the same day that need ends.
- Review `shellboto users list` monthly; soft-delete inactive rows.

### 4. Keep audit retention long

Default 90 days is short for incident response. If you can afford
the disk (usually you can; the DB is small), bump to:

```toml
audit_retention = "8760h"   # 1 year
```

And keep offsite encrypted backups of the DB so you can reconstruct
history even if a compromise wipes local state.

### 5. Alert on `auth_reject` spikes

A spike in rejected auth attempts = someone's probing. Set up a
log forwarder (filebeat / fluentbit / vector) to ship
`journalctl -u shellboto` to your alerting stack, and alert on:

- `auth_reject` rate > baseline × 10.
- Any audit row with `danger_pattern` matched outside your
  operating hours.
- `kind=role_changed` events (promote/demote) that you didn't
  expect.

### 6. Alert on chain breaks

Scheduled `shellboto audit verify`:

```
# /etc/cron.d/shellboto-audit-verify
0 */6 * * * root /usr/local/bin/shellboto audit verify || mail -s "shellboto audit BROKEN on $(hostname)" you@you.net
```

If this fires, you have minutes to start
[runbooks/audit-chain-broken.md](../runbooks/audit-chain-broken.md).

### 7. Consider OS-level hardening

shellboto doesn't fight the OS — it cooperates. Complementary
hardening that shellboto doesn't replace:

- **fail2ban** on SSH. If an attacker SSHs after getting the bot
  shell, you want detection.
- **auditd** for kernel-level syscall logging. Independent of
  shellboto's audit log.
- **File integrity monitoring** (`tripwire`, `aide`). Detects
  `/etc/shadow` or `/etc/passwd` changes the bot didn't make.
- **AppArmor** / **SELinux** profiles. Not shipped with shellboto;
  write your own if you need them.

### 8. Minimise what root can do remotely

- No public-facing services unless they're meant to be public.
- Firewall outbound too, if paranoia allows. The bot itself needs
  only `api.telegram.org:443`; everything else is optional.

## The explicit trade-off

shellboto chose to ship as a root-shell bot because:

- **Operational reality.** You administer a VPS. You need root
  access to do the job. A non-root shell with sudo prompts is
  worse UX (and sudo's own bypass surface adds bugs).
- **Target audience.** This is built for the single-VPS single-
  operator case. Not for multi-tenant platforms, not for untrusted
  end-users.
- **Honesty.** A bot that *claims* to sandbox root shells but can
  be bypassed via any of `eval`, `$(base64 -d)`, interpreter one-
  liners, or just `cp /bin/bash /tmp/b && chmod +s /tmp/b` is
  worse than an honest root shell — because it gives a false sense
  of protection.

The whitelist + 2FA on Telegram is the practical auth. If that
falls, it falls loudly.

## If an admin shell gets compromised

Not "if the bot goes rogue" — that's
[runbooks/token-leak.md](../runbooks/token-leak.md).

If a specific admin's Telegram account is taken:

1. **Revoke the bot token** (`@BotFather` → `/token` → regenerate).
   The attacker's held session dies.
2. **`/deluser <compromised_admin_id>`** from another admin (if
   any) or from superadmin. Their shell auto-closes; their future
   messages bounce as `auth_reject`.
3. Update `SHELLBOTO_TOKEN` with the new value, restart.
4. Run `shellboto audit verify` — make sure nothing was silently
   edited.
5. Run `shellboto audit search --user <compromised_id> --since <window>`
   to see what they did.
6. Treat the VPS as potentially backdoored:
   - `find / -newer <last-trusted-date> -type f` for changes.
   - Check crontabs, `/etc/systemd/system/*.service`, `.bashrc`,
     authorized_keys, PATH hijacks.
7. If in doubt, rotate the VPS: restore from snapshot before the
   compromise window, or stand up a fresh VPS from images.

## The bottom line

shellboto makes operating a single-VPS bot convenient and
auditable. It does **not** make an untrusted admin safe. The trust
boundary is "admin's Telegram account is secure"; everything inside
that boundary is privileged.

## Read next

- [threat-model.md](threat-model.md) — the explicit scope.
- [../configuration/non-root-shells.md](../configuration/non-root-shells.md)
  — how to run `role=user` shells as a non-root identity.
- [../runbooks/](../runbooks/) — what to do when something bad
  actually happens.
