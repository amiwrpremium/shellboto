package commands

import (
	"context"
	"fmt"

	"github.com/go-telegram/bot"
	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/telegram/common"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
)

// HandleAuditVerify: /audit-verify — walks the audit hash chain and
// reports the first tampered row (or "OK / N rows verified"). Admin+.
// Detects row edits and row deletions. Does NOT prevent tampering; a
// determined attacker with root + access to the seed can still forge a
// fresh chain.
func HandleAuditVerify(d *deps.Deps) func(context.Context, *bot.Bot, *tgm.Update, *dbm.User) {
	return func(ctx context.Context, b *bot.Bot, update *tgm.Update, u *dbm.User) {
		if !RequireAdmin(ctx, b, update, u) {
			return
		}
		res, err := d.Audit.Verify(ctx)
		if err != nil {
			common.Reply(ctx, b, update.Message.Chat.ID, "verify failed: "+err.Error())
			return
		}
		if res.OK {
			msg := fmt.Sprintf("✅ audit chain OK — %d rows verified.", res.VerifiedRows)
			if res.PostPrune {
				// Retention prune removed the genesis row; Verify can
				// still check chain continuity between surviving rows
				// but can't attest the link back to the original seed.
				// The audit journal (journald) remains the pre-prune
				// source of truth.
				msg += " (post-prune; genesis not checked)"
			}
			common.Reply(ctx, b, update.Message.Chat.ID, msg)
			return
		}
		reason := res.Reason
		if res.PostPrune {
			reason += " (post-prune state)"
		}
		common.Reply(ctx, b, update.Message.Chat.ID, fmt.Sprintf(
			"❌ audit chain BROKEN\nrows verified before the break: %d\nfirst bad id: %d\nreason: %s",
			res.VerifiedRows, res.FirstBadID, reason))
	}
}
