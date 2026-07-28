package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func post(token, field, fetchSite string) *http.Request {
	body := url.Values{}
	if field != "" {
		body.Set(CSRFField, field)
	}
	r := httptest.NewRequest(http.MethodPost, "/arrangementer", strings.NewReader(body.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if token != "" {
		r.AddCookie(&http.Cookie{Name: csrfCookie, Value: token})
	}
	if fetchSite != "" {
		r.Header.Set("Sec-Fetch-Site", fetchSite)
	}
	return r
}

func TestCSRF(t *testing.T) {
	tests := []struct {
		name                    string
		token, field, fetchSite string
		want                    int
	}{
		{"matching token", "abc", "abc", "same-origin", http.StatusOK},
		{"no Sec-Fetch-Site header (older browser)", "abc", "abc", "", http.StatusOK},
		// The attack this exists to stop: a form on another site posting here
		// with the visitor's cookies attached.
		{"cross-site", "abc", "abc", "cross-site", http.StatusForbidden},
		{"token missing from the body", "abc", "", "same-origin", http.StatusForbidden},
		{"token missing from the cookie", "", "abc", "same-origin", http.StatusForbidden},
		{"tokens disagree", "abc", "xyz", "same-origin", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			CSRF(okHandler()).ServeHTTP(rec, post(tt.token, tt.field, tt.fetchSite))
			if rec.Code != tt.want {
				t.Errorf("got %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

// Reads must never be blocked; only state changes are checked.
func TestCSRFIgnoresSafeMethods(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		rec := httptest.NewRecorder()
		CSRF(okHandler()).ServeHTTP(rec, httptest.NewRequest(method, "/", nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s was blocked", method)
		}
	}
}

func TestCSRFTokenIsStableWithinASession(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	first := CSRFToken(httptest.NewRecorder(), r)
	if first == "" {
		t.Fatal("no token issued")
	}

	// A request that already carries the cookie keeps the same value, so two
	// forms rendered on one page do not disagree.
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: first})
	if second := CSRFToken(httptest.NewRecorder(), r); second != first {
		t.Errorf("token changed: %q then %q", first, second)
	}
}
