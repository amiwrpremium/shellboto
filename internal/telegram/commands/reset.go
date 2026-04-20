package commands

import (
	"context"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/db/repo"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
)

// HandleReset: /reset kills the user's bash; next message respawns fresh.
func HandleReset(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		uid := u.TelegramID
		d.Shells.Reset(uid)
		_, _ = d.Audit.Log(ctx, repo.Event{UserID: &uid, Kind: dbm.KindShellReset})
		common.Reply(ctx, b, update.Message.Chat.ID, "shell killed — will respawn on next message")
	}
}
