// Package middleware exposes the two wrappers that gate every handler:
// WrapText for message-text handlers and WrapCallback for callback queries.
// Both look up the caller's user row, short-circuit on an inactive /
// missing record, and Touch the row with the latest Telegram metadata.
package middleware

import (
	"context"
	"strings"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"
	"go.uber.org/zap"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/db/repo"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
)

// Rate-limit exemptions:
//   - "/cancel" and "/kill" text commands let a user interrupt a
//     runaway command even when otherwise rate-limited.
//   - Callback data "j:c" / "j:k" are the inline-button equivalents
//     of the above.
// Exempting these is a safety property, not convenience.

// isRateLimitExemptText decides whether `text` is a /cancel or /kill
// invocation that should bypass the token bucket.
//
// Uses exact-command matching on the first whitespace-separated token,
// stripping any `@botname` suffix (Telegram's group-chat disambiguator).
// Prefix matching (`strings.HasPrefix(t, "/cancel")`) would let
// `/cancelfoo`, `/canceling`, etc. sneak past the limiter.
func isRateLimitExemptText(text string) bool {
	first := strings.SplitN(strings.TrimSpace(text), " ", 2)[0]
	if i := strings.IndexByte(first, '@'); i > 0 {
		first = first[:i] // strip @botname suffix
	}
	return first == "/cancel" || first == "/kill"
}

func isRateLimitExemptCallback(data string) bool {
	return data == "j:c" || data == "j:k"
}

// TextHandler is a message-text handler that runs after successful auth.
type TextHandler func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User)

// CallbackHandler is a callback-query handler that runs after successful auth.
type CallbackHandler func(ctx context.Context, b *bot.Bot, cb *tgm.CallbackQuery, u *dbm.User)

// TrimUpdate normalizes user-editable string fields on an incoming
// update so handlers don't have to sprinkle strings.TrimSpace calls
// everywhere. Applied at the top of every middleware wrapper (and the
// DefaultHandler) so every handler sees already-trimmed values.
// Mutates the update in place.
func TrimUpdate(u *tgm.Update) {
	if u == nil {
		return
	}
	if u.Message != nil {
		u.Message.Text = strings.TrimSpace(u.Message.Text)
		u.Message.Caption = strings.TrimSpace(u.Message.Caption)
	}
	if u.CallbackQuery != nil {
		u.CallbackQuery.Data = strings.TrimSpace(u.CallbackQuery.Data)
	}
}

// recoverHandler is deferred at the top of WrapText / WrapCallback so
// a panic inside any command or callback handler can't crash the whole
// bot process. The panic is logged with a stack trace at Error;
// we deliberately do NOT attempt a Reply or AnswerCallback — doing so
// could panic again, and the caller-side state is already unknown.
// systemd would restart the bot on a real crash, so failing silently
// here is strictly better than taking down the entire service.
func recoverHandler(log *zap.Logger, kind string) {
	if r := recover(); r != nil {
		if log == nil {
			log = zap.NewNop()
		}
		log.Error("handler panic",
			zap.String("kind", kind),
			zap.Any("panic", r),
			zap.Stack("stack"),
		)
	}
}

// WrapText returns a bot.HandlerFunc that authenticates the caller, touches
// their row, and dispatches to h. If the caller is not an active user, the
// update is silently dropped (with an optional auth_reject audit event).
// Rate-limited callers get a short "rate limited" reply unless the
// command is in the exemption list (/cancel, /kill).
func WrapText(d *deps.Deps, h TextHandler) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update) {
		defer recoverHandler(d.L(), "text")
		TrimUpdate(update)
		if update.Message == nil || update.Message.From == nil {
			return
		}
		usr, err := d.Users.LookupActive(update.Message.From.ID)
		if err != nil {
			LogRejectText(ctx, d, update)
			return
		}
		d.Users.Touch(usr.TelegramID, update.Message.From.Username, update.Message.From.FirstName)

		if d.RateLimit.Enabled() && !isRateLimitExemptText(update.Message.Text) {
			if !d.RateLimit.Allow(usr.TelegramID) {
				common.Reply(ctx, b, update.Message.Chat.ID, "⚠ rate limited — slow down.")
				return
			}
		}

		h(ctx, b, update, usr)
	}
}

// WrapCallback returns a bot.HandlerFunc that authenticates the callback's
// From user and dispatches to h. Unauthenticated taps get a toast alert
// AND an auth_reject audit row (parity with WrapText). Rate-limited
// callers get a "rate limited" toast unless the callback is a signal
// (j:c / j:k) that must always go through.
func WrapCallback(d *deps.Deps, h CallbackHandler) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update) {
		defer recoverHandler(d.L(), "callback")
		TrimUpdate(update)
		cb := update.CallbackQuery
		if cb == nil || cb.From.ID == 0 {
			return
		}
		usr, err := d.Users.LookupActive(cb.From.ID)
		if err != nil {
			LogRejectCallback(ctx, d, cb)
			common.AnswerCallback(ctx, b, cb.ID, "not authorized", true)
			return
		}
		d.Users.Touch(cb.From.ID, cb.From.Username, cb.From.FirstName)

		if d.RateLimit.Enabled() && !isRateLimitExemptCallback(cb.Data) {
			if !d.RateLimit.Allow(usr.TelegramID) {
				common.AnswerCallback(ctx, b, cb.ID, "rate limited — slow down", true)
				return
			}
		}

		h(ctx, b, cb, usr)
	}
}

// RefreshActiveCaller re-fetches the caller's user row immediately
// before a privileged DB mutation, closing the TOCTOU window between
// WrapCallback's initial lookup and the handler's write. Returns the
// fresh row, or nil if the caller was banned/removed/demoted in the
// intervening milliseconds. Callback handlers surface the rejection
// with AnswerCallback + (optionally) an EditCallbackMessage when they
// receive nil.
func RefreshActiveCaller(d *deps.Deps, telegramID int64) *dbm.User {
	u, err := d.Users.LookupActive(telegramID)
	if err != nil {
		return nil
	}
	return u
}

// auditReject emits a Warn log + auth_reject audit row for either a
// text message or a callback tap. `source` is "text" or "callback" so
// operators can filter by how the rejected attempt arrived; `extra` is
// merged into the audit detail JSON (callback_data for callbacks, nil
// for text).
//
// A pre-auth rate limiter (d.AuthRejectLimit, keyed by From-id)
// gates both the log and the DB write. Without it, any non-whitelisted
// Telegram account can write unbounded audit rows by spamming — worst
// case ~1800 rows/min → DB fills in hours. The limiter is generous for
// a first attempt (burst) and then throttles aggressively per-ID, so
// legit new users see their first attempts logged while a
// determined attacker can't fill the DB.
func auditReject(ctx context.Context, d *deps.Deps, userID int64, username, firstName, source string, extra map[string]any) {
	if d.AuthRejectLimit != nil && d.AuthRejectLimit.Enabled() && !d.AuthRejectLimit.Allow(userID) {
		// Silent drop: over-limit attempts add nothing operators can
		// act on, and writing them defeats the rate limit's purpose.
		return
	}
	d.L().Warn("rejected "+source,
		zap.Int64("user_id", userID),
		zap.String("username", username),
		zap.String("first_name", firstName),
	)
	detail := map[string]any{
		"username":   username,
		"first_name": firstName,
		"source":     source,
	}
	for k, v := range extra {
		detail[k] = v
	}
	uid := userID
	_, _ = d.Audit.Log(ctx, repo.Event{
		UserID: &uid,
		Kind:   dbm.KindAuthReject,
		Detail: detail,
	})
}

// LogRejectText records a text-message auth reject (exported so the
// DefaultHandler, which runs outside WrapText, can share the same
// rate-limited audit path).
func LogRejectText(ctx context.Context, d *deps.Deps, update *tgm.Update) {
	if update == nil || update.Message == nil {
		return
	}
	from := update.Message.From
	if from == nil {
		return
	}
	auditReject(ctx, d, from.ID, from.Username, from.FirstName, "text", nil)
}

// LogRejectCallback records an auth reject for a callback-query tap
// from a non-whitelisted account. Includes the callback-data so
// operators can see which button was tapped; truncated to 100 chars
// to keep audit rows bounded. Callback tokens (16 hex, per-user
// scoped, 60s TTL) are not secrets under the trust model — the
// tapping attacker isn't in the user DB, and the tokens are
// single-use anyway.
func LogRejectCallback(ctx context.Context, d *deps.Deps, cb *tgm.CallbackQuery) {
	if cb == nil {
		return
	}
	data := cb.Data
	if len(data) > 100 {
		data = data[:100] + "…"
	}
	auditReject(ctx, d, cb.From.ID, cb.From.Username, cb.From.FirstName, "callback",
		map[string]any{"callback_data": data})
}
