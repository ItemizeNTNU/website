package web

// Package-internal tests for the small pure helpers scattered across the
// handlers: shellPath, NavIndex, attendanceName, parseID, trimFloat and the
// two eventForm constructors. They are unexported (or operate on unexported
// types), so this file is `package web` rather than web_test — Go permits
// both in one directory, and the HTTP-level tests stay external.

import (
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/events"
	"github.com/ItemizeNTNU/website/internal/timefmt"
)

func TestShellPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"root is home", "/", "~"},
		{"empty path is home", "", "~"},
		{"single segment", "/arrangementer", "~/arrangementer"},
		{"trailing slash trimmed", "/a/b/", "~/a/b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shellPath(tt.in); got != tt.want {
				t.Errorf("shellPath(%q) = %q, want %q — the status line would show the wrong working directory for this page", tt.in, got, tt.want)
			}
		})
	}
}

func TestNavIndex(t *testing.T) {
	// Every registered nav key must resolve to its own position, or the tab
	// strip's previous/next arithmetic walks to the wrong tab.
	for i, item := range NavItems {
		t.Run("key "+item.Key, func(t *testing.T) {
			if got := NavIndex(item.Key); got != i {
				t.Errorf("NavIndex(%q) = %d, want %d — the tab strip would highlight or step to the wrong tab", item.Key, got, i)
			}
		})
	}

	for _, key := range []string{"profil", ""} {
		t.Run("unknown key "+key, func(t *testing.T) {
			if got := NavIndex(key); got != -1 {
				t.Errorf("NavIndex(%q) = %d, want -1 — a page outside the nav would be treated as a tab", key, got)
			}
		})
	}
}

// TestNavItemsHrefsMatchKeys guards the invariant NavItems' doc comment
// promises: each tab's Href is derived from its Key, so a tab cannot silently
// point at a path no handler serves.
func TestNavItemsHrefsMatchKeys(t *testing.T) {
	for _, item := range NavItems {
		t.Run(item.Key, func(t *testing.T) {
			want := "/" + item.Key
			if item.Key == "index" {
				want = "/"
			}
			if item.Href != want {
				t.Errorf("nav entry %q links to %q, want %q — a visitor clicking that tab would land on a 404", item.Key, item.Href, want)
			}
		})
	}
}

func TestAttendanceName(t *testing.T) {
	tests := []struct {
		name string
		user *auth.User
		want string
	}{
		{"nil user", nil, ""},
		{
			"full name wins over everything",
			&auth.User{ID: "fa-1", Name: "kari", FullName: "Kari Nordmann", Email: "kari@example.no"},
			"Kari Nordmann",
		},
		{
			"falls back to the display name",
			&auth.User{ID: "fa-1", Name: "kari", Email: "kari@example.no"},
			"kari",
		},
		{
			"falls back to the email",
			&auth.User{ID: "fa-1", Email: "kari@example.no"},
			"kari@example.no",
		},
		{
			"falls back to the bare ID",
			&auth.User{ID: "fa-1"},
			"fa-1",
		},
		{"every field empty", &auth.User{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := attendanceName(tt.user); got != tt.want {
				t.Errorf("attendanceName = %q, want %q — a blank name here is a blank row on the attendance list, which is exactly the failure the old fullName-only code produced", got, tt.want)
			}
		})
	}
}

func TestParseID(t *testing.T) {
	t.Run("rejects malformed identifiers", func(t *testing.T) {
		bad := []struct {
			name string
			in   string
		}{
			{"empty", ""},
			{"not hex", "not-hexnot-hexnot-hexnot"},
			{"23 chars", "0123456789abcdef0123456"},
			{"25 chars", "0123456789abcdef012345678"},
		}
		for _, tt := range bad {
			t.Run(tt.name, func(t *testing.T) {
				if _, ok := parseID(tt.in); ok {
					t.Errorf("parseID(%q) accepted a malformed identifier — a probe could then distinguish bad-format from not-found responses", tt.in)
				}
			})
		}
	})

	t.Run("accepts and round-trips a valid identifier", func(t *testing.T) {
		const hex = "0123456789abcdef01234567"
		id, ok := parseID(hex)
		if !ok {
			t.Fatalf("parseID(%q) rejected a valid 24-char hex identifier — no event could ever be edited or deleted", hex)
		}
		if got := id.Hex(); got != hex {
			t.Errorf("parseID(%q).Hex() = %q — the identifier does not round-trip, so a form or URL built from it would name a different document", hex, got)
		}
	})
}

func TestTrimFloat(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{2, "2"},
		{2.5, "2.5"},
		{0, "0"},
		{0.25, "0.25"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := trimFloat(tt.in); got != tt.want {
				t.Errorf("trimFloat(%v) = %q, want %q — the admin form would show a duration like %q, which reads as a different number to the board member editing it", tt.in, got, tt.want, got)
			}
		})
	}
}

func TestFormFromEvent(t *testing.T) {
	id, ok := parseID("0123456789abcdef01234567")
	if !ok {
		t.Fatal("could not build a fixture ObjectID")
	}
	e := &events.Event{
		ID:          id,
		Name:        "CTF-kveld",
		Location:    events.Place{Name: "R2", URL: "https://use.mazemap.com/r2"},
		RegisterURL: "https://example.no/pamelding",
		Date:        time.Date(2026, 9, 1, 17, 15, 0, 0, timefmt.Oslo),
		Duration:    2.5,
		CTF:         events.Place{Name: "PicoCTF", URL: "https://picoctf.org"},
		Info:        "Ta med laptop.",
		Hidden:      true,
		Discord:     true,
	}

	got := formFromEvent(e)
	want := eventForm{
		ID:           "0123456789abcdef01234567",
		Name:         "CTF-kveld",
		LocationName: "R2",
		LocationURL:  "https://use.mazemap.com/r2",
		RegisterURL:  "https://example.no/pamelding",
		Date:         "2026-09-01T17:15",
		Duration:     "2.5",
		CTFName:      "PicoCTF",
		CTFURL:       "https://picoctf.org",
		Info:         "Ta med laptop.",
		Hidden:       true,
		Discord:      true,
	}
	if got != want {
		t.Errorf("formFromEvent = %+v, want %+v — a field dropped here silently disappears from the event the next time the board saves the edit form", got, want)
	}
}

func TestFormFromSubmission(t *testing.T) {
	tests := []struct {
		name    string
		hidden  []string // nil means the field is absent entirely
		discord []string
		wantH   bool
		wantD   bool
	}{
		{"absent checkboxes are false", nil, nil, false, false},
		{"empty values are false", []string{""}, []string{""}, false, false},
		{"value 1 is true", []string{"1"}, []string{"1"}, true, true},
		{"value on is true", []string{"on"}, []string{"on"}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/arrangementer", nil)
			// In production auth.CSRF has already called ParseForm, so the
			// handler reads r.PostForm directly; setting it here mirrors that.
			r.PostForm = url.Values{
				"id":            {"0123456789abcdef01234567"},
				"name":          {"CTF-kveld"},
				"location.name": {"R2"},
				"location.url":  {"https://use.mazemap.com/r2"},
				"register_url":  {"https://example.no/pamelding"},
				"date":          {"2026-09-01T17:15"},
				"duration":      {"2.5"},
				"ctf.name":      {"PicoCTF"},
				"ctf.url":       {"https://picoctf.org"},
				"info":          {"Ta med laptop."},
			}
			if tt.hidden != nil {
				r.PostForm["hidden"] = tt.hidden
			}
			if tt.discord != nil {
				r.PostForm["discord"] = tt.discord
			}

			got := formFromSubmission(r)
			want := eventForm{
				ID:           "0123456789abcdef01234567",
				Name:         "CTF-kveld",
				LocationName: "R2",
				LocationURL:  "https://use.mazemap.com/r2",
				RegisterURL:  "https://example.no/pamelding",
				Date:         "2026-09-01T17:15",
				Duration:     "2.5",
				CTFName:      "PicoCTF",
				CTFURL:       "https://picoctf.org",
				Info:         "Ta med laptop.",
				Hidden:       tt.wantH,
				Discord:      tt.wantD,
			}
			if got != want {
				t.Errorf("formFromSubmission = %+v, want %+v — a rejected submission would come back to the board member with their typed values missing or their checkboxes flipped", got, want)
			}
		})
	}
}
