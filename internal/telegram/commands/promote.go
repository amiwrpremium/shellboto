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

// HandlePromote posts the candidate list for /promote. Admin+ may promote
// any active role=user; the confirm + apply steps are the pr: callbacks.
func HandlePromote(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		if !RequireAdmin(ctx, b, update, u) {
			return
		}
		users, err := d.Users.ListByRole(dbm.RoleUser, nil)
		if err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "list failed: "+err.Error())
			return
		}
		if len(users) == 0 {
			common.Reply(ctx, b, update.Message.Chat.ID, "no users to promote.")
			return
		}
		text, kb := views.BuildCandidateList("select a user to promote to admin:", users, ns.Promote)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID, Text: text, ReplyMarkup: kb,
		})
	}
}
