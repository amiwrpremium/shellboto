package flows

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type pending struct {
	cmd     string
	expires time.Time
}

// ConfirmStore holds short-lived confirm tokens for dangerous commands.
// Tokens are minted on danger-match and consumed via the inline [Run]
// button's callback data. Never persisted — restart wipes every pending
// confirmation.
type ConfirmStore struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[int64]map[string]pending
}

func NewConfirmStore(ttl time.Duration) *ConfirmStore {
	return &ConfirmStore{ttl: ttl, m: map[int64]map[string]pending{}}
}

func (c *ConfirmStore) Stash(userID int64, cmd string) string {
	tok := newToken()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m[userID] == nil {
		c.m[userID] = map[string]pending{}
	}
	c.m[userID][tok] = pending{cmd: cmd, expires: time.Now().Add(c.ttl)}
	return tok
}

type ClaimResult int

const (
	ClaimOK ClaimResult = iota
	ClaimUnknown
	ClaimExpired
)

func (c *ConfirmStore) Claim(userID int64, token string) (string, ClaimResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	byUser, ok := c.m[userID]
	if !ok {
		return "", ClaimUnknown
	}
	p, ok := byUser[token]
	delete(byUser, token)
	if !ok {
		return "", ClaimUnknown
	}
	if time.Now().After(p.expires) {
		return "", ClaimExpired
	}
	return p.cmd, ClaimOK
}

func (c *ConfirmStore) Sweep() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for u, byUser := range c.m {
		for t, p := range byUser {
			if now.After(p.expires) {
				delete(byUser, t)
			}
		}
		if len(byUser) == 0 {
			delete(c.m, u)
		}
	}
}

// DropByUser removes every pending confirm token for a single user. Wired
// to shell.Manager.ShellGoneHook so that destroying a user's shell (idle
// reap, /reset, promote/demote, ban) also invalidates their pending
// danger confirmation — stops a post-reap tap from dispatching the
// stashed command into a freshly-respawned shell with different env / uid.
func (c *ConfirmStore) DropByUser(userID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, userID)
}

// newToken mints a 16-hex-char (64-bit) random token. Used for danger
// confirm and /adduser wizard tokens. 64 bits is safely beyond brute-
// force given Telegram's message-rate cap and our per-user TTL, with
// zero operational cost over the previous 32-bit size.
func newToken() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
