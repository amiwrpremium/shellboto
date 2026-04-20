package views

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgm "github.com/go-telegram/bot/models"

	dbm "github.com/amiwrpremium/shellboto/internal/db/models"
	"github.com/amiwrpremium/shellboto/internal/db/repo"
	ns "github.com/amiwrpremium/shellboto/internal/telegram/namespaces"
	"github.com/amiwrpremium/shellboto/internal/telegram/rbac"
)

// UsersListMaxButtons caps the /users list keyboard size.
const UsersListMaxButtons = 50

// CandidateListMaxButtons caps promote/demote list keyboard size.
const CandidateListMaxButtons = 50

// BuildUsersList renders the /users screen: one button per user (role emoji
// + 🚫 prefix if banned). Active users first, then banned. No Cancel row —
// /users is the browser home, not a flow with a cancel.
func BuildUsersList(users []*dbm.User) (string, *tgm.InlineKeyboardMarkup) {
	shown := users
	truncated := false
	if len(shown) > UsersListMaxButtons {
		shown = shown[:UsersListMaxButtons]
		truncated = true
	}
	rows := make([][]tgm.InlineKeyboardButton, 0, len(shown))
	for _, x := range shown {
		rows = append(rows, []tgm.InlineKeyboardButton{{
			Text:         UserListLabel(x),
			CallbackData: ns.CBData(ns.Users, ns.Profile, x.TelegramID),
		}})
	}
	text := fmt.Sprintf("users (%d)", len(users))
	if truncated {
		text += fmt.Sprintf(" — showing first %d", UsersListMaxButtons)
	}
	return text, &tgm.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// BuildProfileText renders a user's profile card. `whoIs` resolves a
// telegram_id to a short identifier (caller-supplied because it needs DB
// access). `lastActivity` is the latest audit ts; zero Time = "none".
func BuildProfileText(t *dbm.User, whoIs func(int64) string, lastActivityRel string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s\n", RoleEmoji(t), DisplayLabel(t))
	fmt.Fprintf(&sb, "id: %d", t.TelegramID)
	if t.Username != "" {
		fmt.Fprintf(&sb, " · @%s", t.Username)
	}
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "role: %s", t.Role)
	if t.Role == dbm.RoleAdmin && t.PromotedBy != nil {
		fmt.Fprintf(&sb, " · promoted by %s", whoIs(*t.PromotedBy))
	}
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "added: %s", t.AddedAt.UTC().Format("2006-01-02"))
	if t.AddedBy != nil {
		fmt.Fprintf(&sb, " by %s", whoIs(*t.AddedBy))
	}
	sb.WriteString("\n")
	if lastActivityRel != "" {
		fmt.Fprintf(&sb, "last activity: %s\n", lastActivityRel)
	} else {
		sb.WriteString("last activity: —\n")
	}
	if t.DisabledAt != nil {
		fmt.Fprintf(&sb, "status: 🚫 banned since %s\n", t.DisabledAt.UTC().Format("2006-01-02"))
	}
	return sb.String()
}

// BuildProfileKeyboard chooses action buttons based on caller perms +
// target state. Callers (both /users and callback handlers) feed in
// already-loaded target + caller rows.
func BuildProfileKeyboard(caller, target *dbm.User) *tgm.InlineKeyboardMarkup {
	isSelf := caller.TelegramID == target.TelegramID
	banned := target.DisabledAt != nil
	tidStr := strconv.FormatInt(target.TelegramID, 10)
	_ = tidStr

	var rows [][]tgm.InlineKeyboardButton

	// Row 1: role-change / reinstate (mutually exclusive, context-gated).
	if !isSelf && !target.IsSuperadmin() {
		if banned {
			if rbac.CanActOnLifecycle(caller, target) {
				rows = append(rows, []tgm.InlineKeyboardButton{{
					Text:         "⬆ Reinstate",
					CallbackData: ns.CBData(ns.Users, ns.Reinstate, target.TelegramID),
				}})
			}
		} else {
			switch target.Role {
			case dbm.RoleUser:
				if rbac.CanPromote(caller, target) {
					rows = append(rows, []tgm.InlineKeyboardButton{{
						Text:         "⬆ Promote",
						CallbackData: ns.CBData(ns.Promote, ns.Select, target.TelegramID),
					}})
				}
			case dbm.RoleAdmin:
				if rbac.CanDemote(caller, target) {
					rows = append(rows, []tgm.InlineKeyboardButton{{
						Text:         "⬇ Demote",
						CallbackData: ns.CBData(ns.Demote, ns.Select, target.TelegramID),
					}})
				}
			}
		}
	}

	// Row 2: remove (active non-self non-super, perm-gated).
	if !isSelf && !banned && !target.IsSuperadmin() && rbac.CanActOnLifecycle(caller, target) {
		rows = append(rows, []tgm.InlineKeyboardButton{{
			Text:         "🗑 Remove",
			CallbackData: ns.CBData(ns.Users, ns.Remove, target.TelegramID),
		}})
	}

	// Row 3: view actions (always available).
	rows = append(rows, []tgm.InlineKeyboardButton{
		{Text: "📜 Audit", CallbackData: ns.CBData(ns.Users, ns.Audit, target.TelegramID)},
		{Text: "💬 Commands", CallbackData: ns.CBData(ns.Users, ns.UsrCommands, target.TelegramID)},
	})
	if !isSelf {
		rows = append(rows, []tgm.InlineKeyboardButton{{
			Text:         "📎 Last output",
			CallbackData: ns.CBData(ns.Users, ns.Output, target.TelegramID),
		}})
	}

	// Row 4: back to list.
	rows = append(rows, []tgm.InlineKeyboardButton{{
		Text:         "◀ Back",
		CallbackData: ns.CBData(ns.Users, ns.List),
	}})

	return &tgm.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// BuildCandidateList builds a one-button-per-user message for /promote and
// /demote entry screens. `prefix` is ns.Promote or ns.Demote.
func BuildCandidateList(prompt string, users []*dbm.User, prefix ns.NS) (string, *tgm.InlineKeyboardMarkup) {
	shown := users
	truncated := false
	if len(shown) > CandidateListMaxButtons {
		shown = shown[:CandidateListMaxButtons]
		truncated = true
	}
	rows := make([][]tgm.InlineKeyboardButton, 0, len(shown)+1)
	for _, x := range shown {
		rows = append(rows, []tgm.InlineKeyboardButton{{
			Text:         DisplayLabel(x),
			CallbackData: ns.CBData(prefix, ns.Select, x.TelegramID),
		}})
	}
	rows = append(rows, []tgm.InlineKeyboardButton{{
		Text:         "✖ Cancel",
		CallbackData: ns.CBData(prefix, ns.No),
	}})
	if truncated {
		prompt += fmt.Sprintf("\n(showing first %d of %d)", CandidateListMaxButtons, len(users))
	}
	return prompt, &tgm.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// WhoIsFunc returns a closure that resolves a telegram_id → short name,
// suitable to pass into BuildProfileText.
func WhoIsFunc(users *repo.UserRepo) func(int64) string {
	return func(tid int64) string {
		u, err := users.Lookup(tid)
		if err != nil {
			return fmt.Sprintf("id:%d", tid)
		}
		if u.Username != "" {
			return "@" + u.Username
		}
		if u.Name != "" {
			return u.Name
		}
		return fmt.Sprintf("id:%d", tid)
	}
}

// LastActivityOf returns the relative time of the user's most recent audit
// row, or "" when there is none.
func LastActivityOf(ctx context.Context, audit *repo.AuditRepo, userID int64) string {
	ts, err := audit.LastActivity(ctx, userID)
	if err != nil {
		return ""
	}
	return RelTime(ts)
}

// FormatAudit renders a slice of audit rows as a compact text block, used
// by /audit and /auditme.
func FormatAudit(rows []*repo.Row) string {
	if len(rows) == 0 {
		return "no audit rows"
	}
	var sb strings.Builder
	for _, r := range rows {
		ts := r.TS.Format("01-02 15:04:05")
		user := "—"
		if r.UserID != nil {
			user = strconv.FormatInt(*r.UserID, 10)
		}
		extra := ""
		if r.Kind == dbm.KindCommandRun {
			exit := "?"
			if r.ExitCode != nil {
				exit = strconv.Itoa(*r.ExitCode)
			}
			bytesOut := 0
			if r.BytesOut != nil {
				bytesOut = *r.BytesOut
			}
			outMark := ""
			if r.HasOutput {
				outMark = " ⟶out"
			}
			extra = fmt.Sprintf(" exit=%s bytes=%d%s cmd=%s", exit, bytesOut, outMark, shortStr(r.Cmd, 80))
		} else if r.Cmd != "" {
			extra = " cmd=" + shortStr(r.Cmd, 80)
		} else if r.Detail != "" {
			extra = " " + shortStr(r.Detail, 80)
		}
		fmt.Fprintf(&sb, "#%d %s %s %s%s\n", r.ID, ts, user, r.Kind, extra)
	}
	return sb.String()
}

func shortStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
