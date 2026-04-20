package callbacks

import (
	"github.com/go-telegram/bot"

	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
	"github.com/amiwrpremium/shellboto/internal/telegram/middleware"
	ns "github.com/amiwrpremium/shellboto/internal/telegram/namespaces"
)

// RegisterAll wires every callback-query namespace to its handler.
func RegisterAll(b *bot.Bot, d *deps.Deps) {
	reg := func(prefix ns.NS, h middleware.CallbackHandler) {
		b.RegisterHandler(bot.HandlerTypeCallbackQueryData, ns.CBPrefix(prefix), bot.MatchTypePrefix, middleware.WrapCallback(d, h))
	}
	reg(ns.Danger, HandleDanger(d))
	reg(ns.Job, HandleJob(d))
	reg(ns.AddUser, HandleAddUser(d))
	reg(ns.Promote, HandlePromote(d))
	reg(ns.Demote, HandleDemote(d))
	reg(ns.Users, HandleUsers(d))
}
