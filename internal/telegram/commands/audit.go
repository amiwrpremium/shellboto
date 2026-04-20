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

// HandleAudit: /audit [N] — admin+ sees last N rows across all users.
func HandleAudit(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		if !RequireAdmin(ctx, b, update, u) {
			return
		}
		n := common.ParseIntArg(update.Message.Text, "/audit", 20)
		rows, err := d.Audit.Recent(ctx, nil, n)
		if err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "audit failed: "+err.Error())
			return
		}
		common.Reply(ctx, b, update.Message.Chat.ID, views.FormatAudit(rows))
	}
}
