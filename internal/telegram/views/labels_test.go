package views

import "testing"

func TestSanitizeDisplayName_CleanUnchanged(t *testing.T) {
	cases := []string{
		"Alice",
		"Alice Smith",
		"Zoë Müller", // Unicode letters survive
		"O'Brien",    // apostrophe survives (not bidi / control)
		"Anne-Marie", // hyphen survives
		"",           // empty → empty
		"a",
	}
	for _, in := range cases {
		got := SanitizeDisplayName(in)
		if got != in {
			t.Errorf("%q mutated to %q", in, got)
		}
	}
}

func TestSanitizeDisplayName_CollapsesWhitespace(t *testing.T) {
	cases := map[string]string{
		"Alice  Smith":      "Alice Smith",
		"Alice   \t  Smith": "Alice Smith",
		"Alice\nSmith":      "Alice Smith",
		"Alice\rSmith":      "Alice Smith",
		"Alice\tSmith":      "Alice Smith",
		"  leading":         "leading",
		"trailing  ":        "trailing",
		"\n\tBob\n":         "Bob",
	}
	for in, want := range cases {
		if got := SanitizeDisplayName(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeDisplayName_StripsZeroWidth(t *testing.T) {
	cases := map[string]string{
		"Bob\u200Bname": "Bobname", // ZWSP
		"\uFEFFstart":   "start",   // BOM
		"Al\u200Dice":   "Alice",   // ZWJ
		"X\u2060Y":      "XY",      // WJ
	}
	for in, want := range cases {
		if got := SanitizeDisplayName(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeDisplayName_StripsBidiControls(t *testing.T) {
	// RTL override chars render as direction flips — strip them so
	// admin-typed names can't secretly reverse rendering in super's
	// notification DMs.
	cases := map[string]string{
		"Admin\u202Ecilar":        "Admincilar", // RLO
		"\u202Amixed":             "mixed",
		"\u2066isolate\u2069tail": "isolatetail",
	}
	for in, want := range cases {
		if got := SanitizeDisplayName(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeDisplayName_StripsControlChars(t *testing.T) {
	cases := map[string]string{
		"Bob\x00name":   "Bobname", // NUL
		"Bob\x07name":   "Bobname", // BEL
		"Bob\x1bname":   "Bobname", // ESC
		"Bob\x7fname":   "Bobname", // DEL
		"Bob\u0085name": "Bobname", // NEL (C1)
	}
	for in, want := range cases {
		if got := SanitizeDisplayName(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}
