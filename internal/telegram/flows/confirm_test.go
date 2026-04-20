package flows

import (
	"testing"
	"time"
)

func TestConfirmStashAndClaim(t *testing.T) {
	c := NewConfirmStore(time.Minute)
	tok := c.Stash(42, "rm -rf /tmp/x")
	cmd, res := c.Claim(42, tok)
	if res != ClaimOK {
		t.Fatalf("Claim res = %v, want ClaimOK", res)
	}
	if cmd != "rm -rf /tmp/x" {
		t.Fatalf("Claim cmd = %q", cmd)
	}
	// Second claim on the same token must not succeed (one-shot).
	if _, res := c.Claim(42, tok); res != ClaimUnknown {
		t.Fatalf("second claim res = %v, want ClaimUnknown", res)
	}
}

func TestConfirmExpiresClaim(t *testing.T) {
	c := NewConfirmStore(5 * time.Millisecond)
	tok := c.Stash(1, "x")
	time.Sleep(20 * time.Millisecond)
	if _, res := c.Claim(1, tok); res != ClaimExpired {
		t.Fatalf("expected ClaimExpired, got %v", res)
	}
}

func TestConfirmDropByUser(t *testing.T) {
	c := NewConfirmStore(time.Minute)
	tokA1 := c.Stash(1, "cmd-a1")
	tokA2 := c.Stash(1, "cmd-a2")
	tokB := c.Stash(2, "cmd-b")

	c.DropByUser(1)

	// User 1's tokens are gone.
	for _, tok := range []string{tokA1, tokA2} {
		if _, res := c.Claim(1, tok); res != ClaimUnknown {
			t.Fatalf("user1 token %q should be gone after DropByUser, got %v", tok, res)
		}
	}
	// User 2's token is untouched.
	if cmd, res := c.Claim(2, tokB); res != ClaimOK || cmd != "cmd-b" {
		t.Fatalf("user2 drop leak: cmd=%q res=%v", cmd, res)
	}
}
