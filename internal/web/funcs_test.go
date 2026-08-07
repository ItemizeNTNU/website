package web

// Package-internal tests for the template function map and the email
// obfuscation helpers.

import (
	"slices"
	"strings"
	"testing"
)

// fakeResolver is the minimal AssetResolver the tests need. Defined here and
// shared with render_test.go, which is in the same package.
type fakeResolver struct{}

func (fakeResolver) URL(name string) string { return "/static/" + name }
func (fakeResolver) Has(name string) bool   { return true }
func (fakeResolver) Refresh() error         { return nil }

var _ AssetResolver = fakeResolver{}

func TestEncodeEmailIsAnInvolution(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{"the board address", "styret@itemize.no"},
		{"mixed case", "Styret@Itemize.NO"},
		{"digits pass through", "abc123@x2.no"},
		{"non-ASCII letters untouched", "æøå@x.no"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EncodeEmail(EncodeEmail(tt.addr)); got != tt.addr {
				t.Errorf("EncodeEmail(EncodeEmail(%q)) = %q — app.js applies the same rotation in the browser, so a non-involutive encoding means every mailto link on the site opens with a garbled address", tt.addr, got)
			}
		})
	}
}

func TestEncodeEmailHidesThePlainAddress(t *testing.T) {
	got := EncodeEmail("styret@itemize.no")
	if strings.Contains(got, "itemize") {
		t.Errorf("EncodeEmail(\"styret@itemize.no\") = %q still contains \"itemize\" — the served HTML would match exactly the pattern an address harvester greps for, defeating the encoding's whole purpose", got)
	}
}

func TestObfuscateForHumans(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"simple address", "a@b.no", "a [at] b [dot] no"},
		{"every domain dot replaced", "x@a.b.no", "x [at] a [dot] b [dot] no"},
		{"local-part dots untouched", "first.last@x.no", "first.last [at] x [dot] no"},
		{"no @ passes through", "not-an-address", "not-an-address"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := obfuscateForHumans(tt.in); got != tt.want {
				t.Errorf("obfuscateForHumans(%q) = %q, want %q — the <noscript> fallback would either be unreadable to a person or match a harvester's user@host regexp", tt.in, got, tt.want)
			}
		})
	}
}

func TestDict(t *testing.T) {
	t.Run("even pairs build a map", func(t *testing.T) {
		m, err := dict("a", 1, "b", "two")
		if err != nil {
			t.Fatalf("dict on well-formed pairs returned %v — every partial invoked with arguments would fail to render", err)
		}
		if len(m) != 2 || m["a"] != 1 || m["b"] != "two" {
			t.Errorf("dict(\"a\", 1, \"b\", \"two\") = %v — a partial would see the wrong values for its named arguments", m)
		}
	})

	t.Run("odd argument count is an error", func(t *testing.T) {
		if _, err := dict("a", 1, "dangling"); err == nil {
			t.Error("dict with a dangling key returned no error — a template typo would silently drop or misalign a partial's arguments instead of failing loudly at render time")
		}
	})

	t.Run("non-string key is an error", func(t *testing.T) {
		if _, err := dict(42, "value"); err == nil {
			t.Error("dict with a non-string key returned no error — the mistake would surface later as a missing map entry rather than at the call site")
		}
	})

	t.Run("empty call is an empty map", func(t *testing.T) {
		m, err := dict()
		if err != nil {
			t.Fatalf("dict() returned %v — a partial called without arguments would fail to render", err)
		}
		if len(m) != 0 {
			t.Errorf("dict() = %v, want an empty map", m)
		}
	})
}

func TestHasRole(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
		want  string
		out   bool
	}{
		{"role present", []string{"Medlem", "Styret"}, "Styret", true},
		{"role absent", []string{"Medlem"}, "Styret", false},
		{"empty slice", nil, "Styret", false},
		{"empty want never matches", []string{"Medlem"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasRole(tt.roles, tt.want); got != tt.out {
				t.Errorf("hasRole(%v, %q) = %v, want %v — a template would show or hide board-only controls for the wrong visitors", tt.roles, tt.want, got, tt.out)
			}
		})
	}
}

// TestFuncsKeySet pins the function map to exactly the names the templates
// call (verified by grepping assets/templates/**) plus hasRole. A key removed
// here does not fail compilation — it fails at template parse time, taking
// every page down at startup.
func TestFuncsKeySet(t *testing.T) {
	want := []string{
		"asset", "csrf", "dict", "eml", "emlfallback",
		"hasAsset", "hasRole", "list", "smartTime",
	}

	funcs := Funcs(fakeResolver{})
	got := make([]string, 0, len(funcs))
	for name := range funcs {
		got = append(got, name)
	}
	slices.Sort(got)

	if !slices.Equal(got, want) {
		t.Errorf("Funcs keys = %v, want %v — a template calling a missing function fails to parse, which takes every page on the site down at startup", got, want)
	}
}
