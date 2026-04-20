package commands

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
)

// HandleDelUser: /deluser <id> soft-deletes a user row with role-aware
// rules. Admin can target user; super can target admin; super row is
// always untouchable.
func HandleDelUser(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		if !RequireAdmin(ctx, b, update, u) {
			return
		}
		parts := strings.Fields(TrimCmdArg(update.Message.Text, "/deluser"))
		if len(parts) != 1 {
			common.Reply(ctx, b, update.Message.Chat.ID, "usage: /deluser <telegram_id>")
			return
		}
		target, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "bad telegram_id")
			return
		}
		if target == u.TelegramID {
			common.Reply(ctx, b, update.Message.Chat.ID, "cannot remove yourself")
			return
		}
		tu, err := d.Users.Lookup(target)
		if err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "no such user")
			return
		}
		if tu.IsSuperadmin() {
			common.Reply(ctx, b, update.Message.Chat.ID, "superadmin is managed via SHELLBOTO_SUPERADMIN_ID env — edit that and restart")
			return
		}
		if tu.Role == dbm.RoleAdmin && !u.IsSuperadmin() {
			common.Reply(ctx, b, update.Message.Chat.ID, "only superadmin can remove an admin")
			return
		}
		if err := d.Users.SoftDelete(target); err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "delete failed: "+err.Error())
			return
		}
		uid := u.TelegramID
		_, _ = d.Audit.Log(ctx, repo.Event{
			UserID: &uid, Kind: dbm.KindUserRemoved,
			Detail: map[string]any{"target": target, "was_role": tu.Role},
		})
		if d.Notify != nil {
			d.Notify.Removed(ctx, b, u, tu)
		}
		common.Reply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("removed %d", target))
	}
}
