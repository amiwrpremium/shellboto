package commands

import (
	"context"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
)

const helpBase = `commands:
plain text      — run as shell
/cancel         — SIGINT (Ctrl+C) foreground process
/kill           — SIGKILL foreground process group
/status         — show running command + runtime
/reset          — kill your shell, respawn fresh
/auditme [N]    — your recent audit events
/start /help    — these`

const helpAdmin = `

admin+:
/users        — interactive user browser
/adduser      — add-user wizard (id → name → confirm)
/deluser <id> — remove user
/promote      — pick a user to promote to admin
/demote       — pick an admin to demote
/get <path>   — download file from VPS (bot reads as root, so gated to admin+)
/audit [N]    — recent audit rows
/audit-out <id> — fetch stored output for audit row
/audit-verify — scan the hash chain for tampering`

const helpUpload = `

upload: paperclip → File. Saves to your shell cwd, or to the path in caption.`

// HandleHelp returns the help text tailored to the caller's role.
func HandleHelp(_ *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		text := helpBase + helpUpload
		if u.IsAdminOrAbove() {
			text = helpBase + helpAdmin + helpUpload
		}
		common.Reply(ctx, b, update.Message.Chat.ID, text)
	}
}
