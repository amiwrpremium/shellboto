// Package keyboards holds reusable inline-keyboard builders. Every
// button's callback data is constructed via the namespaces package so
// there are no raw "dc:y" strings here either.
package keyboards

import (
	"github.com/go-telegram/bot/models"

	ns "github.com/amiwrpremium/shellboto/internal/telegram/namespaces"
)

// RunningKeyboard is the [↯ Cancel] [☠ Kill] pair attached to the
// live-streaming message while a command runs.
func Running() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{{
			{Text: "↯ Cancel", CallbackData: ns.CBData(ns.Job, ns.JobCancel)},
			{Text: "☠ Kill", CallbackData: ns.CBData(ns.Job, ns.JobKill)},
		}},
	}
}

// Empty clears any existing keyboard on a message when edited.
func Empty() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{},
	}
}

// BackToList returns a single [◀ Back] button pointing at `<prefix>:l`.
func BackToList(prefix ns.NS) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: "◀ Back", CallbackData: ns.CBData(prefix, ns.List)},
	}}}
}

// YesNoTokenized builds a [Yes][No] pair where Yes carries a string token
// and No is a plain terminator. Used by the /adduser summary and the
// danger confirm warning.
func YesNoTokenized(prefix ns.NS, yesAction, noAction ns.Action, token string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: "✅ Yes", CallbackData: ns.CBDataToken(prefix, yesAction, token)},
		{Text: "✖ No", CallbackData: ns.CBData(prefix, noAction)},
	}}}
}

// YesNoID builds a [Yes][No] pair where both buttons carry the same id.
// Used by promote/demote/remove/reinstate confirm screens where the No
// button returns to the target's profile (e.g. "us:p:<id>").
func YesNoID(prefix ns.NS, yesAction, noAction ns.Action, id int64) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: "✅ Yes", CallbackData: ns.CBData(prefix, yesAction, id)},
		{Text: "✖ No", CallbackData: ns.CBData(prefix, noAction, id)},
	}}}
}

// YesNoSimple builds a [Yes][No] pair where Yes carries the target id and
// No is a plain terminator with no id. Used by promote/demote confirm.
func YesNoSimple(prefix ns.NS, yesAction, noAction ns.Action, id int64) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: "✅ Yes", CallbackData: ns.CBData(prefix, yesAction, id)},
		{Text: "✖ No", CallbackData: ns.CBData(prefix, noAction)},
	}}}
}
