package stream

import (
	"strings"
	"testing"
)

func TestEffectiveHTMLLen(t *testing.T) {
	cases := map[string]int{
		"":         0,
		"hello":    5,
		"<":        4,  // &lt;
		">":        4,  // &gt;
		"&":        5,  // &amp;
		"a<b>":     10, // 1 + 4 + 1 + 4
		"if (a&b)": len("if (a") + 5 + len("b)"),
	}
	for in, want := range cases {
		if got := effectiveHTMLLen([]byte(in)); got != want {
			t.Errorf("effectiveHTMLLen(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestMaxPrefixFittingHonorsEscape(t *testing.T) {
	// "<" costs 4. With cap 4, we can fit exactly one "<".
	if got := maxPrefixFitting([]byte("<<<"), 4); got != 1 {
		t.Fatalf("maxPrefixFitting(<<<, 4) = %d, want 1", got)
	}
	// cap 8 fits two <s (4+4=8) but a third would overflow.
	if got := maxPrefixFitting([]byte("<<<"), 8); got != 2 {
		t.Fatalf("maxPrefixFitting(<<<, 8) = %d, want 2", got)
	}
	// ASCII counts as 1 each.
	if got := maxPrefixFitting([]byte("abcdef"), 4); got != 4 {
		t.Fatalf("maxPrefixFitting(abcdef, 4) = %d, want 4", got)
	}
	// Full fit → returns len(b).
	if got := maxPrefixFitting([]byte("abc"), 100); got != 3 {
		t.Fatalf("maxPrefixFitting(abc, 100) = %d, want 3", got)
	}
}

func TestPickBreakNewlinePreferred(t *testing.T) {
	// Line boundary exists before the fit limit — prefer it.
	body := []byte("line one\nline two\nline three")
	// cap large enough to contain the first two lines, but not a 3rd.
	cut := pickBreak(body, 20)
	if cut == 0 || string(body[:cut]) != "line one\nline two\n" {
		t.Fatalf("pickBreak cut at %d: %q", cut, string(body[:cut]))
	}
}

func TestPickBreakSpaceFallback(t *testing.T) {
	// No newlines; break on last space before cap.
	body := []byte("a b c d e f g h i j k l m n o")
	cut := pickBreak(body, 10)
	chunk := string(body[:cut])
	if !strings.HasSuffix(chunk, " ") {
		t.Fatalf("expected space-terminated chunk, got %q", chunk)
	}
	if len(chunk) > 10 {
		t.Fatalf("chunk exceeds cap: %q", chunk)
	}
}

func TestPickBreakHardCut(t *testing.T) {
	// No newline or space anywhere. Hard cut at the fit limit.
	body := []byte("abcdefghijklmnopqrstuvwxyz")
	cut := pickBreak(body, 5)
	if cut != 5 {
		t.Fatalf("hard-cut expected 5, got %d", cut)
	}
}

func TestPickBreakHonorsHTMLEscape(t *testing.T) {
	// "<<<<<" — each char is 4 escape chars. Cap 8 → fits 2 raw bytes.
	body := []byte("<<<<<")
	cut := pickBreak(body, 8)
	if cut != 2 {
		t.Fatalf("expected cut=2 (8 cap / 4 per char), got %d", cut)
	}
}

func TestRenderBodyEmpty(t *testing.T) {
	got := renderBody(nil, "")
	if got != "⏳ running…" {
		t.Fatalf("empty body/trailer = %q", got)
	}
}

func TestRenderBodyEscapesBodyNotTrailer(t *testing.T) {
	body := []byte("hello <world>")
	trailer := "\n<i>…</i>"
	got := renderBody(body, trailer)
	want := "<pre>hello &lt;world&gt;</pre>\n<i>…</i>"
	if got != want {
		t.Fatalf("renderBody:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestFinalFooter(t *testing.T) {
	cases := []struct {
		exit int
		want string
	}{
		{0, "✅ exit 0"},
		{1, "❌ exit 1"},
		{137, "❌ exit 137"},
		{-1, "⚠ shell died"},
	}
	for _, c := range cases {
		got := finalFooter(c.exit, 0)
		if !strings.HasPrefix(got, c.want) {
			t.Errorf("exit=%d: got %q, want prefix %q", c.exit, got, c.want)
		}
	}
}
