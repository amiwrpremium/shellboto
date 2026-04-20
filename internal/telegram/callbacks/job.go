package callbacks

import (
	"context"
	"strings"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/shell"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
)

// HandleJob handles `j:c` (SIGINT) and `j:k` (SIGKILL) from the live
// streaming message's inline keyboard.
func HandleJob(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.CallbackQuery, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, cb *tgm.CallbackQuery, u *dbm.User) {
		parts := strings.SplitN(cb.Data, ":", 2)
		if len(parts) != 2 {
			common.AnswerCallback(ctx, b, cb.ID, "bad callback", false)
			return
		}
		action := parts[1]
		s, err := d.Shells.Get(u.TelegramID, shell.SpawnOpts{})
		if err != nil || s.Current() == nil {
			common.AnswerCallback(ctx, b, cb.ID, "no active command", false)
			return
		}
		j := s.Current()
		switch action {
		case "c":
			j.SetTermination("canceled")
			if err := s.SigInt(); err != nil {
				common.AnswerCallback(ctx, b, cb.ID, "cancel failed: "+err.Error(), true)
				return
			}
			common.AnswerCallback(ctx, b, cb.ID, "↯ SIGINT sent", false)
		case "k":
			j.SetTermination("killed")
			if err := s.SigKill(); err != nil {
				common.AnswerCallback(ctx, b, cb.ID, "kill failed: "+err.Error(), true)
				return
			}
			common.AnswerCallback(ctx, b, cb.ID, "☠ SIGKILL sent", false)
		default:
			common.AnswerCallback(ctx, b, cb.ID, "unknown action", false)
		}
	}
}
