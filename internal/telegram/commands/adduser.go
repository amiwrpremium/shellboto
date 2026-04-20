package commands

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
	"github.com/amiwrpremium/shellboto/internal/telegram/flows"
	"github.com/amiwrpremium/shellboto/internal/telegram/keyboards"
	ns "github.com/amiwrpremium/shellboto/internal/telegram/namespaces"
)

// nameRegex is the strict format for /adduser names: one or more
// English letter groups separated by exactly one space. Violations
// trigger an automatic ban: an admin who tries to smuggle
// weird bytes (newlines, @mentions, RTL overrides, control chars,
// etc.) into a name that'll surface in super's notifications is doing
// something hostile, not typoing.
var nameRegex = regexp.MustCompile(`^[A-Za-z]+(?: [A-Za-z]+)*$`)

// maxTelegramID is a generous sanity cap on numeric Telegram user IDs.
// Real user IDs today are in the ~10^10 range; bot IDs can be larger
// but still well under 10^13. This exists to reject obvious typos
// (pasted long strings, accidental duplicate digits) before we spend a
// Telegram API round-trip on GetChat.
const maxTelegramID int64 = 10_000_000_000_000

// HandleAddUser kicks off the /adduser wizard. Admin+.
func HandleAddUser(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		if !RequireAdmin(ctx, b, update, u) {
			return
		}
		d.AddUser.Start(u.TelegramID)
		common.Reply(ctx, b, update.Message.Chat.ID,
			"adding new user.\nsend the Telegram user ID of the person to add, or /cancel to abort.")
	}
}

// HandleAddUserFlowText is called from default.go when the caller has an
// active /adduser wizard. Consumes plain text as the current step's answer.
// update.Message.Text has already been trimmed by middleware.TrimUpdate.
//
// Re-check admin status at the top. If the caller was demoted
// (by super) between wizard steps, cancel the flow rather than let a
// now-user-role caller complete /adduser. `u` was freshly loaded by
// the DefaultHandler's LookupActive, so its Role reflects current state.
func HandleAddUserFlowText(ctx context.Context, d *deps.Deps, b *bot.Bot, update *tgm.Update, u *dbm.User, fl *flows.AddUserFlow) {
	chatID := update.Message.Chat.ID

	if !u.IsAdminOrAbove() {
		d.AddUser.Cancel(u.TelegramID)
		common.Reply(ctx, b, chatID, "adduser canceled: you are no longer an admin.")
		return
	}

	text := update.Message.Text

	switch fl.Step {
	case flows.StepAwaitID:
		id, err := strconv.ParseInt(text, 10, 64)
		// Cheap range sanity: positive, and within a generous upper bound
		// that's well past any real Telegram user ID today (~10^10) but
		// rejects obvious finger-slip overflows (extra digits).
		if err != nil || id < 1 || id > maxTelegramID {
			common.Reply(ctx, b, chatID, "invalid id — send a positive integer in Telegram range, or /cancel.")
			return
		}
		// Verify the ID resolves to a chat the bot can see. Succeeds
		// only if the target has messaged this bot at least once (or the
		// bot shares a group with them). Blocks the "typo lands on a real
		// user's ID that was never intended" reservation hole.
		if _, err := b.GetChat(ctx, &bot.GetChatParams{ChatID: id}); err != nil {
			common.Reply(ctx, b, chatID,
				"could not verify user "+strconv.FormatInt(id, 10)+
					" — they must message this bot at least once before you can add them.\nsend a different id, or /cancel.")
			return
		}
		d.AddUser.Advance(u.TelegramID, func(f *flows.AddUserFlow) {
			f.TargetID = id
			f.Step = flows.StepAwaitName
		})
		common.Reply(ctx, b, chatID,
			"send the full name — English letters only, single spaces between words. "+
				"any other characters (digits, punctuation, accents, @, newlines) will ban you.\n"+
				"/cancel to abort.")

	case flows.StepAwaitName:
		if text == "" {
			common.Reply(ctx, b, chatID, "name can't be empty — try again or /cancel.")
			return
		}
		if len(text) > 200 {
			text = text[:200]
		}
		// Names are strictly English letters + single spaces
		// between words. Anything else is either a mistake we'd rather
		// surface now than at notification-display time, or a hostile
		// attempt to inject control chars / fake @mentions into super's
		// DM. Ban-on-violation matches the super-demotion-via-upsert
		// response for known-deliberate /adduser misuse.
		if !nameRegex.MatchString(text) {
			d.AddUser.Cancel(u.TelegramID)
			BanUser(ctx, d, b, chatID, u,
				"/adduser name: "+common.Truncate(text, 200),
				"submitted invalid /adduser name — names must be English letters with single spaces only")
			return
		}
		tok := flows.NewToken()
		var target int64
		d.AddUser.Advance(u.TelegramID, func(f *flows.AddUserFlow) {
			f.Name = text
			f.Token = tok
			f.Step = flows.StepAwaitConfirm
			target = f.TargetID
		})
		summary := fmt.Sprintf("adding new user\nuser id: %d\nname: %s", target, text)
		kb := keyboards.YesNoTokenized(ns.AddUser, ns.Yes, ns.No, tok)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID, Text: summary, ReplyMarkup: kb,
		})

	case flows.StepAwaitConfirm:
		common.Reply(ctx, b, chatID, "tap ✅ Yes or ✖ No, or /cancel.")
	}
}
