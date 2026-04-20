package supernotify

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"
	"go.uber.org/zap"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/db/repo"
	ns "github.com/amiwrpremium/shellboto/internal/telegram/namespaces"
	"github.com/amiwrpremium/shellboto/internal/telegram/views"
)

const (
	// perRecipientQueueCap is the pending-DM backlog allowed per
	// recipient. Beyond this, the oldest queued entry is dropped so a
	// burst from a noisy admin doesn't crowd out fresh events.
	perRecipientQueueCap = 50

	// interSendInterval is the minimum pause between DMs to the same
	// recipient. Telegram's soft per-chat rate is ~1 msg/sec; 1.1s
	// gives enough margin to stay out of 429-retry territory.
	interSendInterval = 1100 * time.Millisecond
)

// Emitter bundles the state supernotify needs. Constructed once in
// main.go and attached to Deps so callbacks / commands can invoke
// event senders via a single handle. Kept here (not in deps) to avoid
// a deps ↔ supernotify import cycle.
type Emitter struct {
	Users   *repo.UserRepo
	Store   *TTLStore
	Log     *zap.Logger
	SuperID int64

	// workers holds per-recipient worker state. Lazy-spawned on first
	// enqueue for a recipient; the worker goroutine drains the channel
	// at ≤1 DM/sec so we never exceed Telegram's per-chat rate limit
	// no matter how bursty admin activity gets. Workers live
	// until either Drain closes their channel or the ReapIdle sweep
	// closes a long-idle one. Each handle tracks the last
	// send time atomically so reap decisions don't need any lock.
	workers sync.Map // map[int64]*workerHandle

	// mu guards `closing` and gates workers-map lifecycle during
	// Drain. RLock'd by enqueue so concurrent sends don't serialise;
	// Lock'd by Drain so the closing flip snapshots atomically.
	mu      sync.RWMutex
	closing bool

	// wg tracks live worker goroutines so Drain can wait for a clean
	// exit within a bounded budget. ensureQueue does Add(1) when it
	// spawns a worker; runWorker defers Done.
	wg sync.WaitGroup

	// drainOnce makes Drain idempotent. Second and subsequent calls
	// are no-ops.
	drainOnce sync.Once
}

// pendingSend is one queued DM waiting for a worker to deliver it.
type pendingSend struct {
	text string
	kb   *tgm.InlineKeyboardMarkup
}

// workerHandle bundles a worker's queue channel with its last-activity
// timestamp for the idle-reaper. lastUsed is a unix-nanos atomic
// so sendOne can bump it without contending on Emitter.mu; ReapIdle
// reads it while holding Emitter.mu so the close-channel/delete pair
// is race-free against enqueue.
type workerHandle struct {
	ch       chan pendingSend
	lastUsed atomic.Int64
}

// NewEmitter constructs an Emitter.
func NewEmitter(users *repo.UserRepo, store *TTLStore, log *zap.Logger, superID int64) *Emitter {
	if log == nil {
		log = zap.NewNop()
	}
	return &Emitter{Users: users, Store: store, Log: log, SuperID: superID}
}

// timestampSuffix returns a compact UTC timestamp line for the DM body.
func timestampSuffix() string {
	return "\n" + time.Now().UTC().Format("2006-01-02 15:04 UTC")
}

// Promoted DMs the chain that `actor` promoted `target` → admin.
// No-op when actor is super (top of hierarchy).
func (e *Emitter) Promoted(ctx context.Context, b *bot.Bot, actor, target *dbm.User) {
	text := fmt.Sprintf("⚡ %s promoted %s → admin",
		displayLabel(actor), displayLabel(target)) + timestampSuffix()
	kb := buildTargetActorKB(
		target.TelegramID, []rowSpec{
			{text: "⬇ Demote", data: ns.CBData(ns.Demote, ns.Select, target.TelegramID)},
			{text: "🚫 Ban", data: ns.CBData(ns.Users, ns.Remove, target.TelegramID)},
			{text: "👁 Profile", data: ns.CBData(ns.Users, ns.Profile, target.TelegramID)},
		},
		actor.TelegramID, []rowSpec{
			{text: "⬇ Demote @actor", data: ns.CBData(ns.Demote, ns.Select, actor.TelegramID)},
			{text: "🚫 Ban @actor", data: ns.CBData(ns.Users, ns.Remove, actor.TelegramID)},
			{text: "👁 Actor profile", data: ns.CBData(ns.Users, ns.Profile, actor.TelegramID)},
		},
	)
	e.fanOut(ctx, b, actor.TelegramID, text, kb)
}

// Demoted DMs the chain that `actor` demoted `target` → user.
func (e *Emitter) Demoted(ctx context.Context, b *bot.Bot, actor, target *dbm.User, cascadedCount int) {
	text := fmt.Sprintf("⬇ %s demoted %s → user",
		displayLabel(actor), displayLabel(target))
	if cascadedCount > 0 {
		text += fmt.Sprintf("\n+ %d cascaded", cascadedCount)
	}
	text += timestampSuffix()
	kb := buildTargetActorKB(
		target.TelegramID, []rowSpec{
			{text: "⬆ Promote", data: ns.CBData(ns.Promote, ns.Select, target.TelegramID)},
			{text: "🚫 Ban", data: ns.CBData(ns.Users, ns.Remove, target.TelegramID)},
			{text: "👁 Profile", data: ns.CBData(ns.Users, ns.Profile, target.TelegramID)},
		},
		actor.TelegramID, []rowSpec{
			{text: "⬇ Demote @actor", data: ns.CBData(ns.Demote, ns.Select, actor.TelegramID)},
			{text: "🚫 Ban @actor", data: ns.CBData(ns.Users, ns.Remove, actor.TelegramID)},
			{text: "👁 Actor profile", data: ns.CBData(ns.Users, ns.Profile, actor.TelegramID)},
		},
	)
	e.fanOut(ctx, b, actor.TelegramID, text, kb)
}

// Added DMs the chain that `actor` added user `targetID` (name = X).
func (e *Emitter) Added(ctx context.Context, b *bot.Bot, actor *dbm.User, targetID int64, name string) {
	targetLabel := fmt.Sprintf("id: %d", targetID)
	if name != "" {
		targetLabel = fmt.Sprintf("%s (id: %d)", name, targetID)
	}
	text := fmt.Sprintf("➕ %s added %s as user",
		displayLabel(actor), targetLabel) + timestampSuffix()
	kb := buildTargetActorKB(
		targetID, []rowSpec{
			{text: "⬆ Promote", data: ns.CBData(ns.Promote, ns.Select, targetID)},
			{text: "🚫 Remove", data: ns.CBData(ns.Users, ns.Remove, targetID)},
			{text: "👁 Profile", data: ns.CBData(ns.Users, ns.Profile, targetID)},
		},
		actor.TelegramID, []rowSpec{
			{text: "⬇ Demote @actor", data: ns.CBData(ns.Demote, ns.Select, actor.TelegramID)},
			{text: "🚫 Ban @actor", data: ns.CBData(ns.Users, ns.Remove, actor.TelegramID)},
			{text: "👁 Actor profile", data: ns.CBData(ns.Users, ns.Profile, actor.TelegramID)},
		},
	)
	e.fanOut(ctx, b, actor.TelegramID, text, kb)
}

// Removed DMs the chain that `actor` removed `target`.
func (e *Emitter) Removed(ctx context.Context, b *bot.Bot, actor, target *dbm.User) {
	text := fmt.Sprintf("🗑 %s removed %s",
		displayLabel(actor), displayLabel(target)) + timestampSuffix()
	kb := buildTargetActorKB(
		target.TelegramID, []rowSpec{
			{text: "⬆ Reinstate", data: ns.CBData(ns.Users, ns.Reinstate, target.TelegramID)},
			{text: "👁 Profile", data: ns.CBData(ns.Users, ns.Profile, target.TelegramID)},
		},
		actor.TelegramID, []rowSpec{
			{text: "⬇ Demote @actor", data: ns.CBData(ns.Demote, ns.Select, actor.TelegramID)},
			{text: "🚫 Ban @actor", data: ns.CBData(ns.Users, ns.Remove, actor.TelegramID)},
			{text: "👁 Actor profile", data: ns.CBData(ns.Users, ns.Profile, actor.TelegramID)},
		},
	)
	e.fanOut(ctx, b, actor.TelegramID, text, kb)
}

// Banned DMs the chain that `target` was banned (system-triggered).
// There is no separate actor for bans — the caller of a banned action
// is the one being banned, so the keyboard only carries target buttons.
func (e *Emitter) Banned(ctx context.Context, b *bot.Bot, target *dbm.User, reason string) {
	text := fmt.Sprintf("🚫 %s was banned (system)\nreason: %s",
		displayLabel(target), reason) + timestampSuffix()
	kb := &tgm.InlineKeyboardMarkup{InlineKeyboard: [][]tgm.InlineKeyboardButton{{
		{Text: "⬆ Reinstate", CallbackData: ns.CBData(ns.Users, ns.Reinstate, target.TelegramID)},
		{Text: "📜 Audit", CallbackData: ns.CBData(ns.Users, ns.Audit, target.TelegramID)},
		{Text: "👁 Profile", CallbackData: ns.CBData(ns.Users, ns.Profile, target.TelegramID)},
	}}}
	// For bans, the "actor" for fanout purposes is the banned user
	// themselves — fanout walks THEIR ancestry (promoter / adder) so
	// the admin who vouched for them sees the consequence.
	e.fanOut(ctx, b, target.TelegramID, text, kb)
}

// rowSpec is a compact (label, callback_data) pair used to build rows.
type rowSpec struct {
	text string
	data string
}

// buildTargetActorKB returns a keyboard with two rows: one for actions
// against the target, one for actions against the actor. Rows are
// suppressed when target and actor are the same user.
func buildTargetActorKB(targetID int64, targetRow []rowSpec, actorID int64, actorRow []rowSpec) *tgm.InlineKeyboardMarkup {
	rows := make([][]tgm.InlineKeyboardButton, 0, 2)
	rows = append(rows, toRow(targetRow))
	if actorID != targetID {
		rows = append(rows, toRow(actorRow))
	}
	return &tgm.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func toRow(specs []rowSpec) []tgm.InlineKeyboardButton {
	row := make([]tgm.InlineKeyboardButton, len(specs))
	for i, s := range specs {
		row[i] = tgm.InlineKeyboardButton{Text: s.text, CallbackData: s.data}
	}
	return row
}

// displayLabel is an emoji-prefixed short identifier for a user.
func displayLabel(u *dbm.User) string {
	return views.RoleEmoji(u) + " " + views.DisplayLabel(u)
}

// fanOut routes `text` + `kb` to every recipient returned by
// NotifyChainFor(actorID) via each recipient's worker queue. Returns
// immediately — actual delivery happens asynchronously in the
// per-recipient worker goroutines so a burst of admin activity can't
// flood Telegram's per-chat rate limit.
func (e *Emitter) fanOut(ctx context.Context, b *bot.Bot, actorID int64, text string, kb *tgm.InlineKeyboardMarkup) {
	if e == nil || e.Users == nil {
		return
	}
	recipients, err := e.Users.NotifyChainFor(actorID)
	if err != nil {
		e.Log.Warn("supernotify NotifyChainFor failed",
			zap.Int64("actor_id", actorID), zap.Error(err))
		return
	}
	for _, rid := range recipients {
		e.enqueue(b, rid, text, kb)
	}
}

// enqueue pushes a DM onto the recipient's worker queue, lazy-spawning
// the worker on first use. Drops the oldest queued entry if the queue
// is full, so newest events always make it in (stale burst state gets
// kicked out in favor of fresh state).
//
// Rejects the enqueue if Drain has started (closing flag set).
// RLock is held across ensureQueue + tryEnqueue so an idle
// reaper can't close the channel between us selecting and sending.
// `tryEnqueue` is non-blocking (select with default), so the
// critical section is microseconds.
func (e *Emitter) enqueue(b *bot.Bot, recipient int64, text string, kb *tgm.InlineKeyboardMarkup) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.closing {
		e.Log.Debug("supernotify closing — dropped enqueue",
			zap.Int64("recipient", recipient))
		return
	}
	q := e.ensureQueue(b, recipient)
	queued, dropped := tryEnqueue(q, pendingSend{text: text, kb: kb})
	if dropped {
		e.Log.Warn("supernotify queue full — dropped oldest entry",
			zap.Int64("recipient", recipient))
	}
	if !queued {
		// Extremely rare: even after drop-oldest, another goroutine
		// filled the slot. Log and move on.
		e.Log.Warn("supernotify queue saturated — dropped new entry",
			zap.Int64("recipient", recipient))
	}
}

// ensureQueue returns the recipient's worker channel, creating a
// fresh handle + spawning the worker on first call. Safe under
// concurrent callers.
func (e *Emitter) ensureQueue(b *bot.Bot, recipient int64) chan pendingSend {
	if raw, ok := e.workers.Load(recipient); ok {
		return raw.(*workerHandle).ch
	}
	h := &workerHandle{ch: make(chan pendingSend, perRecipientQueueCap)}
	h.lastUsed.Store(time.Now().UnixNano())
	actual, loaded := e.workers.LoadOrStore(recipient, h)
	winner := actual.(*workerHandle)
	if !loaded {
		// We won the race; start the worker. Losers fall through and
		// use the winner's channel. Add to the WaitGroup before the
		// goroutine starts so Drain.Wait is never racing with Add.
		e.wg.Add(1)
		go e.runWorker(b, recipient, winner)
	}
	return winner.ch
}

// runWorker drains a recipient's pending queue at ≤1 DM/sec during
// normal operation. Exits cleanly when Drain closes the channel.
//
// Each iteration is panic-guarded via sendOne. Without that,
// a single panic in b.SendMessage (e.g., a nil-deref in the upstream
// library on a malformed response) would kill the worker goroutine
// silently — the channel would keep accepting enqueues but nothing
// would drain, and every future notification to this recipient
// would be silently lost. Per-iteration recovery keeps the worker
// draining even through pathological send errors.
//
// During Drain, the closing flag is set; the rate-limit sleep
// is skipped so a small backlog empties in microseconds rather than
// seconds. A Telegram 429 under shutdown pressure is strictly better
// than losing the DM (the pre-drain behaviour).
func (e *Emitter) runWorker(b *bot.Bot, recipient int64, h *workerHandle) {
	defer e.wg.Done()
	for req := range h.ch {
		e.sendOne(b, recipient, req)
		// Bump lastUsed after each send so the reaper doesn't
		// evict a worker that's actively draining a burst.
		h.lastUsed.Store(time.Now().UnixNano())
		e.mu.RLock()
		closing := e.closing
		e.mu.RUnlock()
		if closing {
			continue
		}
		time.Sleep(interSendInterval)
	}
}

// Drain stops accepting new DMs, closes each worker's queue so its
// range loop exits after draining its backlog, and waits for every
// worker to exit or until ctx expires. Idempotent — subsequent calls
// are no-ops.
//
// The ctx deadline is the TOTAL wait across all workers, not
// per-recipient. At ~1ms per sendOne without the rate-limit sleep,
// even hundreds of queued messages drain well under a 5-second budget.
// On timeout, any still-queued messages are simply dropped when the
// process exits — same end-state as pre-drain behaviour, just
// announced in the log.
func (e *Emitter) Drain(ctx context.Context) {
	if e == nil {
		return
	}
	e.drainOnce.Do(func() {
		// 1) Flip closing under the write lock. Snapshots the worker
		//    queues under the same lock so we don't race with a
		//    just-arriving ensureQueue (blocked on its RLock in
		//    enqueue, which will read closing=true and return without
		//    ever calling ensureQueue).
		e.mu.Lock()
		e.closing = true
		var queues []chan pendingSend
		e.workers.Range(func(_, v any) bool {
			queues = append(queues, v.(*workerHandle).ch)
			return true
		})
		e.mu.Unlock()

		// 2) Close every queue — each worker's range loop drains its
		//    backlog and exits (wg.Done fires from the deferred).
		for _, q := range queues {
			close(q)
		}

		// 3) Wait for all workers with the ctx budget.
		done := make(chan struct{})
		go func() {
			e.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			e.Log.Info("supernotify drain complete",
				zap.Int("workers", len(queues)))
		case <-ctx.Done():
			e.Log.Warn("supernotify drain timed out",
				zap.Int("workers", len(queues)),
				zap.Error(ctx.Err()))
		}
	})
}

// ReapIdle closes and removes worker queues whose last send was older
// than maxIdle and whose queue is currently empty. Active workers
// (with items pending OR with a recent send) are skipped. A worker
// whose channel is closed exits its range loop naturally; the next
// enqueue for that recipient spawns a fresh handle via ensureQueue.
//
// Without this, per-admin worker goroutines accumulate over
// process lifetime (bounded by admin count, but semantically sloppy).
// With it, idle workers are reclaimed; memory and scheduler pressure
// scale with current activity rather than cumulative.
//
// Safety: takes the write lock to exclude concurrent enqueues. The
// enqueue hot-path holds RLock across ensureQueue+tryEnqueue, so it
// cannot race with our close.
func (e *Emitter) ReapIdle(maxIdle time.Duration) int {
	if e == nil || maxIdle <= 0 {
		return 0
	}
	cutoff := time.Now().Add(-maxIdle).UnixNano()

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closing {
		// Drain owns the worker lifecycle once it starts. Don't fight it.
		return 0
	}
	reaped := 0
	e.workers.Range(func(k, v any) bool {
		h := v.(*workerHandle)
		if h.lastUsed.Load() > cutoff {
			return true
		}
		if len(h.ch) != 0 {
			// Idle by timestamp but queue has pending work — skip.
			// The worker will process it and bump lastUsed.
			return true
		}
		close(h.ch)
		e.workers.Delete(k)
		reaped++
		return true
	})
	if reaped > 0 {
		e.Log.Info("supernotify reaped idle workers",
			zap.Int("count", reaped),
			zap.Duration("max_idle", maxIdle))
	}
	return reaped
}

// StartReaper runs ReapIdle on a ticker until ctx is canceled. Matches
// the shell reaper's shape — main.go is expected to call this once.
// period is the sweep interval; maxIdle is the idle threshold.
func (e *Emitter) StartReaper(ctx context.Context, period, maxIdle time.Duration) {
	if e == nil || period <= 0 {
		return
	}
	go func() {
		t := time.NewTicker(period)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				e.ReapIdle(maxIdle)
			}
		}
	}()
}

// sendOne delivers a single queued DM. A panic inside the send path
// is recovered, logged at Error with stack, and swallowed so the
// worker loop continues.
func (e *Emitter) sendOne(b *bot.Bot, recipient int64, req pendingSend) {
	defer func() {
		if r := recover(); r != nil {
			e.Log.Error("supernotify sendOne panic",
				zap.Int64("recipient", recipient),
				zap.Any("panic", r),
				zap.Stack("stack"))
		}
	}()
	msg, err := b.SendMessage(context.Background(), &bot.SendMessageParams{
		ChatID:      recipient,
		Text:        req.text,
		ReplyMarkup: req.kb,
	})
	if err != nil {
		// Typical failure: recipient has never /start'd the bot so
		// their chat_id isn't addressable. No retry.
		e.Log.Warn("supernotify worker DM failed",
			zap.Int64("recipient", recipient), zap.Error(err))
		return
	}
	if e.Store != nil && msg != nil {
		e.Store.Track(recipient, msg.ID)
	}
}

// tryEnqueue attempts a non-blocking send to q. On a full queue it
// drops the oldest queued entry and retries once. Returns:
//   - queued=true if req landed in the queue (possibly after a drop).
//   - droppedOldest=true if we had to eject an older entry to make room.
//   - queued=false if a concurrent producer filled the slot between
//     the drop and retry (extremely rare; exposed so callers can log).
//
// Extracted as a pure function for easy testing.
func tryEnqueue(q chan pendingSend, req pendingSend) (queued, droppedOldest bool) {
	select {
	case q <- req:
		return true, false
	default:
	}
	select {
	case <-q:
		droppedOldest = true
	default:
	}
	select {
	case q <- req:
		return true, droppedOldest
	default:
		return false, droppedOldest
	}
}
