package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/redact"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
)

// HandleStatus: /status reports whether a command is running + runtime.
func HandleStatus(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		s, err := d.Shells.Get(u.TelegramID, ShellOptsFor(d, u))
		if err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "no shell")
			return
		}
		j := s.Current()
		if j == nil {
			common.Reply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("idle · bash pid %d · last activity %s ago",
				s.BashPID(), time.Since(s.LastActivity()).Round(time.Second)))
			return
		}
		snap, _ := j.Snapshot()
		preview := string(snap)
		if len(preview) > 120 {
			preview = "…" + preview[len(preview)-120:]
		}
		// The cmd + preview land in Telegram chat history, so a
		// session leak would expose secrets unless we scrub here.
		// Redact BEFORE the display truncate so [REDACTED] placeholders
		// don't get cut mid-word.
		preview = redact.RedactString(preview)
		cmdSafe := common.Truncate(redact.RedactString(j.Cmd), 120)
		common.Reply(ctx, b, update.Message.Chat.ID, fmt.Sprintf(
			"running for %s · %d bytes of output\ncmd: %s\ntail: %s",
			time.Since(j.Started).Round(time.Second), len(snap), cmdSafe, preview))
	}
}
