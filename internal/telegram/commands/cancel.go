package commands

import (
	"context"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
)

// HandleCancel: /cancel aborts an /adduser wizard if one is active, else
// sends SIGINT to the foreground process of the caller's shell.
func HandleCancel(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		if flow := d.AddUser.Current(u.TelegramID); flow != nil {
			d.AddUser.Cancel(u.TelegramID)
			common.Reply(ctx, b, update.Message.Chat.ID, "adduser canceled.")
			return
		}
		s, err := d.Shells.Get(u.TelegramID, ShellOptsFor(d, u))
		if err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "no shell")
			return
		}
		if j := s.Current(); j != nil {
			j.SetTermination("canceled")
		}
		if err := s.SigInt(); err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "cancel failed: "+err.Error())
			return
		}
		common.Reply(ctx, b, update.Message.Chat.ID, "↯ SIGINT sent")
	}
}
