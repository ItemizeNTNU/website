package web

// Package-internal tests for the flash cookie: the SetFlash/takeFlash round
// trip, the parse branches that discard a mangled cookie, and the ClearFlash
// middleware that expires whatever was displayed.

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// flashRequest builds a request carrying the cookies a previous response set,
// the way a browser would replay them on the redirect that follows a form
// post.
func flashRequest(t *testing.T, rec *httptest.ResponseRecorder) *http.Request {
	t.Helper()
	r := httptest.NewRequest("GET", "/", nil)
	for _, c := range rec.Result().Cookies() {
		r.AddCookie(c)
	}
	return r
}

func TestFlashRoundTrip(t *testing.T) {
	rec := httptest.NewRecorder()
	// The colon matters: the cookie format is kind:text, and the first cut
	// must win so a colon inside the message is not treated as a delimiter.
	SetFlash(rec, "success", "Lagret: alt vel")

	got := takeFlash(flashRequest(t, rec))
	if len(got) != 1 {
		t.Fatalf("takeFlash returned %d toasts, want 1 — the visitor would never see the confirmation for the form they just submitted", len(got))
	}
	if got[0].Kind != "success" || got[0].Text != "Lagret: alt vel" {
		t.Errorf("takeFlash = %+v, want success/\"Lagret: alt vel\" — a colon in the message text must survive the round trip, or half the confirmation goes missing", got[0])
	}
}

func TestTakeFlashParseBranches(t *testing.T) {
	t.Run("no cookie", func(t *testing.T) {
		if got := takeFlash(httptest.NewRequest("GET", "/", nil)); got != nil {
			t.Errorf("takeFlash with no cookie = %+v, want nil — every anonymous page load would show a phantom toast", got)
		}
	})

	// The remaining branches feed a raw cookie value straight in, the way a
	// visitor hand-editing their cookies could.
	tests := []struct {
		name  string
		value string
		want  *Toast // nil means no toast expected
	}{
		{"bad escape sequence", "%zz", nil},
		{"no colon", "nocolon", nil},
		{"empty text", "success:", nil},
		{"unknown kind coerced to info", "weird:msg", &Toast{Kind: "info", Text: "msg"}},
		{"success kept", "success:ok", &Toast{Kind: "success", Text: "ok"}},
		{"error kept", "error:ok", &Toast{Kind: "error", Text: "ok"}},
		{"info kept", "info:ok", &Toast{Kind: "info", Text: "ok"}},
		{"warning kept", "warning:ok", &Toast{Kind: "warning", Text: "ok"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.AddCookie(&http.Cookie{Name: flashCookie, Value: tt.value})

			got := takeFlash(r)
			if tt.want == nil {
				if got != nil {
					t.Errorf("takeFlash(%q) = %+v, want nil — a mangled cookie should be dropped silently, not rendered as a toast", tt.value, got)
				}
				return
			}
			if len(got) != 1 || got[0] != *tt.want {
				t.Errorf("takeFlash(%q) = %+v, want [%+v] — the toast styling keys off Kind, so an unrecognised kind must degrade to info rather than an unstyled box", tt.value, got, *tt.want)
			}
		})
	}
}

func TestSetFlashCookieAttributes(t *testing.T) {
	rec := httptest.NewRecorder()
	SetFlash(rec, "info", "hei")

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("SetFlash set %d cookies, want 1", len(cookies))
	}
	c := cookies[0]
	if c.Path != "/" {
		t.Errorf("flash cookie Path = %q, want \"/\" — a message set on one path would never be visible on the page the redirect lands on", c.Path)
	}
	if c.MaxAge != 60 {
		t.Errorf("flash cookie MaxAge = %d, want 60 — without a short lifetime a never-displayed message would linger and pop up days later", c.MaxAge)
	}
	if !c.HttpOnly {
		t.Error("flash cookie is not HttpOnly — page script has no business reading it")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("flash cookie SameSite = %v, want Lax — the cookie must survive the top-level redirect after a form post", c.SameSite)
	}
}

func TestClearFlash(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	t.Run("cookie present is expired", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/", nil)
		r.AddCookie(&http.Cookie{Name: flashCookie, Value: "info:hei"})
		rec := httptest.NewRecorder()
		ClearFlash(next).ServeHTTP(rec, r)

		var cleared bool
		for _, c := range rec.Result().Cookies() {
			if c.Name == flashCookie {
				cleared = true
				if c.MaxAge >= 0 {
					t.Errorf("ClearFlash set MaxAge %d, want negative — the cookie would survive and the same toast would reappear on every page until it expired", c.MaxAge)
				}
			}
		}
		if !cleared {
			t.Error("ClearFlash did not expire the flash cookie — the visitor would see the same toast on every page load for the next minute")
		}
	})

	t.Run("no cookie means no Set-Cookie", func(t *testing.T) {
		rec := httptest.NewRecorder()
		ClearFlash(next).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

		for _, c := range rec.Result().Cookies() {
			if c.Name == flashCookie {
				t.Errorf("ClearFlash set a flash cookie on a request that carried none: %+v — every response would carry a pointless Set-Cookie header", c)
			}
		}
	})
}
