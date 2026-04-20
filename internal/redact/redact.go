// Package redact applies a best-effort scrub pass to audit content
// before it's persisted. It's NOT a substitute for not-storing-at-all
// (use audit_output_mode = never for that) and will miss novel secret
// shapes — the pattern list is a maintained default, not a guarantee.
//
// Every pattern is applied in order; the result is a new byte slice.
package redact

import "regexp"

type pattern struct {
	re  *regexp.Regexp
	sub string // replacement (may reference $1 etc.)
}

// patterns are applied in declared order. Each pattern's replacement
// should preserve enough structure that the surrounding line remains
// greppable (e.g. `password=foo` → `password=[REDACTED]` rather than
// blanking the whole line).
var patterns = []pattern{
	// SSH / TLS private keys — block-spanning, so (?s) for dotall.
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
		"[REDACTED-PRIVATE-KEY]"},

	// Database URIs with embedded credentials.
	{regexp.MustCompile(`\b(postgres|postgresql|mongodb|mongodb\+srv|mysql|mariadb|redis|amqp|amqps)://[^:/\s@]+:([^@\s/]+)@`),
		"$1://[REDACTED]:[REDACTED]@"},

	// HTTP auth headers.
	{regexp.MustCompile(`(?i)(authorization:\s*(bearer|basic))\s+\S+`),
		"$1 [REDACTED]"},

	// JWTs (three base64url segments separated by dots).
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\b`),
		"[REDACTED-JWT]"},

	// Well-known token formats.
	{regexp.MustCompile(`\bghp_[A-Za-z0-9]{36}\b`), "[REDACTED-GH-TOKEN]"},
	{regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,}\b`), "[REDACTED-GH-PAT]"},
	{regexp.MustCompile(`\bgho_[A-Za-z0-9]{36}\b`), "[REDACTED-GH-OAUTH]"},
	{regexp.MustCompile(`\bghs_[A-Za-z0-9]{36}\b`), "[REDACTED-GH-SECRET]"},
	{regexp.MustCompile(`\bglpat-[A-Za-z0-9_\-]{20,}\b`), "[REDACTED-GL-PAT]"},
	{regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`), "[REDACTED-GOOGLE-KEY]"},
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), "[REDACTED-AWS-KEY]"},
	{regexp.MustCompile(`\b(sk|pk|rk)_(live|test)_[A-Za-z0-9]{24,}\b`), "[REDACTED-STRIPE]"},
	{regexp.MustCompile(`\bxox[abopsr]-[A-Za-z0-9-]{10,}\b`), "[REDACTED-SLACK]"},

	// mysql-style `-ppassword` single token (run AFTER GH-style literal
	// tokens so it doesn't catch those). We only redact when the string
	// after -p is 4+ chars, to avoid eating `-p` with a following positional.
	{regexp.MustCompile(`(\s|^)-p([^\s=]{4,})`), "$1-p[REDACTED]"},

	// --password=..., -password=...
	{regexp.MustCompile(`(?i)--?pass(word)?\s*=\s*\S+`), "--password=[REDACTED]"},

	// Generic key=value where the key name screams secret.
	// Match common connectors (`=`, `:`, `: `). Case-insensitive on the
	// key name. Captures the whole key so the output preserves it.
	{regexp.MustCompile(`(?i)(\b[A-Z0-9_]*(TOKEN|SECRET|APIKEY|API_KEY|ACCESS_KEY|PRIVATE_KEY|PASSWORD|PASSWD|PASSPHRASE|CREDENTIAL|CREDENTIALS)[A-Z0-9_]*)\s*[:=]\s*\S+`),
		"$1=[REDACTED]"},

	// Shadow/password file hash lines: `user:$6$salt$hash:...`.
	{regexp.MustCompile(`(^|\n)([a-z_][a-z0-9_-]*):\$[1-9][aby]?\$[^:\s]+`),
		"$1$2:[REDACTED-HASH]"},
}

// Redact applies every built-in pattern to b and returns the scrubbed
// bytes. b itself is never modified (a fresh slice is returned).
func Redact(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	out := b
	for _, p := range patterns {
		out = p.re.ReplaceAll(out, []byte(p.sub))
	}
	return out
}

// ansiRegex matches terminal escape sequences that a pager / cat would
// interpret as commands. Four families plus BEL:
//   - CSI:     ESC [ params intermediates final   (cursor, color, clear)
//   - OSC:     ESC ] params (BEL | ESC \)         (window title, hyperlinks)
//   - short:   ESC intermediates final            (VT100 Fp/Fe/nF — covers
//     single-byte C1, DECKPAM,
//     DECKPNM, charset select)
//   - BEL:     \x07
//
// CSI and OSC are tried first so legitimate complete sequences match the
// more specific branch; the short form is the fallback for anything else
// starting with ESC. Not exhaustive against every obscure private-mode
// sequence, but covers everything a command typically emits.
var ansiRegex = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[ -/]*[0-~])|\x07`)

// StripTerminalEscapes removes ANSI escape sequences and BEL from b.
// Intended for audit output before storage so `zcat audit-N.txt.gz`
// on an operator's terminal can't clear the screen, move the cursor,
// or retitle the window. Returns a new slice; b is not modified.
func StripTerminalEscapes(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	return ansiRegex.ReplaceAll(b, nil)
}

// StripTerminalEscapesString is the string convenience wrapper.
func StripTerminalEscapesString(s string) string {
	if s == "" {
		return s
	}
	return string(StripTerminalEscapes([]byte(s)))
}

// RedactString is a convenience wrapper.
func RedactString(s string) string {
	if s == "" {
		return s
	}
	return string(Redact([]byte(s)))
}
