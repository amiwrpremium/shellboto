package commands

import (
	"context"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/db/repo"
	"github.com/amiwrpremium/shellboto/internal/files"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
)

// HandleGet: /get <path> sends the file as a Telegram document.
//
// Admin+ only. The bot process is root, so /get reads ANY file on the
// filesystem regardless of the caller's shell permissions — that would
// let a `user`-role caller pull /etc/shadow, /root/.ssh/id_rsa, or even
// the audit DB and bot token. Restricting to admin+ matches the trust
// model: admins are already root, so /get exposes nothing they couldn't
// read from their shell anyway.
func HandleGet(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		if !RequireAdmin(ctx, b, update, u) {
			// Also audit the attempt so admins can see if users probe.
			uid := u.TelegramID
			_, _ = d.Audit.Log(ctx, repo.Event{
				UserID: &uid, Kind: dbm.KindAuthReject,
				Cmd:    update.Message.Text,
				Detail: map[string]any{"reason": "/get is admin+ only"},
			})
			return
		}
		arg := TrimCmdArg(update.Message.Text, "/get")
		if arg == "" {
			common.Reply(ctx, b, update.Message.Chat.ID, "usage: /get <path>")
			return
		}
		var size int64
		err := WithUploadIndicator(ctx, d, update.Message.Chat.ID, func() error {
			var e error
			size, e = files.Send(ctx, b, update.Message.Chat.ID, arg)
			return e
		})
		uid := u.TelegramID
		if err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "get failed: "+err.Error())
			_, _ = d.Audit.Log(ctx, repo.Event{
				UserID: &uid, Kind: dbm.KindFileDownload,
				Detail: map[string]any{"path": arg, "ok": false, "err": err.Error()},
			})
			return
		}
		sz := int(size)
		_, _ = d.Audit.Log(ctx, repo.Event{
			UserID: &uid, Kind: dbm.KindFileDownload, BytesOut: &sz,
			Detail: map[string]any{"path": arg, "ok": true},
		})
	}
}
