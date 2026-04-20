package commands

import (
	"context"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
	ns "github.com/amiwrpremium/shellboto/internal/telegram/namespaces"
	"github.com/amiwrpremium/shellboto/internal/telegram/views"
)

// HandleDemote posts the candidate list for /demote. Super sees every
// active admin; plain admin sees only admins they promoted themselves.
func HandleDemote(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		if !RequireAdmin(ctx, b, update, u) {
			return
		}
		var filter *int64
		if !u.IsSuperadmin() {
			id := u.TelegramID
			filter = &id
		}
		admins, err := d.Users.ListByRole(dbm.RoleAdmin, filter)
		if err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "list failed: "+err.Error())
			return
		}
		if len(admins) == 0 {
			common.Reply(ctx, b, update.Message.Chat.ID, "no admins you can demote.")
			return
		}
		text, kb := views.BuildCandidateList("select an admin to demote to user:", admins, ns.Demote)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID, Text: text, ReplyMarkup: kb,
		})
	}
}
