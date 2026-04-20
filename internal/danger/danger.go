package danger

import (
	"fmt"
	"regexp"
)

// defaults is the built-in regex list of "probably destructive" commands.
// A match triggers /confirm for admin+ callers and an auto-ban for users.
//
// Honest scope: regexes can never close the hole — bash syntax tricks
// (variable indirection, base64 → sh, $'\xNN' composition, eval of
// reassembled strings) defeat any pattern. The real defense against a
// user-role caller running destructive things as root is OS-level
// isolation: user-role shells are non-root (see user_shell_user), so
// most of these commands fail at the OS level even when the regex is
// bypassed. This list is the typo-guard for admins plus a speedbump
// for the lazy attacker.
var defaults = []string{
	// Destructive disk/device writes.
	`\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f|-[a-zA-Z]*f[a-zA-Z]*r)\b`,
	`\bdd\b.*\bof=/dev/`,
	`\bmkfs\.`,
	`\b(fdisk|parted|wipefs)\b`,
	`>\s*["']?/dev/(sd|nvme|mmcblk|vd|xvd)`,             // tolerate quoted path
	`\btee\s+(-a\s+)?["']?/dev/(sd|nvme|mmcblk|vd|xvd)`, // tee to raw disk

	// Power / init.
	`\b(shutdown|reboot|halt|poweroff)\b`,
	`\binit\s+[06]\b`, // init 0 / init 6

	// Wholesale chmod/chown of /.
	`\b(chown|chmod)\s+-R\s+/\s*($|[^a-zA-Z0-9_])`,

	// Piping into a shell (curl | sh etc).
	`\|\s*(ba|z|k|c|d|)sh\b`,

	// Users / creds.
	`\buserdel\b`,
	`\bpasswd\s+root\b`,

	// Fork bomb (classic form).
	`:\(\)\s*\{\s*:\|:&\s*\};:`,

	// Firewall wipe + SSH shutdown.
	`\biptables\s+-F\b`,
	`\bsystemctl\s+(stop|disable|mask)\s+(ssh|sshd)\b`,

	// Overwrites of core auth / ssh files.
	`>\s*["']?/etc/(shadow|passwd|sudoers|group|gshadow)\b`,
	`>\s*["']?/etc/sudoers\.d/`,
	`>\s*["']?/root/\.ssh/`,

	// Find-based deletion / exec.
	`\bfind\b[^\n]*\s-delete\b`,
	`\bfind\b[^\n]*\s-exec\s+rm\b`,

	// Zero-length truncation.
	`\btruncate\s+-s\s*0\b`,

	// Scripting-language one-liners. `-e/-c/-E/-r/-R` cover perl,
	// python, ruby, node, php one-liner flags. Admins use these
	// frequently; false positives mean admin taps /confirm often.
	// Tradeoff accepted.
	`\b(perl|python[23]?|ruby|node|php)\s+-[ecErR]\b`,

	// Immutable-flag flip (hides tampering from routine tools).
	`\bchattr\s+[+-]i\b`,

	// Obvious reverse-shell / listener patterns.
	`\b(nc|ncat|netcat)\s+-[elL]\b`,
	// `/dev/tcp/` is a bash built-in that opens TCP sockets. It has
	// essentially no legitimate use outside reverse shells / one-off
	// connectivity tests. Matched anywhere in the command (handles
	// `bash -i >& /dev/tcp/...`, `exec 5<>/dev/tcp/...`, etc.)
	`/dev/tcp/`,
}

type Matcher struct {
	patterns []*regexp.Regexp
}

func New(extra []string) (*Matcher, error) {
	all := append([]string{}, defaults...)
	all = append(all, extra...)
	m := &Matcher{}
	for _, p := range all {
		r, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("danger pattern %q: %w", p, err)
		}
		m.patterns = append(m.patterns, r)
	}
	return m, nil
}

func (m *Matcher) Match(cmd string) (string, bool) {
	for _, r := range m.patterns {
		if r.MatchString(cmd) {
			return r.String(), true
		}
	}
	return "", false
}
