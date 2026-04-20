// Package ratelimit provides a per-key token-bucket rate limiter used to
// cap how fast any single Telegram user can drive the bot.
//
// Cost model: every non-exempt handler invocation spends exactly one
// token. With burst=10 and refill=1/sec the steady state is 60 actions
// per minute with short spikes absorbed, matching typical ops cadence.
//
// Signal-type actions (/cancel, /kill, and their inline-button
// equivalents) are exempted at the middleware layer — a user must be
// able to interrupt a runaway command even after being rate-limited.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter is a per-key token bucket. Zero value is NOT usable; use New.
type Limiter struct {
	capacity float64
	refill   float64 // tokens per second
	mu       sync.Mutex
	buckets  map[int64]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

// New constructs a limiter. `capacity` is the burst size; `refillPerSec`
// is the steady-state refill rate. A capacity of 0 disables limiting
// entirely (Allow always returns true).
func New(capacity int, refillPerSec float64) *Limiter {
	return &Limiter{
		capacity: float64(capacity),
		refill:   refillPerSec,
		buckets:  map[int64]*bucket{},
	}
}

// Enabled reports whether Allow will ever return false.
func (l *Limiter) Enabled() bool { return l != nil && l.capacity > 0 }

// Allow reports whether `key` may spend one token right now. Refills
// the bucket based on elapsed time since last access. When rate-limit
// is disabled (capacity=0), always returns true.
func (l *Limiter) Allow(key int64) bool {
	if !l.Enabled() {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.capacity, last: now}
		l.buckets[key] = b
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * l.refill
	if b.tokens > l.capacity {
		b.tokens = l.capacity
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens -= 1
	return true
}

// Sweep drops buckets that haven't been touched for `idle`. Reaping by
// age alone is safe: a user who comes back after the idle window gets
// a fresh bucket at full capacity, which is strictly GENEROUS compared
// to keeping their partial state — no security regression.
func (l *Limiter) Sweep(idle time.Duration) {
	if !l.Enabled() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-idle)
	for k, b := range l.buckets {
		if b.last.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}

// Len returns the number of active buckets (for metrics / tests).
func (l *Limiter) Len() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
