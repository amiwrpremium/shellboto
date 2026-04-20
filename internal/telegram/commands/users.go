package commands

import (
	"context"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
	"github.com/amiwrpremium/shellboto/internal/telegram/views"
)

// HandleUsers posts the /users browser entry screen. All subsequent
// navigation is via the us: callback prefix.
func HandleUsers(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		if !RequireAdmin(ctx, b, update, u) {
			return
		}
		users, err := d.Users.ListAll()
		if err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "list failed: "+err.Error())
			return
		}
		text, kb := views.BuildUsersList(users)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID, Text: text, ReplyMarkup: kb,
		})
	}
}
