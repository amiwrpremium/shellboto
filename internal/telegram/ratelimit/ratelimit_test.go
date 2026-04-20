package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterBurstThenReject(t *testing.T) {
	l := New(3, 0.0) // cap=3, no refill (slot stays empty once drained)
	for i := 0; i < 3; i++ {
		if !l.Allow(42) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	// 4th request: bucket empty, no refill.
	if l.Allow(42) {
		t.Fatalf("4th request should be rejected")
	}
}

func TestLimiterRefill(t *testing.T) {
	l := New(2, 100.0) // cap=2, refill=100/sec → ~every 10ms
	if !l.Allow(1) {
		t.Fatal("first call")
	}
	if !l.Allow(1) {
		t.Fatal("second call (burst)")
	}
	if l.Allow(1) {
		t.Fatal("third call should be rejected (bucket drained)")
	}
	time.Sleep(25 * time.Millisecond)
	if !l.Allow(1) {
		t.Fatal("after sleep, refill should have restored at least 1 token")
	}
}

func TestLimiterIndependentKeys(t *testing.T) {
	l := New(1, 0.0)
	if !l.Allow(1) {
		t.Fatal("user 1 first call")
	}
	if l.Allow(1) {
		t.Fatal("user 1 second call should be rejected")
	}
	// user 2 has their own bucket.
	if !l.Allow(2) {
		t.Fatal("user 2 first call should be allowed (independent bucket)")
	}
}

func TestLimiterDisabledWhenCapacityZero(t *testing.T) {
	l := New(0, 1.0)
	for i := 0; i < 10_000; i++ {
		if !l.Allow(1) {
			t.Fatalf("disabled limiter rejected call %d", i)
		}
	}
	if l.Enabled() {
		t.Fatalf("Enabled() should be false with capacity=0")
	}
}

func TestLimiterSweepByAge(t *testing.T) {
	l := New(5, 10.0)
	l.Allow(1)
	if l.Len() != 1 {
		t.Fatalf("expected 1 bucket after Allow")
	}
	// Idle cutoff further than elapsed → not yet stale.
	l.Sweep(time.Hour)
	if l.Len() != 1 {
		t.Fatalf("bucket should not be reaped within idle window")
	}
	time.Sleep(30 * time.Millisecond)
	l.Sweep(10 * time.Millisecond)
	if l.Len() != 0 {
		t.Fatalf("stale bucket should be reaped, got len=%d", l.Len())
	}
}
