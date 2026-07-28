package ical

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestEscape(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{"a,b", `a\,b`},
		{"a;b", `a\;b`},
		{"line\nbreak", `line\nbreak`},
		{"crlf\r\nbreak", `crlf\nbreak`},
		// The backslash must be doubled before the other escapes are inserted,
		// or those escapes would themselves be escaped.
		{`back\slash`, `back\\slash`},
		{`\,`, `\\\,`},
	}
	for _, tt := range tests {
		if got := escape(tt.in); got != tt.want {
			t.Errorf("escape(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFoldRespectsOctetLimit(t *testing.T) {
	line := "DESCRIPTION:" + strings.Repeat("a", 300)
	folded := fold(line)

	if !strings.HasSuffix(folded, "\r\n") {
		t.Fatal("folded output must end with CRLF")
	}
	for i, l := range strings.Split(strings.TrimSuffix(folded, "\r\n"), "\r\n") {
		if len(l) > 75 {
			t.Errorf("line %d is %d octets, over the 75 limit: %q", i, len(l), l)
		}
		if i > 0 && !strings.HasPrefix(l, " ") {
			t.Errorf("continuation line %d must begin with a space: %q", i, l)
		}
	}
}

// A split in the middle of a multi-byte character produces a feed that some
// clients reject and others render as mojibake. Norwegian event titles make
// this a live concern, not a theoretical one.
func TestFoldNeverSplitsARune(t *testing.T) {
	line := "SUMMARY:" + strings.Repeat("æøå", 60)
	folded := fold(line)

	for _, l := range strings.Split(strings.TrimSuffix(folded, "\r\n"), "\r\n") {
		if !utf8.ValidString(strings.TrimPrefix(l, " ")) {
			t.Fatalf("fold produced invalid UTF-8: %q", l)
		}
	}

	// Unfolding must reproduce the original exactly.
	var rebuilt strings.Builder
	for i, l := range strings.Split(strings.TrimSuffix(folded, "\r\n"), "\r\n") {
		if i > 0 {
			l = strings.TrimPrefix(l, " ")
		}
		rebuilt.WriteString(l)
	}
	if rebuilt.String() != line {
		t.Errorf("unfolding did not round-trip:\n got %q\nwant %q", rebuilt.String(), line)
	}
}

func TestFoldShortLineIsUntouched(t *testing.T) {
	if got, want := fold("VERSION:2.0"), "VERSION:2.0\r\n"; got != want {
		t.Errorf("fold() = %q, want %q", got, want)
	}
}

func TestCalendarStructure(t *testing.T) {
	start := time.Date(2026, 2, 1, 17, 15, 0, 0, time.UTC)
	cal := New("Itemize Arrangementer")
	cal.Add(Event{
		UID:         UID("507f1f77bcf86cd799439011"),
		Start:       start,
		End:         start.Add(3 * time.Hour),
		Stamp:       start,
		Summary:     "Pizza, øl og CTF",
		Description: "Ta med laptop;\nvi starter 17:15",
		Location:    "Savannen, IIK",
		URL:         "https://itemize.no/arrangementer",
	})
	out := cal.String()

	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n",
		"VERSION:2.0\r\n",
		"BEGIN:VEVENT\r\n",
		"UID:507f1f77bcf86cd799439011@itemize.no\r\n",
		"DTSTART:20260201T171500Z\r\n",
		"DTEND:20260201T201500Z\r\n",
		"END:VEVENT\r\n",
		"END:VCALENDAR\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("feed is missing %q\n---\n%s", want, out)
		}
	}

	// Semicolons and newlines in a description are structural and must be
	// escaped; a URL is not TEXT and must not be.
	if !strings.Contains(out, `Ta med laptop\;\nvi starter 17:15`) {
		t.Errorf("description was not escaped:\n%s", out)
	}
	if !strings.Contains(out, "URL:https://itemize.no/arrangementer") {
		t.Errorf("URL should not be escaped:\n%s", out)
	}

	// Every line in the feed, not just the ones we fold explicitly.
	for _, l := range strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n") {
		if len(l) > 75 {
			t.Errorf("line over 75 octets: %q", l)
		}
	}
}
