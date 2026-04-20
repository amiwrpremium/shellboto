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
	"github.com/amiwrpremium/shellboto/internal/telegram/rbac"
	"github.com/amiwrpremium/shellboto/internal/telegram/views"
)

// HandleDemote handles every dm:* callback. Cascade demotion on confirm.
func HandleDemote(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.CallbackQuery, *dbm.User) {
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
			editToDemoteList(ctx, d, b, chatID, msgID, u)
			common.AnswerCallback(ctx, b, cb.ID, "", false)
			return
		}
		if action == "n" {
			common.AnswerCallback(ctx, b, cb.ID, "canceled", false)
			common.EditOutcome(ctx, b, chatID, msgID, "✖ canceled", ns.Demote)
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
		if err != nil || tu.Role != dbm.RoleAdmin {
			common.AnswerCallback(ctx, b, cb.ID, "target no longer demotable", true)
			common.EditOutcome(ctx, b, chatID, msgID, "target no longer demotable", ns.Demote)
			return
		}
		if !rbac.CanDemote(u, tu) {
			common.AnswerCallback(ctx, b, cb.ID, "you didn't promote this admin", true)
			common.EditOutcome(ctx, b, chatID, msgID, "you can only demote admins you promoted", ns.Demote)
			return
		}

		switch action {
		case "s":
			prompt := fmt.Sprintf("demote %s (id: %d) to user?\nadmins they promoted will also be demoted.", views.DisplayLabel(tu), tu.TelegramID)
			kb := keyboards.YesNoSimple(ns.Demote, ns.Yes, ns.No, tu.TelegramID)
			_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
				ChatID: chatID, MessageID: msgID, Text: prompt, ReplyMarkup: kb,
			})
			common.AnswerCallback(ctx, b, cb.ID, "", false)

		case "y":
			// TOCTOU re-check: caller + target + permission, all
			// freshly re-read immediately before the cascade mutation.
			fresh := middleware.RefreshActiveCaller(d, u.TelegramID)
			if fresh == nil || !fresh.IsAdminOrAbove() {
				common.AnswerCallback(ctx, b, cb.ID, "not authorized anymore", true)
				common.EditOutcome(ctx, b, chatID, msgID, "not authorized anymore", ns.Demote)
				return
			}
			freshTU, err := d.Users.LookupActive(target)
			if err != nil || freshTU.Role != dbm.RoleAdmin {
				common.AnswerCallback(ctx, b, cb.ID, "target no longer demotable", true)
				common.EditOutcome(ctx, b, chatID, msgID, "target no longer demotable", ns.Demote)
				return
			}
			if !rbac.CanDemote(fresh, freshTU) {
				common.AnswerCallback(ctx, b, cb.ID, "you can no longer demote this admin", true)
				common.EditOutcome(ctx, b, chatID, msgID, "you can no longer demote this admin", ns.Demote)
				return
			}
			actor := fresh.TelegramID
			subtree, err := d.Users.CollectAdminSubtree(target)
			if err != nil {
				common.AnswerCallback(ctx, b, cb.ID, "collect failed", true)
				return
			}

			// Snapshot each node's PromotedBy BEFORE demotion wipes it, so
			// cascaded users can be told which of their promoters lost
			// admin (not the ultimate root actor).
			priorPromoter := make(map[int64]int64, len(subtree))
			for _, id := range subtree {
				if tg, err := d.Users.Lookup(id); err == nil && tg.PromotedBy != nil {
					priorPromoter[id] = *tg.PromotedBy
				}
			}

			if err := d.Users.Demote(subtree); err != nil {
				common.AnswerCallback(ctx, b, cb.ID, "demote failed", true)
				return
			}
			// Drop every demoted user's existing root shell so their next
			// message spawns an unprivileged one with the new credentials.
			for _, id := range subtree {
				d.Shells.Reset(id)
			}
			for i, id := range subtree {
				_, _ = d.Audit.Log(ctx, repo.Event{
					UserID: &actor, Kind: dbm.KindRoleChanged,
					Detail: map[string]any{
						"target":   id,
						"from":     dbm.RoleAdmin,
						"to":       dbm.RoleUser,
						"by":       actor,
						"cascaded": i != 0,
					},
				})
				if i == 0 {
					notifyDemoted(ctx, d, b, id, actor, 0)
				} else {
					notifyDemoted(ctx, d, b, id, actor, priorPromoter[id])
				}
			}
			msg := fmt.Sprintf("✅ demoted %s → user", views.DisplayLabel(freshTU))
			if len(subtree) > 1 {
				msg += fmt.Sprintf("\n(+ %d cascaded)", len(subtree)-1)
			}
			if d.Notify != nil {
				d.Notify.Demoted(ctx, b, fresh, freshTU, len(subtree)-1)
			}
			common.AnswerCallback(ctx, b, cb.ID, "demoted", false)
			common.EditOutcome(ctx, b, chatID, msgID, msg, ns.Demote)

		default:
			common.AnswerCallback(ctx, b, cb.ID, "unknown action", false)
		}
	}
}

func editToDemoteList(ctx context.Context, d *deps.Deps, b *bot.Bot, chatID int64, msgID int, caller *dbm.User) {
	var filter *int64
	if !caller.IsSuperadmin() {
		id := caller.TelegramID
		filter = &id
	}
	admins, err := d.Users.ListByRole(dbm.RoleAdmin, filter)
	if err != nil || len(admins) == 0 {
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID: chatID, MessageID: msgID, Text: "no admins you can demote.",
			ReplyMarkup: keyboards.Empty(),
		})
		return
	}
	text, kb := views.BuildCandidateList("select an admin to demote to user:", admins, ns.Demote)
	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID: chatID, MessageID: msgID, Text: text, ReplyMarkup: kb,
	})
}
