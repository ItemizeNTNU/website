package auth

// Inject clears a session cookie it cannot open (middleware.go:19). The login
// callback sets a new one. Both run on the same response for the same cookie
// name, because Inject wraps the whole mux — the callback route included
// (cmd/website/main.go:146-158). These tests are about that overlap.
//
// The case is not hypothetical: it is exactly what every signed-in member hits
// the first time they log in after the session key is rotated. Their old cookie
// no longer opens, so Inject clears it on the very request that is trying to
// establish the new session. If the clearing were to win, the rotation would
// lock out every member until they cleared their own cookies by hand — with no
// error anywhere, because each half is behaving as designed.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// injectThenCallback runs the callback through Inject, the way the real chain
// does, rather than calling Callback on its own.
func injectThenCallback(t *testing.T, a *Authenticator, flow *http.Cookie,
	query url.Values, stale string) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequest(http.MethodGet, "/callback?"+query.Encode(), nil)
	if flow != nil {
		r.AddCookie(flow)
	}
	if stale != "" {
		r.AddCookie(&http.Cookie{Name: SessionCookie, Value: stale})
	}

	rec := httptest.NewRecorder()
	a.Inject(http.HandlerFunc(a.Callback)).ServeHTTP(rec, r)
	return rec
}

// A member whose old cookie cannot be opened must still be able to log in. The
// response carries two Set-Cookie headers for the same name — Inject's clear
// and the callback's new session — and the browser keeps the last one.
func TestLoginSucceedsDespiteAStaleSessionCookie(t *testing.T) {
	idp := newFakeIDP(t)
	a, sealer := newAuthenticator(t, idp, "HS256", "")

	authURL, flow := startLogin(t, a, "/profil")
	idp.idToken = signClaims(t, testClientSecret, idp.claimsFor(authURL.Query().Get("nonce")))

	// Not openable with the current key: what a cookie sealed with the previous
	// key looks like after a rotation.
	rec := injectThenCallback(t, a, flow, url.Values{
		"state": {authURL.Query().Get("state")},
		"code":  {"the-authorization-code"},
	}, "not-a-cookie-this-key-can-open")

	if rec.Code != http.StatusFound {
		t.Fatalf("the callback returned %d rather than a redirect: %s", rec.Code, rec.Body.String())
	}

	sess := sessionFrom(t, sealer, rec)
	if sess == nil {
		t.Fatal("no readable session survived the response, so a member logging in after a " +
			"key rotation would be bounced straight back to the login page — and would stay " +
			"stuck there, because every attempt carries the same stale cookie")
	}
	if sess.ID != "11111111-2222-4333-8444-999999999999" {
		t.Errorf("the established session is not the one the provider just vouched for: %q", sess.ID)
	}
}

// The order matters, not just the presence of both headers. A browser applies
// Set-Cookie in the order it receives them, so the clear has to come first; if
// a refactor ever moved Inject's write after the handler's, the member would be
// logged straight back out and the test above would still pass whenever the
// recorder happened to hand back the working cookie.
func TestTheNewSessionCookieIsWrittenAfterTheClear(t *testing.T) {
	idp := newFakeIDP(t)
	a, _ := newAuthenticator(t, idp, "HS256", "")

	authURL, flow := startLogin(t, a, "")
	idp.idToken = signClaims(t, testClientSecret, idp.claimsFor(authURL.Query().Get("nonce")))

	rec := injectThenCallback(t, a, flow, url.Values{
		"state": {authURL.Query().Get("state")},
		"code":  {"the-authorization-code"},
	}, "not-a-cookie-this-key-can-open")

	var cleared, established int
	for i, c := range rec.Result().Cookies() {
		if c.Name != SessionCookie {
			continue
		}
		if c.Value == "" {
			cleared = i + 1
			continue
		}
		established = i + 1
	}

	if cleared == 0 {
		t.Fatal("Inject did not clear the unopenable cookie, so it would be re-sent on every " +
			"later request for as long as the browser kept it")
	}
	if established == 0 {
		t.Fatal("the callback established no session cookie")
	}
	if cleared > established {
		t.Errorf("the clearing Set-Cookie is emitted after the new session (positions %d and %d); "+
			"a browser applies them in order, so the member would be logged out by the very "+
			"response that logged them in", cleared, established)
	}
}

// The clear must not fire for a visitor who sent no cookie at all: a first-time
// member logging in should receive exactly one session cookie, not a clear they
// never needed followed by the real one.
func TestAFirstLoginSetsOnlyOneSessionCookie(t *testing.T) {
	idp := newFakeIDP(t)
	a, _ := newAuthenticator(t, idp, "HS256", "")

	authURL, flow := startLogin(t, a, "")
	idp.idToken = signClaims(t, testClientSecret, idp.claimsFor(authURL.Query().Get("nonce")))

	rec := injectThenCallback(t, a, flow, url.Values{
		"state": {authURL.Query().Get("state")},
		"code":  {"the-authorization-code"},
	}, "")

	var n int
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookie {
			n++
		}
	}
	if n != 1 {
		t.Errorf("a first-time login wrote %d %s cookies, want 1 — Inject is clearing a cookie "+
			"the visitor never sent", n, SessionCookie)
	}
}
