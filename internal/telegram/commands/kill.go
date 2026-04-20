package commands

import (
	"context"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
)

// HandleKill: /kill sends SIGKILL to the shell's foreground pgid.
func HandleKill(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		s, err := d.Shells.Get(u.TelegramID, ShellOptsFor(d, u))
		if err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "no shell")
			return
		}
		if j := s.Current(); j != nil {
			j.SetTermination("killed")
		}
		if err := s.SigKill(); err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "kill failed: "+err.Error())
			return
		}
		common.Reply(ctx, b, update.Message.Chat.ID, "☠ SIGKILL sent to foreground pgid")
	}
}
