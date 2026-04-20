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
	"github.com/amiwrpremium/shellboto/internal/telegram/middleware"
)

// DefaultHandler is the fall-through for any message that isn't a
// registered slash command. It handles:
//   - plain text → shell (or /adduser wizard answer if mid-flow)
//   - documents → upload to shell cwd
//   - photos/video/voice → polite rejection
//   - unauthorized senders → silent drop + auth_reject audit
func DefaultHandler(d *deps.Deps) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update) {
		middleware.TrimUpdate(update)
		if update.Message == nil || update.Message.From == nil {
			return
		}
		usr, err := d.Users.LookupActive(update.Message.From.ID)
		if err != nil {
			// Silent drop + rate-limited audit via the shared reject
			// path — deduplicates the log/audit call with the
			// WrapText flow and gates it behind AuthRejectLimit.
			middleware.LogRejectText(ctx, d, update)
			return
		}
		d.Users.Touch(usr.TelegramID, update.Message.From.Username, update.Message.From.FirstName)

		if update.Message.Document != nil {
			handleDocumentUpload(ctx, d, b, update, usr)
			return
		}
		if update.Message.Photo != nil || update.Message.Video != nil || update.Message.Voice != nil || update.Message.Audio != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "only documents are accepted — send as File (paperclip → File)")
			return
		}

		// Mid-flow /adduser: next plain text is the answer.
		if flow := d.AddUser.Current(usr.TelegramID); flow != nil {
			HandleAddUserFlowText(ctx, d, b, update, usr, flow)
			return
		}

		// TrimUpdate already ran at top.
		text := update.Message.Text
		if text == "" {
			return
		}
		ExecShell(ctx, d, b, update.Message.Chat.ID, usr, text)
	}
}

func handleDocumentUpload(ctx context.Context, d *deps.Deps, b *bot.Bot, update *tgm.Update, u *dbm.User) {
	s, err := d.Shells.Get(u.TelegramID, ShellOptsFor(d, u))
	if err != nil {
		common.Reply(ctx, b, update.Message.Chat.ID, "no shell: "+err.Error())
		return
	}
	cwd, err := ShellCwd(s.BashPID())
	if err != nil {
		common.Reply(ctx, b, update.Message.Chat.ID,
			"upload failed: couldn't determine your shell's cwd (your shell may have exited). send any command to respawn, then retry.")
		return
	}
	var chown *files.ChownTo
	if !u.IsAdminOrAbove() && d.UserShellCreds != nil {
		chown = &files.ChownTo{Uid: d.UserShellCreds.Uid, Gid: d.UserShellCreds.Gid}
	}
	saved, nbytes, err := files.Receive(ctx, b, update.Message.Document, update.Message.Caption, cwd, chown)
	uid := u.TelegramID
	if err != nil {
		common.Reply(ctx, b, update.Message.Chat.ID, "upload failed: "+err.Error())
		_, _ = d.Audit.Log(ctx, repo.Event{
			UserID: &uid, Kind: dbm.KindFileUpload,
			Detail: map[string]any{"ok": false, "err": err.Error()},
		})
		return
	}
	sz := int(nbytes)
	_, _ = d.Audit.Log(ctx, repo.Event{
		UserID: &uid, Kind: dbm.KindFileUpload, BytesOut: &sz,
		Detail: map[string]any{
			"ok":       true,
			"filename": update.Message.Document.FileName,
			"saved_to": saved,
		},
	})
	common.Reply(ctx, b, update.Message.Chat.ID, "saved → "+saved)
}
