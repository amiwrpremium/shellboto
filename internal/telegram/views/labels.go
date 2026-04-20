// Package views builds reusable Telegram-facing payloads — button labels,
// profile cards, candidate lists. Called from both commands/ and
// callbacks/ so the rendering code isn't duplicated across the entry
// points and the back-navigation paths.
package views

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/amiwrpremium/shellboto/internal/db/models"
)

// SanitizeDisplayName normalizes a user-supplied name for display.
// Unlike the /adduser intake regex (which strictly rejects non-ASCII
// and bans violators), this is a permissive cleanup: keep Unicode
// letters, digits, punctuation, spaces — but strip display-hostile
// bytes that break layouts or enable mild UI spoofing. Legacy rows
// predating the strict intake regex might carry such bytes; names that
// already passed intake are unchanged here (defense-in-depth).
//
// Strips:
//   - C0 + C1 control characters (0x00-0x1F, 0x7F-0x9F).
//   - Zero-width + bidi-control chars (ZWSP/ZWNJ/ZWJ/LRM/RLM/LRE/RLE/
//     PDF/LRO/RLO/WJ/LRI/RLI/FSI/PDI/BOM).
//   - Collapses whitespace runs (incl. \n, \r, \t) to a single space.
//
// Trims surrounding whitespace. Returns "" for empty input.
func SanitizeDisplayName(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	emitSpace := func() {
		if !inSpace && b.Len() > 0 {
			b.WriteByte(' ')
			inSpace = true
		}
	}
	for _, r := range s {
		switch {
		// Ordinary ASCII whitespace → collapse to one space.
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			emitSpace()
		// Strip zero-width + bidi-formatting runes (some of which
		// unicode.IsControl considers "format", caught here first).
		case r == '\u200B' || r == '\u200C' || r == '\u200D' ||
			r == '\u200E' || r == '\u200F' ||
			r == '\u202A' || r == '\u202B' || r == '\u202C' ||
			r == '\u202D' || r == '\u202E' ||
			r == '\u2060' ||
			r == '\u2066' || r == '\u2067' || r == '\u2068' || r == '\u2069' ||
			r == '\uFEFF':
			// drop
		// C0 (non-whitespace) + C1 controls. Includes NEL (U+0085)
		// which is technically whitespace but also a C1 control; we
		// strip rather than collapse because it's exotic and a legacy
		// name containing it is almost certainly bad data.
		case unicode.IsControl(r):
			// drop
		// Unicode whitespace that isn't already handled (NBSP, etc.).
		case unicode.IsSpace(r):
			emitSpace()
		default:
			b.WriteRune(r)
			inSpace = false
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// RoleEmoji returns the single-glyph role badge.
func RoleEmoji(u *models.User) string {
	switch u.Role {
	case models.RoleSuperadmin:
		return "👑"
	case models.RoleAdmin:
		return "⚡"
	default:
		return "👤"
	}
}

// DisplayLabel returns a short human identifier — name, else @handle,
// else id:N. Capped at 60 chars for button text. Name is passed
// through SanitizeDisplayName so legacy rows with weird
// bytes render cleanly.
func DisplayLabel(u *models.User) string {
	name := SanitizeDisplayName(u.Name)
	if name == "" && u.Username != "" {
		name = "@" + u.Username
	}
	if name == "" {
		name = fmt.Sprintf("id:%d", u.TelegramID)
	}
	if len(name) > 60 {
		name = name[:60] + "…"
	}
	return name
}

// UserListLabel is the button label used in /users: role emoji + name,
// with a 🚫 prefix when banned.
func UserListLabel(u *models.User) string {
	prefix := RoleEmoji(u)
	if u.DisabledAt != nil {
		prefix = "🚫 " + prefix
	}
	return prefix + " " + DisplayLabel(u)
}

// RelTime is "3h ago" / "2d ago" / "just now" — used for last-activity.
func RelTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 0:
		return t.UTC().Format("2006-01-02 15:04")
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
