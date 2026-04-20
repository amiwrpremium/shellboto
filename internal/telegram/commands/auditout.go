package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/files"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
)

// HandleAuditOut: /audit-out <id> — admin+ fetches the gzipped output blob
// for a command_run row.
func HandleAuditOut(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		if !RequireAdmin(ctx, b, update, u) {
			return
		}
		parts := strings.Fields(TrimCmdArg(update.Message.Text, "/audit-out"))
		if len(parts) != 1 {
			common.Reply(ctx, b, update.Message.Chat.ID, "usage: /audit-out <audit_id>")
			return
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "bad audit_id")
			return
		}
		gz, orig, err := d.Audit.FetchOutput(ctx, id)
		if err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "no output for id "+parts[0])
			return
		}
		filename := fmt.Sprintf("audit-%d.txt.gz", id)
		caption := fmt.Sprintf("audit #%d · %d bytes gzipped · %d bytes original", id, len(gz), orig)
		if err := WithUploadIndicator(ctx, d, update.Message.Chat.ID, func() error {
			return files.SendBytes(ctx, b, update.Message.Chat.ID, filename, gz, caption)
		}); err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "send failed: "+err.Error())
		}
	}
}
