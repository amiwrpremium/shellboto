// Package deps exposes the shared-state bag every Telegram handler
// receives. It replaces the old Router struct. Kept in its own package to
// avoid the parent telegram/ package importing its children.
package deps

import (
	"syscall"

	"go.uber.org/zap"

	"github.com/amiwrpremium/shellboto/internal/config"
	"github.com/amiwrpremium/shellboto/internal/danger"
	"github.com/amiwrpremium/shellboto/internal/db/repo"
	"github.com/amiwrpremium/shellboto/internal/shell"
	"github.com/amiwrpremium/shellboto/internal/stream"
	"github.com/amiwrpremium/shellboto/internal/telegram/flows"
	"github.com/amiwrpremium/shellboto/internal/telegram/ratelimit"
	"github.com/amiwrpremium/shellboto/internal/telegram/supernotify"
)

// Deps is everything a Telegram handler might need. Built in main.go and
// threaded through commands.RegisterAll / callbacks.RegisterAll.
type Deps struct {
	Cfg      *config.Config
	Users    *repo.UserRepo
	Audit    *repo.AuditRepo
	Shells   *shell.Manager
	Streamer *stream.Streamer
	Danger   *danger.Matcher
	Confirm  *flows.ConfirmStore
	AddUser  *flows.AddUserFlows
	Log      *zap.Logger

	// UserShellCreds is nil when user-role shells should run as root
	// (dev mode). When set, `user`-role shells spawn with these credentials.
	UserShellCreds *syscall.Credential
	// UserShellHome is the base dir for per-telegram-user home dirs when
	// UserShellCreds is non-nil, e.g. "/home/shellboto-user".
	UserShellHome string

	// RateLimit is a per-user token-bucket applied in the middleware
	// wrappers. Nil / disabled = unlimited.
	RateLimit *ratelimit.Limiter

	// AuthRejectLimit is a SECOND rate limiter, keyed by sender From-id
	// regardless of whether they're authenticated. Applied before any
	// auth_reject audit write to prevent a non-whitelisted attacker
	// from filling the DB by spamming the bot. Nil / disabled
	// = unlimited (auth_reject writes are never rate-limited).
	AuthRejectLimit *ratelimit.Limiter

	// Notify DMs the super + actor's direct promoter for user-lifecycle
	// events (promote / demote / add / remove / ban) with quick-action
	// inline keyboards. Nil = no notifications.
	Notify *supernotify.Emitter
}

// L returns the logger, or a nop if unset.
func (d *Deps) L() *zap.Logger {
	if d.Log == nil {
		return zap.NewNop()
	}
	return d.Log
}
