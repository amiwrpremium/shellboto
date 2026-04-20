package redact

import (
	"strings"
	"testing"
)

func TestRedactPasswordsAndTokens(t *testing.T) {
	cases := []struct {
		in, wantSubstr, mustNotContain string
	}{
		{
			in:             `mysql -uroot -pS3cr3tP4ssw0rd -e "SELECT 1"`,
			wantSubstr:     `-p[REDACTED]`,
			mustNotContain: `S3cr3tP4ssw0rd`,
		},
		{
			in:             `curl --password=hunter2 https://example.com`,
			wantSubstr:     `--password=[REDACTED]`,
			mustNotContain: `hunter2`,
		},
		{
			in:             `PGPASSWORD=super_secret psql -c 'SELECT 1'`,
			wantSubstr:     `[REDACTED]`,
			mustNotContain: `super_secret`,
		},
		{
			in:             `export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`,
			wantSubstr:     `[REDACTED]`,
			mustNotContain: `wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`,
		},
		{
			in:             `echo "AKIAIOSFODNN7EXAMPLE"`,
			wantSubstr:     `[REDACTED-AWS-KEY]`,
			mustNotContain: `AKIAIOSFODNN7EXAMPLE`,
		},
		{
			in:             `GITHUB_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz0123456789`,
			wantSubstr:     `[REDACTED`,
			mustNotContain: `ghp_abcdefghijklmnopqrstuvwxyz0123456789`,
		},
		{
			in:             `Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.abc.def`,
			wantSubstr:     `[REDACTED`,
			mustNotContain: `eyJhbGciOiJIUzI1NiJ9.abc.def`,
		},
		{
			in:             `postgres://bob:supersecret@db.example:5432/app`,
			wantSubstr:     `postgres://[REDACTED]:[REDACTED]@`,
			mustNotContain: `supersecret`,
		},
		{
			in:             `root:$6$abc123$longhashvalueGoesHere:19000:0:99999:7:::`,
			wantSubstr:     `root:[REDACTED-HASH]`,
			mustNotContain: `$6$abc123$longhashvalueGoesHere`,
		},
		{
			in:             "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAE\n-----END OPENSSH PRIVATE KEY-----",
			wantSubstr:     `[REDACTED-PRIVATE-KEY]`,
			mustNotContain: `b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAE`,
		},
	}

	for i, c := range cases {
		got := RedactString(c.in)
		if !strings.Contains(got, c.wantSubstr) {
			t.Errorf("[%d] %q → %q; expected to contain %q", i, c.in, got, c.wantSubstr)
		}
		if strings.Contains(got, c.mustNotContain) {
			t.Errorf("[%d] %q → %q; secret %q still present", i, c.in, got, c.mustNotContain)
		}
	}
}

func TestRedactNoopOnSafeText(t *testing.T) {
	safe := []string{
		"",
		"ls -la /tmp",
		"echo hello world",
		"cat /etc/hosts",
		"grep abc /var/log/messages",
		"pwd",
	}
	for _, s := range safe {
		if got := RedactString(s); got != s {
			t.Errorf("safe input %q was modified → %q", s, got)
		}
	}
}

func TestRedactEmpty(t *testing.T) {
	if got := Redact(nil); len(got) != 0 {
		t.Errorf("Redact(nil) = %v, want empty", got)
	}
	if got := Redact([]byte{}); len(got) != 0 {
		t.Errorf("Redact(empty) = %v, want empty", got)
	}
}

func TestStripTerminalEscapesCSI(t *testing.T) {
	// CSI: ESC [ params ; intermediates final. Two common examples:
	// ESC[2J clears screen, ESC[0;0H homes cursor, ESC[31m sets fg red.
	in := "hello\x1b[2J\x1b[0;0H\x1b[31mworld\x1b[0m"
	got := StripTerminalEscapesString(in)
	if got != "helloworld" {
		t.Fatalf("CSI strip = %q, want %q", got, "helloworld")
	}
}

func TestStripTerminalEscapesOSC(t *testing.T) {
	// OSC terminated by BEL: ESC ] 0 ; title BEL — common window-title tattoo.
	in := "before\x1b]0;Window Title\x07after"
	got := StripTerminalEscapesString(in)
	if got != "beforeafter" {
		t.Fatalf("OSC/BEL strip = %q, want %q", got, "beforeafter")
	}
	// OSC terminated by ST (ESC\).
	in2 := "a\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\b"
	got2 := StripTerminalEscapesString(in2)
	if got2 != "alinkb" {
		t.Fatalf("OSC/ST strip = %q, want %q", got2, "alinkb")
	}
}

func TestStripTerminalEscapesSingleByteC1(t *testing.T) {
	// Single-byte C1 (ESC@, ESCM, etc.) — less common but should strip.
	in := "x\x1bMy\x1b=z"
	got := StripTerminalEscapesString(in)
	if got != "xyz" {
		t.Fatalf("C1 strip = %q, want %q", got, "xyz")
	}
}

func TestStripTerminalEscapesBEL(t *testing.T) {
	in := "ding\x07dong\x07"
	got := StripTerminalEscapesString(in)
	if got != "dingdong" {
		t.Fatalf("BEL strip = %q, want %q", got, "dingdong")
	}
}

func TestStripTerminalEscapesLeavesPlainText(t *testing.T) {
	safe := []string{
		"",
		"ls -la /tmp",
		"hello world\n",
		"tabs\there\nnewlines\n",
		"plain cat /etc/hosts output",
	}
	for _, s := range safe {
		if got := StripTerminalEscapesString(s); got != s {
			t.Errorf("plain %q changed → %q", s, got)
		}
	}
}
