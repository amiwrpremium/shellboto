package callbacks

import (
	"context"
	"fmt"

	"github.com/go-telegram/bot"
	"go.uber.org/zap"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
	"github.com/amiwrpremium/shellboto/internal/telegram/views"
)

// notifyPromoted tells the target they've been promoted to admin.
func notifyPromoted(ctx context.Context, d *deps.Deps, b *bot.Bot, target *dbm.User, actor *dbm.User) {
	whoIs := views.WhoIsFunc(d.Users)
	text := fmt.Sprintf("🎉 you have been promoted to admin by %s.", whoIs(actor.TelegramID))
	sendDM(ctx, d, b, target.TelegramID, text)
}

// notifyDemoted tells the target they've been demoted to user.
// `cascadedFromID` is the telegram_id of the direct promoter whose
// demotion cascaded into this one; 0 means it's the primary target the
// actor clicked on directly.
func notifyDemoted(ctx context.Context, d *deps.Deps, b *bot.Bot, targetID, actorID, cascadedFromID int64) {
	whoIs := views.WhoIsFunc(d.Users)
	var text string
	if cascadedFromID == 0 {
		text = fmt.Sprintf("⬇ you have been demoted to user by %s.", whoIs(actorID))
	} else {
		text = fmt.Sprintf(
			"⬇ you have been demoted to user.\n%s (who promoted you) was demoted, so you lost admin rights.",
			whoIs(cascadedFromID))
	}
	sendDM(ctx, d, b, targetID, text)
}

// sendDM sends a private message to a user's telegram_id, logging at Warn
// on failure (typically: the target has never /start'd the bot so there's
// no accessible private chat). Never propagates the error back up to the
// caller — notification failure must not roll back the role change.
func sendDM(ctx context.Context, d *deps.Deps, b *bot.Bot, telegramID int64, text string) {
	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: telegramID,
		Text:   text,
	}); err != nil {
		d.L().Warn("dm notification failed",
			zap.Int64("target", telegramID),
			zap.Error(err),
		)
	}
}
