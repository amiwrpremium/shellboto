# Danger-matcher pattern reference

Single-page tabular view of every built-in regex from
`internal/danger/danger.go`.

For full prose treatment with examples and rationale, see
[../security/danger-matcher.md](../security/danger-matcher.md).

| # | Concern | Regex |
|---|---------|-------|
| 1 | rm -rf | `\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f\|-[a-zA-Z]*f[a-zA-Z]*r)\b` |
| 2 | dd to /dev/* | `\bdd\b.*\bof=/dev/` |
| 3 | mkfs.* | `\bmkfs\.` |
| 4 | fdisk/parted/wipefs | `\b(fdisk\|parted\|wipefs)\b` |
| 5 | redirect to /dev/sd* | `>\s*["']?/dev/(sd\|nvme\|mmcblk\|vd\|xvd)` |
| 6 | tee to /dev/sd* | `\btee\s+(-a\s+)?["']?/dev/(sd\|nvme\|mmcblk\|vd\|xvd)` |
| 7 | shutdown/reboot/halt/poweroff | `\b(shutdown\|reboot\|halt\|poweroff)\b` |
| 8 | init 0/6 | `\binit\s+[06]\b` |
| 9 | chown/chmod -R / | `\b(chown\|chmod)\s+-R\s+/\s*($\|[^a-zA-Z0-9_])` |
| 10 | pipe to shell | `\|\s*(ba\|z\|k\|c\|d\|)sh\b` |
| 11 | userdel | `\buserdel\b` |
| 12 | passwd root | `\bpasswd\s+root\b` |
| 13 | fork bomb | `:\(\)\s*\{\s*:\|:&\s*\};:` |
| 14 | iptables -F | `\biptables\s+-F\b` |
| 15 | systemctl stop sshd | `\bsystemctl\s+(stop\|disable\|mask)\s+(ssh\|sshd)\b` |
| 16 | overwrite /etc/{shadow,passwd,sudoers,group,gshadow} | `>\s*["']?/etc/(shadow\|passwd\|sudoers\|group\|gshadow)\b` |
| 17 | overwrite /etc/sudoers.d/* | `>\s*["']?/etc/sudoers\.d/` |
| 18 | overwrite /root/.ssh/* | `>\s*["']?/root/\.ssh/` |
| 19 | find -delete | `\bfind\b[^\n]*\s-delete\b` |
| 20 | find -exec rm | `\bfind\b[^\n]*\s-exec\s+rm\b` |
| 21 | truncate -s 0 | `\btruncate\s+-s\s*0\b` |
| 22 | interpreter -e/-c | `\b(perl\|python[23]?\|ruby\|node\|php)\s+-[ecErR]\b` |
| 23 | chattr +/- i | `\bchattr\s+[+-]i\b` |
| 24 | netcat -e/-l | `\b(nc\|ncat\|netcat)\s+-[elL]\b` |
| 25 | bash /dev/tcp | `/dev/tcp/` |

## Custom additions

`extra_danger_patterns` config key. Each string is a Go regex.
Compiled at startup; bad regex = fatal error.

```toml
extra_danger_patterns = [
  '\bvisudo\b',
  '\bnewgrp\b',
  '\bcrontab\s+-r\b',
]
```

Merged into the built-in list; first-match wins.

## Test a pattern

```bash
shellboto simulate 'your command here'
```

Reports the matched pattern (if any) with no side effects.

## Read next

- [../security/danger-matcher.md](../security/danger-matcher.md)
  — every regex with worked example + rationale.
