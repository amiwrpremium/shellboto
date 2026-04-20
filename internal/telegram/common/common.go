// Package common holds small stateless helpers used by most handlers:
// reply shortcuts, callback-message introspection, and string utilities.
package common

import (
	"context"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/amiwrpremium/shellboto/internal/telegram/keyboards"
	ns "github.com/amiwrpremium/shellboto/internal/telegram/namespaces"
)

// Reply sends a plain text message.
func Reply(ctx context.Context, b *bot.Bot, chatID int64, text string) {
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
}

// CallbackMessageRef extracts chat+msg id from a callback's source message,
// handling go-telegram/bot's MaybeInaccessibleMessage wrapper.
func CallbackMessageRef(cb *models.CallbackQuery) (int64, int) {
	if cb.Message.Message != nil {
		return cb.Message.Message.Chat.ID, cb.Message.Message.ID
	}
	if cb.Message.InaccessibleMessage != nil {
		return cb.Message.InaccessibleMessage.Chat.ID, cb.Message.InaccessibleMessage.MessageID
	}
	return 0, 0
}

// EditCallbackMessage edits the callback source message to plain text with
// the keyboard stripped.
func EditCallbackMessage(ctx context.Context, b *bot.Bot, chatID int64, msgID int, text string) {
	if chatID == 0 || msgID == 0 {
		return
	}
	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        text,
		ReplyMarkup: keyboards.Empty(),
	})
}

// EditOutcome edits the callback source message to a terminal text, with a
// [◀ Back] button pointing at `<prefix>:l`. Used for promote/demote
// success/cancel/error outcomes.
func EditOutcome(ctx context.Context, b *bot.Bot, chatID int64, msgID int, text string, prefix ns.NS) {
	if chatID == 0 || msgID == 0 {
		return
	}
	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        text,
		ReplyMarkup: keyboards.BackToList(prefix),
	})
}

// AnswerCallback sends AnswerCallbackQuery with optional toast text.
func AnswerCallback(ctx context.Context, b *bot.Bot, cbID, text string, alert bool) {
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: cbID,
		Text:            text,
		ShowAlert:       alert,
	})
}

// Truncate chops s to n characters with an ellipsis if needed.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ParseIntArg reads the first whitespace-separated arg after `cmd`, returns
// `def` if missing or invalid. Clamps at 200 as a safety cap.
func ParseIntArg(text, cmd string, def int) int {
	p := strings.Fields(strings.TrimPrefix(text, cmd))
	if len(p) == 0 {
		return def
	}
	n, err := strconv.Atoi(p[0])
	if err != nil || n <= 0 {
		return def
	}
	if n > 200 {
		n = 200
	}
	return n
}

// ClipText trims a string so the HTML-escaped form fits under Telegram's
// 4096-char cap — used for long audit/commands views.
func ClipText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-20] + "\n\n… (truncated)"
}
