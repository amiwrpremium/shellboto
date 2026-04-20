package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsUsableDir_NonExistent(t *testing.T) {
	// Nothing at the path yet → safe to let MkdirAll create it.
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist", "nested")
	ok, reason := isUsableDir(path)
	if !ok {
		t.Fatalf("non-existent path should be usable, got reason=%q", reason)
	}
}

func TestIsUsableDir_RegularDir(t *testing.T) {
	dir := t.TempDir()
	ok, reason := isUsableDir(dir)
	if !ok {
		t.Fatalf("regular dir should be usable, got reason=%q", reason)
	}
}

// TestIsUsableDir_Symlink locks in symlink rejection: if a symlink
// ever passes isUsableDir, a shellboto-user could redirect the bot's
// chown to /etc and escalate.
func TestIsUsableDir_Symlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink not supported here: %v", err)
	}
	ok, reason := isUsableDir(link)
	if ok {
		t.Fatalf("symlink should NOT be usable")
	}
	if !strings.Contains(reason, "symlink") {
		t.Fatalf("reason %q doesn't mention symlink", reason)
	}
}

func TestIsUsableDir_RegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	ok, reason := isUsableDir(path)
	if ok {
		t.Fatalf("regular file should not be usable as a dir")
	}
	if !strings.Contains(reason, "not a directory") {
		t.Fatalf("reason %q doesn't mention not-a-directory", reason)
	}
}

func TestNameRegex_Accepts(t *testing.T) {
	ok := []string{
		"Alice",
		"Alice Smith",
		"Bob Jones Doe",
		"A",
		"MARY JO",
		"lowercase single",
		"Mixed Case Stuff",
	}
	for _, s := range ok {
		if !nameRegex.MatchString(s) {
			t.Errorf("should accept %q", s)
		}
	}
}

func TestNameRegex_Rejects(t *testing.T) {
	bad := []string{
		"",                 // empty
		" Alice",           // leading space
		"Alice ",           // trailing space
		"Alice  Smith",     // double space
		"Alice\tSmith",     // tab
		"Alice\nSmith",     // newline
		"Alice Smith!",     // punctuation
		"Zoë",              // accent
		"O'Brien",          // apostrophe
		"Anne-Marie",       // hyphen
		"alice42",          // digit
		"@alice",           // at-mention
		"Alice@Bob",        // at
		"Alice_Smith",      // underscore
		"Admin\u202Ecilar", // RTL override
		"\uFEFFBob",        // BOM
		"Bob\u200Bname",    // zero-width space
		"中文",               // CJK
		"",                 // empty again (sanity)
	}
	for _, s := range bad {
		if nameRegex.MatchString(s) {
			t.Errorf("should reject %q", s)
		}
	}
}

func TestEscalationRegex(t *testing.T) {
	hits := []string{
		"sudo rm -rf /tmp",
		"echo x | sudo tee /etc/foo",
		"SUDO_ASKPASS=/bin/false sudo -n -u root whoami",
		"su root -c whoami",
		"su - root",
		"pkexec apt-get install curl",
		"doas cat /etc/shadow",
		"runuser -u root -- whoami",
		"setpriv --reuid=0 whoami",
	}
	for _, c := range hits {
		if !escalationRegex.MatchString(c) {
			t.Errorf("should match escalation: %q", c)
		}
	}
}

func TestEscalationRegexNoFalsePositives(t *testing.T) {
	misses := []string{
		"grep -i supervisor logs", // "su" is inside "supervisor", not a word
		"pseudo_thing --help",     // "su" is inside "pseudo", not a word
		"submarine.sh",            // "su" is inside "submarine"
		"subway-login",            // "subway"
		"cat user_data.txt",       // "user" (not runuser)
		"echo hello",              // nothing related
		"ls -la sudoers.d.bak",    // "sudo" is inside a filename, should match \bsudo\b though.
	}
	// "ls -la sudoers.d.bak" DOES match \bsudo\b because "sudoers" starts
	// with "sudo\b" — sudo is followed by "ers" which is word so... wait,
	// \b is transition between word and non-word; "sudo" followed by "e"
	// is word→word, NO boundary. So it doesn't match. Let's verify.
	// If a user writes "ls -la sudoers.d.bak", regex should not match.
	for _, c := range misses {
		if escalationRegex.MatchString(c) {
			t.Errorf("should NOT match escalation: %q → matched %q",
				c, escalationRegex.FindString(c))
		}
	}
}

func TestShellCwdErrorsOnMissingPID(t *testing.T) {
	// PID 0 exists as a placeholder but has no /proc entry a normal
	// user can readlink. PID 1 is init; it exists, so we pick a high
	// unlikely-to-exist PID instead.
	_, err := ShellCwd(999_999_999)
	if err == nil {
		t.Fatalf("ShellCwd(bogus pid) should error")
	}
}

func TestShellCwdReturnsForOwnProcess(t *testing.T) {
	// The test process itself has a readable /proc/self/cwd.
	cwd, err := ShellCwd(os.Getpid())
	if err != nil {
		t.Fatalf("ShellCwd(self): %v", err)
	}
	if cwd == "" {
		t.Fatalf("ShellCwd(self) returned empty")
	}
}

func TestIsPrintableASCII(t *testing.T) {
	ok := []string{
		"",
		"ls -la /tmp",
		"echo hello world",
		"hello\nworld",     // LF
		"tabs\there",       // tab
		"carriage\rreturn", // CR
		"!@#$%^&*()[]{}|\\:;<>,.?/",
	}
	for _, s := range ok {
		if !isPrintableASCII(s) {
			t.Errorf("expected ascii-ok: %q", s)
		}
	}
	bad := []string{
		"café",            // é
		"ѕudo whoami",     // Cyrillic s
		"null\x00byte",    // null
		"bell\x07hi",      // BEL
		"del\x7fchar",     // DEL
		"日本語",             // CJK
		"zero\u200bwidth", // zero-width space
	}
	for _, s := range bad {
		if isPrintableASCII(s) {
			t.Errorf("expected ascii-bad: %q", s)
		}
	}
}
