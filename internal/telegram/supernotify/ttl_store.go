// Package supernotify DMs the super + the actor's direct promoter for
// user-lifecycle events (promote / demote / add / remove / ban) with
// inline quick-action buttons. Buttons auto-strip after a configurable
// window so stale DMs can't cause accidental mutations.
package supernotify

import (
	"context"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"
)

// sweepConcurrency caps the number of EditMessageReplyMarkup calls in
// flight at once during a single Sweep. Telegram's global bot rate is
// ~30 req/sec; 5 concurrent keeps us well under that while cutting
// wall-clock time on large bursts from "≥10s serial" to "~2s".
const sweepConcurrency = 5

type sentMsg struct {
	chatID  int64
	msgID   int
	expires time.Time
}

// TTLStore tracks super-notification DMs that carry quick-action
// keyboards so a scheduled sweep can strip the keyboard after the TTL
// elapses. In-memory only — restart drops tracked entries (stale
// buttons after restart are safe: callback handlers re-check rbac +
// target state on tap, so a stale tap fails with a toast rather than
// mutating).
type TTLStore struct {
	mu   sync.Mutex
	ttl  time.Duration
	msgs []sentMsg
}

// NewTTLStore constructs a TTLStore. ttl=0 disables auto-strip
// (Track still records; Sweep never acts).
func NewTTLStore(ttl time.Duration) *TTLStore {
	return &TTLStore{ttl: ttl}
}

// Enabled reports whether Sweep will ever clear keyboards.
func (s *TTLStore) Enabled() bool { return s != nil && s.ttl > 0 }

// Track records a super-notification DM for later sweep. No-op if
// TTL is disabled.
func (s *TTLStore) Track(chatID int64, msgID int) {
	if !s.Enabled() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, sentMsg{
		chatID:  chatID,
		msgID:   msgID,
		expires: time.Now().Add(s.ttl),
	})
}

// takeExpired removes and returns every tracked entry with expires
// <= now. Exposed on the package internally so Sweep can act on the
// result outside the store's lock and tests can verify partitioning
// without a real bot.
func (s *TTLStore) takeExpired(now time.Time) []sentMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.msgs) == 0 {
		return nil
	}
	expired := make([]sentMsg, 0)
	kept := s.msgs[:0]
	for _, m := range s.msgs {
		if now.After(m.expires) {
			expired = append(expired, m)
		} else {
			kept = append(kept, m)
		}
	}
	s.msgs = kept
	return expired
}

// Sweep strips the reply_markup from every tracked message whose TTL
// has elapsed. Safe to call on a timer; removed entries are
// garbage-collected. A failed EditMessageReplyMarkup is logged at Debug
// (a user who deleted the DM leaves a stale tracked entry; we just
// drop it on the floor).
//
// API calls run with bounded concurrency (sweepConcurrency) so a
// burst of 100+ expired messages completes in seconds instead of
// blocking the sweep goroutine for half a minute. Pileup-safe
// under overlapping tick calls: takeExpired atomically removes entries
// from the store before the API calls start, so a second Sweep firing
// mid-first-sweep sees no duplicates.
func (s *TTLStore) Sweep(ctx context.Context, b *bot.Bot, log *zap.Logger) {
	if !s.Enabled() {
		return
	}
	if log == nil {
		log = zap.NewNop()
	}
	expired := s.takeExpired(time.Now())
	if len(expired) == 0 {
		return
	}
	emptyKB := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}}
	sem := make(chan struct{}, sweepConcurrency)
	var wg sync.WaitGroup
	for _, m := range expired {
		wg.Add(1)
		sem <- struct{}{}
		go func(m sentMsg) {
			// Deferred LIFO: recover runs, then sem release, then
			// wg.Done. wg.Done is guaranteed even on panic so
			// wg.Wait() can't hang.
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					log.Error("ttl sweep goroutine panic",
						zap.Int64("chat_id", m.chatID),
						zap.Int("msg_id", m.msgID),
						zap.Any("panic", r),
					)
				}
			}()
			_, err := b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
				ChatID:      m.chatID,
				MessageID:   m.msgID,
				ReplyMarkup: emptyKB,
			})
			if err != nil {
				log.Debug("ttl strip",
					zap.Int64("chat_id", m.chatID),
					zap.Int("msg_id", m.msgID),
					zap.Error(err))
			}
		}(m)
	}
	wg.Wait()
}

// Len returns the number of tracked entries (for tests / metrics).
func (s *TTLStore) Len() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.msgs)
}
