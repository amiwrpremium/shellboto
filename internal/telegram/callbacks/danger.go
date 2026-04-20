package callbacks

import (
	"context"
	"strings"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/db/repo"
	"github.com/amiwrpremium/shellboto/internal/telegram/commands"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
	"github.com/amiwrpremium/shellboto/internal/telegram/flows"
	"github.com/amiwrpremium/shellboto/internal/telegram/middleware"
)

// HandleDanger handles `dc:y:<tok>` (run) and `dc:c:<tok>` (cancel) from
// the danger confirm warning. The inline button is the only path to
// confirm — the `/confirm <token>` text fallback was removed so the
// token never appears in plaintext message history.
func HandleDanger(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.CallbackQuery, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, cb *tgm.CallbackQuery, u *dbm.User) {
		parts := strings.SplitN(cb.Data, ":", 3)
		if len(parts) != 3 {
			common.AnswerCallback(ctx, b, cb.ID, "bad callback", false)
			return
		}
		action, token := parts[1], parts[2]
		chatID, msgID := common.CallbackMessageRef(cb)
		uid := u.TelegramID

		if action == "c" { // cancel
			d.Confirm.Claim(uid, token)
			common.AnswerCallback(ctx, b, cb.ID, "canceled", false)
			common.EditCallbackMessage(ctx, b, chatID, msgID, "✖ canceled")
			return
		}
		if action != "y" { // only "run" remains
			common.AnswerCallback(ctx, b, cb.ID, "unknown action", false)
			return
		}

		cmdStr, res := d.Confirm.Claim(uid, token)
		switch res {
		case flows.ClaimExpired:
			_, _ = d.Audit.Log(ctx, repo.Event{UserID: &uid, Kind: dbm.KindDangerExpired})
			common.AnswerCallback(ctx, b, cb.ID, "token expired", true)
			common.EditCallbackMessage(ctx, b, chatID, msgID, "⏱ token expired")
			return
		case flows.ClaimUnknown:
			common.AnswerCallback(ctx, b, cb.ID, "token invalid", true)
			common.EditCallbackMessage(ctx, b, chatID, msgID, "token invalid")
			return
		}
		// TOCTOU re-check: the caller might have been banned/
		// demoted between the warning message and their tap. Don't
		// execute their stashed command if they're no longer active.
		fresh := middleware.RefreshActiveCaller(d, uid)
		if fresh == nil {
			common.AnswerCallback(ctx, b, cb.ID, "not authorized anymore", true)
			common.EditCallbackMessage(ctx, b, chatID, msgID, "not authorized anymore")
			return
		}
		_, _ = d.Audit.Log(ctx, repo.Event{UserID: &uid, Kind: dbm.KindDangerConfirmed, Cmd: cmdStr})
		common.AnswerCallback(ctx, b, cb.ID, "running", false)
		common.EditCallbackMessage(ctx, b, chatID, msgID, "▶ running confirmed command…")
		commands.DispatchShell(ctx, d, b, chatID, fresh, cmdStr)
	}
}
