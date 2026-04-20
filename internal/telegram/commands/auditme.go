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

// HandleAuditMe: /auditme [N] — any user's last N events of their own.
func HandleAuditMe(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		n := common.ParseIntArg(update.Message.Text, "/auditme", 20)
		uid := u.TelegramID
		rows, err := d.Audit.Recent(ctx, &uid, n)
		if err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "audit failed: "+err.Error())
			return
		}
		common.Reply(ctx, b, update.Message.Chat.ID, views.FormatAudit(rows))
	}
}
