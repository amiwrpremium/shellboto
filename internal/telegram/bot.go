// Package telegram wires up the bot: one New(deps) returns a ready-to-
// Start *bot.Bot with every command and callback registered.
package telegram

import (
	"context"

	"github.com/go-telegram/bot"

	"github.com/amiwrpremium/shellboto/internal/telegram/callbacks"
	"github.com/amiwrpremium/shellboto/internal/telegram/commands"
	"github.com/amiwrpremium/shellboto/internal/telegram/deps"
)

// New constructs the underlying *bot.Bot and registers every handler.
// The default (fall-through) handler is set via bot.WithDefaultHandler so
// plain text / documents / media land on commands.DefaultHandler.
func New(token string, d *deps.Deps) (*bot.Bot, error) {
	b, err := bot.New(token, bot.WithDefaultHandler(commands.DefaultHandler(d)))
	if err != nil {
		return nil, err
	}
	commands.RegisterAll(b, d)
	callbacks.RegisterAll(b, d)
	return b, nil
}

// Start is a thin pass-through so callers don't need to import go-telegram
// just to start the bot.
func Start(ctx context.Context, b *bot.Bot) {
	b.Start(ctx)
}
