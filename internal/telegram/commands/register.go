package commands

import (
	"github.com/go-telegram/bot"

	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
	"github.com/amiwrpremium/shellboto/internal/telegram/middleware"
)

// RegisterAll wires every /command to the given bot.Bot. Each handler is
// wrapped with middleware.WrapText for auth + row-touching.
func RegisterAll(b *bot.Bot, d *deps.Deps) {
	reg := func(cmd string, h middleware.TextHandler) {
		b.RegisterHandler(bot.HandlerTypeMessageText, cmd, bot.MatchTypeCommand, middleware.WrapText(d, h))
	}
	reg("/start", HandleStart(d))
	reg("/help", HandleHelp(d))
	reg("/cancel", HandleCancel(d))
	reg("/kill", HandleKill(d))
	reg("/status", HandleStatus(d))
	reg("/reset", HandleReset(d))
	reg("/get", HandleGet(d))
	reg("/adduser", HandleAddUser(d))
	reg("/deluser", HandleDelUser(d))
	reg("/promote", HandlePromote(d))
	reg("/demote", HandleDemote(d))
	reg("/audit", HandleAudit(d))
	reg("/auditme", HandleAuditMe(d))
	reg("/audit-out", HandleAuditOut(d))
	reg("/audit-verify", HandleAuditVerify(d))
	reg("/users", HandleUsers(d))
}
