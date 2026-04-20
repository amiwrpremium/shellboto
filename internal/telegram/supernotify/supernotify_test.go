package supernotify

import (
	"context"
	"strings"
	"testing"
	"time"

	tgm "github.com/go-telegram/bot/models"
	"go.uber.org/zap"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	ns "github.com/amiwrpremium/shellboto/internal/telegram/namespaces"
)

func TestBuildTargetActorKB_BothRowsWhenDifferent(t *testing.T) {
	target := int64(99)
	actor := int64(10)
	kb := buildTargetActorKB(target, []rowSpec{
		{text: "A", data: ns.CBData(ns.Demote, ns.Select, target)},
	}, actor, []rowSpec{
		{text: "B", data: ns.CBData(ns.Demote, ns.Select, actor)},
	})
	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(kb.InlineKeyboard))
	}
	if kb.InlineKeyboard[0][0].CallbackData != ns.CBData(ns.Demote, ns.Select, target) {
		t.Errorf("row 0 button 0 callback = %q", kb.InlineKeyboard[0][0].CallbackData)
	}
	if kb.InlineKeyboard[1][0].CallbackData != ns.CBData(ns.Demote, ns.Select, actor) {
		t.Errorf("row 1 button 0 callback = %q", kb.InlineKeyboard[1][0].CallbackData)
	}
}

func TestBuildTargetActorKB_CollapsesWhenSame(t *testing.T) {
	id := int64(42)
	kb := buildTargetActorKB(id, []rowSpec{
		{text: "only", data: "d"},
	}, id, []rowSpec{
		{text: "should not appear", data: "d2"},
	})
	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("expected 1 row when target==actor, got %d", len(kb.InlineKeyboard))
	}
	if kb.InlineKeyboard[0][0].Text != "only" {
		t.Errorf("wrong row kept: %+v", kb.InlineKeyboard[0])
	}
}

func TestBannedKB_TargetOnly(t *testing.T) {
	// Exercise Banned's KB shape via a direct construction using the
	// same buttons as the real Banned() emits.
	target := &dbm.User{TelegramID: 99, Role: dbm.RoleUser}
	kb := &tgm.InlineKeyboardMarkup{InlineKeyboard: [][]tgm.InlineKeyboardButton{{
		{Text: "⬆ Reinstate", CallbackData: ns.CBData(ns.Users, ns.Reinstate, target.TelegramID)},
		{Text: "📜 Audit", CallbackData: ns.CBData(ns.Users, ns.Audit, target.TelegramID)},
		{Text: "👁 Profile", CallbackData: ns.CBData(ns.Users, ns.Profile, target.TelegramID)},
	}}}
	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("banned kb should have 1 row, got %d", len(kb.InlineKeyboard))
	}
	if len(kb.InlineKeyboard[0]) != 3 {
		t.Fatalf("banned row should have 3 buttons, got %d", len(kb.InlineKeyboard[0]))
	}
}

func TestDisplayLabel_PrefixesRoleEmoji(t *testing.T) {
	super := &dbm.User{TelegramID: 1, Role: dbm.RoleSuperadmin, Name: "Sam"}
	admin := &dbm.User{TelegramID: 2, Role: dbm.RoleAdmin, Name: "Alice"}
	user := &dbm.User{TelegramID: 3, Role: dbm.RoleUser, Name: "Bob"}
	if !strings.HasPrefix(displayLabel(super), "👑") {
		t.Errorf("super label missing crown: %q", displayLabel(super))
	}
	if !strings.HasPrefix(displayLabel(admin), "⚡") {
		t.Errorf("admin label missing bolt: %q", displayLabel(admin))
	}
	if !strings.HasPrefix(displayLabel(user), "👤") {
		t.Errorf("user label missing bust: %q", displayLabel(user))
	}
}

func TestTTLStore_TrackAndLenDisabled(t *testing.T) {
	s := NewTTLStore(0)
	if s.Enabled() {
		t.Fatalf("TTL=0 should disable")
	}
	s.Track(1, 2)
	if s.Len() != 0 {
		t.Fatalf("Track on disabled store should be no-op, got len=%d", s.Len())
	}
}

func TestTTLStore_TrackIncrements(t *testing.T) {
	s := NewTTLStore(time.Minute)
	s.Track(1, 10)
	s.Track(2, 20)
	if s.Len() != 2 {
		t.Fatalf("expected 2 tracked, got %d", s.Len())
	}
}

func TestTTLStore_TakeExpiredPartitions(t *testing.T) {
	s := NewTTLStore(time.Hour) // long enough that Track entries start "unexpired"
	s.Track(1, 10)
	s.Track(2, 20)
	s.Track(3, 30)
	// Push a "now" past the TTL of all entries → all three should be
	// returned as expired and the store should be emptied.
	future := time.Now().Add(2 * time.Hour)
	got := s.takeExpired(future)
	if len(got) != 3 {
		t.Fatalf("takeExpired returned %d, want 3", len(got))
	}
	if s.Len() != 0 {
		t.Fatalf("store should be empty after take, got len=%d", s.Len())
	}
}

func TestTryEnqueue_FitsUpToCap(t *testing.T) {
	q := make(chan pendingSend, 3)
	for i := 0; i < 3; i++ {
		queued, dropped := tryEnqueue(q, pendingSend{text: string(rune('a' + i))})
		if !queued {
			t.Fatalf("fill %d: queued=false, want true", i)
		}
		if dropped {
			t.Fatalf("fill %d: dropped=true, want false (not at cap yet)", i)
		}
	}
	if len(q) != 3 {
		t.Fatalf("q len = %d, want 3", len(q))
	}
}

func TestTryEnqueue_OverflowDropsOldestAndSeatsNew(t *testing.T) {
	q := make(chan pendingSend, 3)
	// Fill to cap with a, b, c.
	tryEnqueue(q, pendingSend{text: "a"})
	tryEnqueue(q, pendingSend{text: "b"})
	tryEnqueue(q, pendingSend{text: "c"})

	// Overflow: d should kick out a (the oldest).
	queued, dropped := tryEnqueue(q, pendingSend{text: "d"})
	if !queued {
		t.Fatalf("overflow: queued=false, want true after drop-oldest")
	}
	if !dropped {
		t.Fatalf("overflow: dropped=false, want true (should have kicked oldest)")
	}
	if len(q) != 3 {
		t.Fatalf("q len after overflow = %d, want 3", len(q))
	}

	// Drain and verify FIFO order — expect [b, c, d] (a was evicted).
	close(q)
	var got []string
	for r := range q {
		got = append(got, r.text)
	}
	want := []string{"b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("pos %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

// TestSendOneRecoversPanic locks in sendOne's panic swallowing: the
// worker goroutine must keep draining even if SendMessage panics. We
// force a panic by passing a nil *bot.Bot (b.SendMessage on a nil
// receiver nil-derefs).
func TestSendOneRecoversPanic(t *testing.T) {
	e := &Emitter{Log: zap.NewNop()}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic escaped sendOne's recover: %v", r)
		}
	}()
	e.sendOne(nil, 42, pendingSend{text: "won't send; nil-bot nil-derefs"})
}

func TestEnsureQueue_SpawnsWorkerOncePerRecipient(t *testing.T) {
	// Use an Emitter with nil Users / nil bot — we only exercise the
	// channel-creation path, not the worker's send. The worker WILL
	// spawn and block on the channel forever; that's fine for a test
	// since it does nothing while the channel is empty + the process
	// exits at test-end.
	e := &Emitter{Log: zap.NewNop()}
	q1 := e.ensureQueue(nil, 42)
	q2 := e.ensureQueue(nil, 42)
	if q1 != q2 {
		t.Fatalf("ensureQueue returned two different channels for the same recipient")
	}
	q3 := e.ensureQueue(nil, 43)
	if q3 == q1 {
		t.Fatalf("ensureQueue returned the same channel for different recipients")
	}
}

func TestTTLStore_TakeExpiredLeavesUnexpired(t *testing.T) {
	s := NewTTLStore(time.Hour)
	s.Track(1, 10)
	// "now" before TTL elapsed → none expired.
	got := s.takeExpired(time.Now())
	if len(got) != 0 {
		t.Fatalf("takeExpired at t<expiry returned %d, want 0", len(got))
	}
	if s.Len() != 1 {
		t.Fatalf("store should still hold the entry, got len=%d", s.Len())
	}
}

// --- Drain regression tests ----------------------------------------------

// TestDrain_FlushesPendingBacklog verifies that Drain closes worker
// queues, causes workers to finish their backlog without the rate-limit
// sleep, and returns cleanly within its ctx budget. Uses a nil bot so
// sendOne's panic-recover path makes each "send" microsecond-cheap —
// we're exercising the drain plumbing, not the network path.
func TestDrain_FlushesPendingBacklog(t *testing.T) {
	e := &Emitter{Log: zap.NewNop()}

	// Spawn a worker by enqueuing 10 messages. interSendInterval
	// would make this take 11+ seconds at normal rates; Drain should
	// cut that to ~microseconds by skipping the sleep.
	for i := 0; i < 10; i++ {
		e.enqueue(nil, 42, "msg", nil)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	e.Drain(ctx)
	elapsed := time.Since(start)
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("Drain took %s — rate-limit sleep was not skipped", elapsed)
	}

	// The worker's channel must now be closed; reading drains zero
	// value with ok=false.
	raw, ok := e.workers.Load(int64(42))
	if !ok {
		t.Fatalf("worker entry missing after drain")
	}
	q := raw.(*workerHandle).ch
	if _, open := <-q; open {
		t.Fatalf("worker channel is still open after drain")
	}
}

// --- ReapIdle regression tests ----------------------------------------

// TestReapIdle_ReapsIdleWorker spawns a worker, lets it finish processing,
// backdates the lastUsed timestamp, and verifies ReapIdle closes the
// channel and removes the map entry.
func TestReapIdle_ReapsIdleWorker(t *testing.T) {
	e := &Emitter{Log: zap.NewNop()}
	e.enqueue(nil, 42, "x", nil)

	// Let the worker drain (nil bot → panic-recover fast path, then
	// interSendInterval sleep; give it room).
	waitForEmptyQueue(t, e, 42, 2*time.Second)

	// Backdate lastUsed well past any reasonable maxIdle.
	raw, ok := e.workers.Load(int64(42))
	if !ok {
		t.Fatalf("worker entry missing — drain or reap raced the test")
	}
	h := raw.(*workerHandle)
	h.lastUsed.Store(time.Now().Add(-10 * time.Hour).UnixNano())

	n := e.ReapIdle(time.Minute)
	if n != 1 {
		t.Fatalf("ReapIdle returned %d, want 1", n)
	}
	if _, still := e.workers.Load(int64(42)); still {
		t.Fatalf("worker entry still present after reap")
	}
	// Channel must be closed — draining returns zero value + ok=false.
	if _, open := <-h.ch; open {
		t.Fatalf("worker channel still open after reap")
	}
}

// TestReapIdle_KeepsActiveWorker verifies a worker that was recently
// active is NOT reaped.
func TestReapIdle_KeepsActiveWorker(t *testing.T) {
	e := &Emitter{Log: zap.NewNop()}
	e.enqueue(nil, 77, "x", nil)
	waitForEmptyQueue(t, e, 77, 2*time.Second)

	// Worker's lastUsed is at "just now" — maxIdle=1h must not reap it.
	n := e.ReapIdle(time.Hour)
	if n != 0 {
		t.Fatalf("ReapIdle reaped %d active workers, want 0", n)
	}
	if _, ok := e.workers.Load(int64(77)); !ok {
		t.Fatalf("active worker was reaped")
	}
}

// TestReapIdle_SpawnsFreshAfterReap verifies that after a worker is
// reaped, a subsequent enqueue for the same recipient spawns a fresh
// handle — not a reuse of the closed channel.
func TestReapIdle_SpawnsFreshAfterReap(t *testing.T) {
	e := &Emitter{Log: zap.NewNop()}

	e.enqueue(nil, 99, "first", nil)
	waitForEmptyQueue(t, e, 99, 2*time.Second)

	// Backdate + reap.
	raw, _ := e.workers.Load(int64(99))
	oldHandle := raw.(*workerHandle)
	oldHandle.lastUsed.Store(time.Now().Add(-10 * time.Hour).UnixNano())
	if n := e.ReapIdle(time.Second); n != 1 {
		t.Fatalf("ReapIdle returned %d, want 1", n)
	}

	// Re-enqueue for the same recipient. ensureQueue must create a
	// brand-new handle; the old closed channel must NOT be reused.
	e.enqueue(nil, 99, "second", nil)
	raw2, ok := e.workers.Load(int64(99))
	if !ok {
		t.Fatalf("no new worker after post-reap enqueue")
	}
	newHandle := raw2.(*workerHandle)
	if newHandle == oldHandle {
		t.Fatalf("old handle reused after reap — ensureQueue didn't spawn fresh")
	}
	waitForEmptyQueue(t, e, 99, 2*time.Second)
}

// waitForEmptyQueue polls until the recipient's channel is drained.
// Tests use this to synchronise against the worker goroutine without
// introducing arbitrary sleeps.
func waitForEmptyQueue(t *testing.T, e *Emitter, recipient int64, budget time.Duration) {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		raw, ok := e.workers.Load(recipient)
		if !ok {
			return // worker gone (reaped?) — nothing to wait for
		}
		if len(raw.(*workerHandle).ch) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("queue for recipient %d still has items after %s", recipient, budget)
}

// TestDrain_RejectsNewEnqueuesAfterStart locks in that post-Drain
// enqueue calls are silently dropped — no new worker spawns, no entries
// land in the workers map.
func TestDrain_RejectsNewEnqueuesAfterStart(t *testing.T) {
	e := &Emitter{Log: zap.NewNop()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	e.Drain(ctx)

	// After Drain, any enqueue must be a no-op.
	e.enqueue(nil, 99, "late arrival", nil)
	e.enqueue(nil, 100, "another late arrival", nil)

	empty := true
	e.workers.Range(func(_, _ any) bool {
		empty = false
		return false
	})
	if !empty {
		t.Fatalf("workers map has entries after post-drain enqueue — new enqueues were not rejected")
	}
}

// TestDrain_IsIdempotent asserts a second Drain call is a no-op (does
// not panic on double-close of channels, does not block waiting for
// already-exited workers).
func TestDrain_IsIdempotent(t *testing.T) {
	e := &Emitter{Log: zap.NewNop()}
	// Spawn a worker so there's a channel to accidentally double-close.
	e.enqueue(nil, 7, "x", nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	e.Drain(ctx)

	// Second call must return promptly with no panic.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	start := time.Now()
	e.Drain(ctx2)
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("second Drain call took %s — should be a no-op", elapsed)
	}
}
