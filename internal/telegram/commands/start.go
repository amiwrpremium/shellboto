package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
)

// HandleStart greets the caller with host, role, and shell pid.
func HandleStart(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		s, err := d.Shells.Get(u.TelegramID, ShellOptsFor(d, u))
		if err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "failed to spawn shell: "+err.Error())
			return
		}
		hn, _ := os.Hostname()
		common.Reply(ctx, b, update.Message.Chat.ID, fmt.Sprintf(
			"shellboto ready\nhost: %s\nyour role: %s\nshell pid: %d\nsend any command — /help for controls",
			hn, u.Role, s.BashPID()))
	}
}
