package callbacks

import (
	"context"
	"errors"
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
	"github.com/amiwrpremium/shellboto/internal/telegram/middleware"
)

// HandleAddUser handles `au:y:<tok>` (yes) and `au:n:<tok>` (no) on the
// /adduser summary card.
func HandleAddUser(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.CallbackQuery, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, cb *tgm.CallbackQuery, u *dbm.User) {
		parts := strings.SplitN(cb.Data, ":", 3)
		if len(parts) != 3 {
			common.AnswerCallback(ctx, b, cb.ID, "bad callback", false)
			return
		}
		action, token := parts[1], parts[2]
		chatID, msgID := common.CallbackMessageRef(cb)
		uid := u.TelegramID

		flow := d.AddUser.ClaimByToken(uid, token)
		if flow == nil {
			common.AnswerCallback(ctx, b, cb.ID, "flow expired", true)
			common.EditCallbackMessage(ctx, b, chatID, msgID, "⏱ flow expired")
			return
		}

		if action == "n" {
			d.AddUser.Cancel(uid)
			common.AnswerCallback(ctx, b, cb.ID, "canceled", false)
			common.EditCallbackMessage(ctx, b, chatID, msgID, "✖ canceled")
			return
		}
		if action != "y" {
			common.AnswerCallback(ctx, b, cb.ID, "unknown action", false)
			return
		}

		target := flow.TargetID
		name := flow.Name
		d.AddUser.Cancel(uid)

		// TOCTOU re-check: ensure the caller is still admin+ right
		// before inserting. Otherwise a just-demoted admin could still
		// succeed via a stale WrapCallback lookup.
		fresh := middleware.RefreshActiveCaller(d, uid)
		if fresh == nil || !fresh.IsAdminOrAbove() {
			common.AnswerCallback(ctx, b, cb.ID, "not authorized anymore", true)
			common.EditCallbackMessage(ctx, b, chatID, msgID, "not authorized anymore")
			return
		}

		if err := d.Users.Add(target, dbm.RoleUser, name, uid); err != nil {
			// Attempting to add the superadmin via /adduser would
			// UPSERT-demote them to role=user. The caller is trying to
			// demote the super; ban them. Super-calling-on-self is a
			// typo and handled gracefully (SoftDelete refuses super
			// anyway, so "banning" would be a misleading no-op).
			if errors.Is(err, repo.ErrTargetIsSuperadmin) {
				if fresh.IsSuperadmin() {
					common.AnswerCallback(ctx, b, cb.ID, "superadmin is env-seeded", true)
					common.EditCallbackMessage(ctx, b, chatID, msgID,
						"⚠ cannot add superadmin — managed via SHELLBOTO_SUPERADMIN_ID")
					return
				}
				commands.BanUser(ctx, d, b, chatID, fresh,
					"/adduser "+strconv.FormatInt(target, 10),
					"attempted to add the superadmin")
				common.AnswerCallback(ctx, b, cb.ID, "banned", true)
				common.EditCallbackMessage(ctx, b, chatID, msgID,
					"🚫 banned: attempted to add the superadmin")
				return
			}
			common.AnswerCallback(ctx, b, cb.ID, "add failed", true)
			common.EditCallbackMessage(ctx, b, chatID, msgID, "❌ add failed: "+err.Error())
			return
		}
		_, _ = d.Audit.Log(ctx, repo.Event{
			UserID: &uid, Kind: dbm.KindUserAdded,
			Detail: map[string]any{"target": target, "role": dbm.RoleUser, "name": name},
		})
		if d.Notify != nil {
			d.Notify.Added(ctx, b, fresh, target, name)
		}
		common.AnswerCallback(ctx, b, cb.ID, "added", false)
		common.EditCallbackMessage(ctx, b, chatID, msgID, fmt.Sprintf("✅ added %d as user\nname: %s", target, name))
	}
}
