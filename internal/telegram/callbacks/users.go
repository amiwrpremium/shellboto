package callbacks

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/db/repo"
	"github.com/amiwrpremium/shellboto/internal/telegram/commands"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
	"github.com/amiwrpremium/shellboto/internal/telegram/keyboards"
	"github.com/amiwrpremium/shellboto/internal/telegram/middleware"
	ns "github.com/amiwrpremium/shellboto/internal/telegram/namespaces"
	"github.com/amiwrpremium/shellboto/internal/telegram/rbac"
	"github.com/amiwrpremium/shellboto/internal/telegram/views"
)

// HandleUsers is the big /users browser dispatcher.
func HandleUsers(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.CallbackQuery, *dbm.User) {
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

		needID := func() (int64, bool) {
			if len(parts) != 3 {
				common.AnswerCallback(ctx, b, cb.ID, "bad callback", false)
				return 0, false
			}
			tid, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				common.AnswerCallback(ctx, b, cb.ID, "bad id", false)
				return 0, false
			}
			return tid, true
		}

		switch action {
		case "l":
			renderList(ctx, d, b, cb.ID, chatID, msgID)
		case "p":
			if tid, ok := needID(); ok {
				renderProfile(ctx, d, b, cb.ID, chatID, msgID, u, tid)
			}
		case "a":
			if tid, ok := needID(); ok {
				renderAuditView(ctx, d, b, cb.ID, chatID, msgID, tid)
			}
		case "c":
			if tid, ok := needID(); ok {
				renderCommandsView(ctx, d, b, cb.ID, chatID, msgID, tid)
			}
		case "o":
			if tid, ok := needID(); ok {
				sendLastOutput(ctx, d, b, cb.ID, chatID, tid)
			}
		case "r":
			if tid, ok := needID(); ok {
				renderRemoveConfirm(ctx, d, b, cb.ID, chatID, msgID, u, tid)
			}
		case "rY":
			if tid, ok := needID(); ok {
				executeRemove(ctx, d, b, cb.ID, chatID, msgID, u, tid)
			}
		case "i":
			if tid, ok := needID(); ok {
				renderReinstateConfirm(ctx, d, b, cb.ID, chatID, msgID, u, tid)
			}
		case "iY":
			if tid, ok := needID(); ok {
				executeReinstate(ctx, d, b, cb.ID, chatID, msgID, u, tid)
			}
		default:
			common.AnswerCallback(ctx, b, cb.ID, "unknown action", false)
		}
	}
}

func renderList(ctx context.Context, d *deps.Deps, b *bot.Bot, cbID string, chatID int64, msgID int) {
	users, err := d.Users.ListAll()
	if err != nil {
		common.AnswerCallback(ctx, b, cbID, "list failed", true)
		return
	}
	text, kb := views.BuildUsersList(users)
	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID: chatID, MessageID: msgID, Text: text, ReplyMarkup: kb,
	})
	common.AnswerCallback(ctx, b, cbID, "", false)
}

func renderProfile(ctx context.Context, d *deps.Deps, b *bot.Bot, cbID string, chatID int64, msgID int, caller *dbm.User, tid int64) {
	target, err := d.Users.Lookup(tid)
	if err != nil {
		common.AnswerCallback(ctx, b, cbID, "no such user", true)
		return
	}
	text := views.BuildProfileText(target, views.WhoIsFunc(d.Users), views.LastActivityOf(ctx, d.Audit, target.TelegramID))
	kb := views.BuildProfileKeyboard(caller, target)
	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID: chatID, MessageID: msgID, Text: text, ReplyMarkup: kb,
	})
	common.AnswerCallback(ctx, b, cbID, "", false)
}

func renderAuditView(ctx context.Context, d *deps.Deps, b *bot.Bot, cbID string, chatID int64, msgID int, tid int64) {
	rows, err := d.Audit.Recent(ctx, &tid, 20)
	if err != nil {
		common.AnswerCallback(ctx, b, cbID, "audit failed", true)
		return
	}
	text := "audit for user " + strconv.FormatInt(tid, 10) + " (last 20):\n" + views.FormatAudit(rows)
	kb := &tgm.InlineKeyboardMarkup{InlineKeyboard: [][]tgm.InlineKeyboardButton{{
		{Text: "◀ Back", CallbackData: ns.CBData(ns.Users, ns.Profile, tid)},
	}}}
	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID: chatID, MessageID: msgID, Text: common.ClipText(text, 3900), ReplyMarkup: kb,
	})
	common.AnswerCallback(ctx, b, cbID, "", false)
}

func renderCommandsView(ctx context.Context, d *deps.Deps, b *bot.Bot, cbID string, chatID int64, msgID int, tid int64) {
	rows, err := d.Audit.Recent(ctx, &tid, 200)
	if err != nil {
		common.AnswerCallback(ctx, b, cbID, "failed", true)
		return
	}
	filtered := make([]*repo.Row, 0, 20)
	for _, row := range rows {
		if row.Kind == dbm.KindCommandRun {
			filtered = append(filtered, row)
			if len(filtered) >= 20 {
				break
			}
		}
	}
	text := "commands for user " + strconv.FormatInt(tid, 10) + " (last 20):\n" + views.FormatAudit(filtered)
	kb := &tgm.InlineKeyboardMarkup{InlineKeyboard: [][]tgm.InlineKeyboardButton{{
		{Text: "◀ Back", CallbackData: ns.CBData(ns.Users, ns.Profile, tid)},
	}}}
	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID: chatID, MessageID: msgID, Text: common.ClipText(text, 3900), ReplyMarkup: kb,
	})
	common.AnswerCallback(ctx, b, cbID, "", false)
}

func sendLastOutput(ctx context.Context, d *deps.Deps, b *bot.Bot, cbID string, chatID int64, tid int64) {
	id, err := d.Audit.LatestCommandRun(ctx, tid)
	if err != nil {
		common.AnswerCallback(ctx, b, cbID, "no command output on record", true)
		return
	}
	gz, orig, err := d.Audit.FetchOutput(ctx, id)
	if err != nil {
		common.AnswerCallback(ctx, b, cbID, "fetch failed", true)
		return
	}
	filename := fmt.Sprintf("user-%d-audit-%d.txt.gz", tid, id)
	caption := fmt.Sprintf("user %d · audit #%d · %d bytes gz · %d bytes original", tid, id, len(gz), orig)
	err = commands.WithUploadIndicator(ctx, d, chatID, func() error {
		_, e := b.SendDocument(ctx, &bot.SendDocumentParams{
			ChatID: chatID,
			Document: &tgm.InputFileUpload{
				Filename: filename,
				Data:     bytes.NewReader(gz),
			},
			Caption: caption,
		})
		return e
	})
	if err != nil {
		common.AnswerCallback(ctx, b, cbID, "send failed", true)
		return
	}
	common.AnswerCallback(ctx, b, cbID, "sent", false)
}

func renderRemoveConfirm(ctx context.Context, d *deps.Deps, b *bot.Bot, cbID string, chatID int64, msgID int, caller *dbm.User, tid int64) {
	target, err := d.Users.Lookup(tid)
	if err != nil || target.IsSuperadmin() || target.TelegramID == caller.TelegramID || !rbac.CanActOnLifecycle(caller, target) {
		common.AnswerCallback(ctx, b, cbID, "not allowed", true)
		return
	}
	text := fmt.Sprintf("remove %s (id: %d)?", views.DisplayLabel(target), target.TelegramID)
	kb := keyboards.YesNoID(ns.Users, ns.RemoveYes, ns.Profile, tid)
	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID: chatID, MessageID: msgID, Text: text, ReplyMarkup: kb,
	})
	common.AnswerCallback(ctx, b, cbID, "", false)
}

func executeRemove(ctx context.Context, d *deps.Deps, b *bot.Bot, cbID string, chatID int64, msgID int, caller *dbm.User, tid int64) {
	// TOCTOU re-check: re-fetch both caller and target.
	fresh := middleware.RefreshActiveCaller(d, caller.TelegramID)
	if fresh == nil {
		common.AnswerCallback(ctx, b, cbID, "not authorized anymore", true)
		return
	}
	target, err := d.Users.Lookup(tid)
	if err != nil || target.IsSuperadmin() || target.TelegramID == fresh.TelegramID || !rbac.CanActOnLifecycle(fresh, target) {
		common.AnswerCallback(ctx, b, cbID, "not allowed", true)
		return
	}
	if err := d.Users.SoftDelete(tid); err != nil {
		common.AnswerCallback(ctx, b, cbID, "remove failed", true)
		return
	}
	d.Shells.Reset(tid)
	uid := fresh.TelegramID
	_, _ = d.Audit.Log(ctx, repo.Event{
		UserID: &uid, Kind: dbm.KindUserRemoved,
		Detail: map[string]any{"target": tid, "was_role": target.Role, "via": "users_menu"},
	})
	if d.Notify != nil {
		d.Notify.Removed(ctx, b, fresh, target)
	}
	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID: chatID, MessageID: msgID,
		Text:        fmt.Sprintf("✅ removed %s", views.DisplayLabel(target)),
		ReplyMarkup: keyboards.BackToList(ns.Users),
	})
	common.AnswerCallback(ctx, b, cbID, "removed", false)
}

func renderReinstateConfirm(ctx context.Context, d *deps.Deps, b *bot.Bot, cbID string, chatID int64, msgID int, caller *dbm.User, tid int64) {
	target, err := d.Users.Lookup(tid)
	if err != nil || target.DisabledAt == nil || target.IsSuperadmin() || !rbac.CanActOnLifecycle(caller, target) {
		common.AnswerCallback(ctx, b, cbID, "not allowed", true)
		return
	}
	text := fmt.Sprintf("reinstate %s (id: %d)?", views.DisplayLabel(target), target.TelegramID)
	kb := keyboards.YesNoID(ns.Users, ns.ReinstateYes, ns.Profile, tid)
	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID: chatID, MessageID: msgID, Text: text, ReplyMarkup: kb,
	})
	common.AnswerCallback(ctx, b, cbID, "", false)
}

func executeReinstate(ctx context.Context, d *deps.Deps, b *bot.Bot, cbID string, chatID int64, msgID int, caller *dbm.User, tid int64) {
	// TOCTOU re-check.
	fresh := middleware.RefreshActiveCaller(d, caller.TelegramID)
	if fresh == nil {
		common.AnswerCallback(ctx, b, cbID, "not authorized anymore", true)
		return
	}
	target, err := d.Users.Lookup(tid)
	if err != nil || target.IsSuperadmin() || !rbac.CanActOnLifecycle(fresh, target) {
		common.AnswerCallback(ctx, b, cbID, "not allowed", true)
		return
	}
	if err := d.Users.Reinstate(tid); err != nil {
		common.AnswerCallback(ctx, b, cbID, "reinstate failed", true)
		return
	}
	uid := fresh.TelegramID
	_, _ = d.Audit.Log(ctx, repo.Event{
		UserID: &uid, Kind: dbm.KindUserAdded,
		Detail: map[string]any{"target": tid, "reinstated": true},
	})
	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID: chatID, MessageID: msgID,
		Text:        fmt.Sprintf("✅ reinstated %s", views.DisplayLabel(target)),
		ReplyMarkup: keyboards.BackToList(ns.Users),
	})
	common.AnswerCallback(ctx, b, cbID, "reinstated", false)
}
