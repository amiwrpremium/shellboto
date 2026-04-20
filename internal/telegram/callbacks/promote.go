package callbacks

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/db/repo"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
	"github.com/amiwrpremium/shellboto/internal/telegram/keyboards"
	"github.com/amiwrpremium/shellboto/internal/telegram/middleware"
	ns "github.com/amiwrpremium/shellboto/internal/telegram/namespaces"
	"github.com/amiwrpremium/shellboto/internal/telegram/views"
)

// HandlePromote handles every pr:* callback (list, select, confirm, cancel).
func HandlePromote(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.CallbackQuery, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, cb *tgm.CallbackQuery, u *dbm.User) {
		if !u.IsAdminOrAbove() {
			common.AnswerCallback(ctx, b, cb.ID, "not authorized", true)
			return
		}
		parts := strings.SplitN(cb.Data, ":", 3)
		if len(parts) < 2 {
			common.AnswerCallback(ctx, b, cb.ID, "bad callback", false)
			return
		}
		action := parts[1]
		chatID, msgID := common.CallbackMessageRef(cb)

		if action == "l" {
			editToPromoteList(ctx, d, b, chatID, msgID)
			common.AnswerCallback(ctx, b, cb.ID, "", false)
			return
		}
		if action == "n" {
			common.AnswerCallback(ctx, b, cb.ID, "canceled", false)
			common.EditOutcome(ctx, b, chatID, msgID, "✖ canceled", ns.Promote)
			return
		}
		if len(parts) != 3 {
			common.AnswerCallback(ctx, b, cb.ID, "bad callback", false)
			return
		}
		target, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			common.AnswerCallback(ctx, b, cb.ID, "bad id", false)
			return
		}
		tu, err := d.Users.LookupActive(target)
		if err != nil {
			common.AnswerCallback(ctx, b, cb.ID, "user no longer available", true)
			common.EditOutcome(ctx, b, chatID, msgID, "user no longer available", ns.Promote)
			return
		}
		if tu.Role != dbm.RoleUser {
			common.AnswerCallback(ctx, b, cb.ID, "target is not a user anymore", true)
			common.EditOutcome(ctx, b, chatID, msgID, "target is no longer a plain user", ns.Promote)
			return
		}

		switch action {
		case "s":
			prompt := fmt.Sprintf("promote %s (id: %d) to admin?", views.DisplayLabel(tu), tu.TelegramID)
			kb := keyboards.YesNoSimple(ns.Promote, ns.Yes, ns.No, tu.TelegramID)
			_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
				ChatID: chatID, MessageID: msgID, Text: prompt, ReplyMarkup: kb,
			})
			common.AnswerCallback(ctx, b, cb.ID, "", false)
		case "y":
			// TOCTOU re-check: ensure the caller is still an active
			// admin+ right before the mutation.
			fresh := middleware.RefreshActiveCaller(d, u.TelegramID)
			if fresh == nil || !fresh.IsAdminOrAbove() {
				common.AnswerCallback(ctx, b, cb.ID, "not authorized anymore", true)
				common.EditOutcome(ctx, b, chatID, msgID, "not authorized anymore", ns.Promote)
				return
			}
			actor := fresh.TelegramID
			if err := d.Users.Promote(target, actor); err != nil {
				common.AnswerCallback(ctx, b, cb.ID, "promote failed: "+err.Error(), true)
				return
			}
			_, _ = d.Audit.Log(ctx, repo.Event{
				UserID: &actor, Kind: dbm.KindRoleChanged,
				Detail: map[string]any{"target": target, "from": dbm.RoleUser, "to": dbm.RoleAdmin, "by": actor},
			})
			// Kill their existing shell so the next message spawns a new one
			// with the new role's credentials (user-unprivileged → root).
			d.Shells.Reset(target)
			notifyPromoted(ctx, d, b, tu, u)
			if d.Notify != nil {
				d.Notify.Promoted(ctx, b, fresh, tu)
			}
			common.AnswerCallback(ctx, b, cb.ID, "promoted", false)
			common.EditOutcome(ctx, b, chatID, msgID, fmt.Sprintf("✅ promoted %s → admin", views.DisplayLabel(tu)), ns.Promote)
		default:
			common.AnswerCallback(ctx, b, cb.ID, "unknown action", false)
		}
	}
}

// editToPromoteList re-renders the promote list into the existing message.
func editToPromoteList(ctx context.Context, d *deps.Deps, b *bot.Bot, chatID int64, msgID int) {
	users, err := d.Users.ListByRole(dbm.RoleUser, nil)
	if err != nil || len(users) == 0 {
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID: chatID, MessageID: msgID, Text: "no users to promote.",
			ReplyMarkup: keyboards.Empty(),
		})
		return
	}
	text, kb := views.BuildCandidateList("select a user to promote to admin:", users, ns.Promote)
	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID: chatID, MessageID: msgID, Text: text, ReplyMarkup: kb,
	})
}
