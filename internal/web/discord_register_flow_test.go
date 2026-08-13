package web_test

// The registration-time Discord flow: someone who just signed up holds a
// sealed cookie instead of a session, and both the start route and the
// callback must treat that cookie as the only thing that vouches for them.
// The logged-in profile flow's tests live in discord_flow_test.go and are
// deliberately untouched — passing unmodified is what proves that flow kept
// its exact behaviour.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/users"
)

// fakeDiscordLinker satisfies web.DiscordLinker and records what Complete was
// asked to do. The concrete users.DiscordService offers no seam past the
// state check — Complete talks to the compile-time discord.APIBase — so the
// callback's success path can only be exercised through this fake.
type fakeDiscordLinker struct {
	available   bool
	completeErr error
	link        *users.Link
	completed   []completedCall
}

type completedCall struct{ userID, code, redirectURI string }

func (f *fakeDiscordLinker) Available() bool { return f.available }

func (f *fakeDiscordLinker) AuthorizeURL(state, redirectURI string) (string, error) {
	if !f.available {
		return "", users.ErrUnavailable
	}
	// The same endpoint the real client points at, so the tests assert the
	// same shape of redirect either way.
	return "https://discord.com/oauth2/authorize?state=" + url.QueryEscape(state) +
		"&redirect_uri=" + url.QueryEscape(redirectURI), nil
}

func (f *fakeDiscordLinker) Complete(_ context.Context, userID, code, redirectURI string) (*users.Link, error) {
	f.completed = append(f.completed, completedCall{userID, code, redirectURI})
	if f.completeErr != nil {
		return nil, f.completeErr
	}
	if f.link != nil {
		return f.link, nil
	}
	return &users.Link{ID: "d-1", Username: "kari", IsMember: true}, nil
}

func (f *fakeDiscordLinker) Refresh(context.Context, string) (*users.Link, error) {
	return nil, users.ErrNotLinked
}

func (f *fakeDiscordLinker) Unlink(context.Context, string) error {
	return users.ErrNotLinked
}

// regCookieValue seals a registration cookie the way submitRegistration
// writes one. The field tags and the purpose string are duplicated here
// rather than shared with the handler: they are part of what the sealed
// cookie means, and a change to either should fail these tests loudly rather
// than be silently followed.
func regCookieValue(t *testing.T, userID string, expires time.Time) string {
	t.Helper()
	v, err := testSealer.Seal(struct {
		Purpose string    `json:"p"`
		UserID  string    `json:"u"`
		Expires time.Time `json:"exp"`
	}{"register-discord", userID, expires})
	if err != nil {
		t.Fatalf("sealing the registration cookie: %v", err)
	}
	return v
}

// cookieNamed returns the named cookie set on the response, or nil.
func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// regRequest builds an anonymous GET carrying the given cookies.
func regRequest(path string, cookies ...*http.Cookie) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	return r
}

// Without a valid registration cookie the start route must refuse to begin
// the OAuth round trip at all: no state cookie, no redirect to Discord —
// otherwise the URL is an open door to starting a flow that would attach a
// Discord identity to nothing, or to whatever a forged cookie names.
func TestDiscordRegisterLinkRequiresValidCookie(t *testing.T) {
	wrongPurpose, err := testSealer.Seal(struct {
		Purpose string    `json:"p"`
		UserID  string    `json:"u"`
		Expires time.Time `json:"exp"`
	}{"some-other-purpose", member.ID, time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatalf("sealing: %v", err)
	}

	tests := []struct {
		name   string
		cookie string
	}{
		{"no cookie at all", ""},
		{"garbage that never was sealed", "not-a-sealed-value"},
		{"expired inside the seal", regCookieValue(t, member.ID, time.Now().Add(-time.Minute))},
		{"sealed for another purpose", wrongPurpose},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := newSite(t, siteConfig{discordSvc: &fakeDiscordLinker{available: true}})

			var cookies []*http.Cookie
			if tt.cookie != "" {
				cookies = append(cookies, &http.Cookie{Name: "itemize_registrering", Value: tt.cookie})
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, regRequest("/registrer/discord", cookies...))

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("got %d, want 303 back to the confirmation page", rec.Code)
			}
			if got := rec.Header().Get("Location"); got != "/registrert" {
				t.Errorf("redirected to %q, want /registrert", got)
			}
			if c := cookieNamed(rec, "itemize_discord_state"); c != nil {
				t.Errorf("a state cookie %q was set, so the OAuth flow was started for nobody", c.Value)
			}
		})
	}
}

// A valid cookie starts the same browser-bound OAuth round trip as the
// profile flow, but with the longer thirty-minute window — this person may be
// creating a Discord account on the way.
func TestDiscordRegisterLinkStartsOAuthWithBoundState(t *testing.T) {
	mux := newSite(t, siteConfig{discordSvc: &fakeDiscordLinker{available: true}})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, regRequest("/registrer/discord", &http.Cookie{
		Name: "itemize_registrering", Value: regCookieValue(t, member.ID, time.Now().Add(time.Hour)),
	}))

	if rec.Code != http.StatusFound {
		t.Fatalf("got %d, want 302 to Discord", rec.Code)
	}
	loc := rec.Header().Get("Location")
	parsed, err := url.Parse(loc)
	if err != nil || parsed.Host != "discord.com" {
		t.Fatalf("redirected to %q, want the Discord authorize endpoint", loc)
	}
	state := parsed.Query().Get("state")
	if state == "" {
		t.Fatal("no state parameter; the callback would accept any forged completion")
	}

	cookie := cookieNamed(rec, "itemize_discord_state")
	if cookie == nil {
		t.Fatal("no state cookie was set, so the callback can never verify the round trip")
	}
	if cookie.Value != state {
		t.Errorf("cookie state %q differs from the URL's %q; every callback would be rejected", cookie.Value, state)
	}
	if !cookie.HttpOnly {
		t.Error("the state cookie is readable by script")
	}
	if cookie.MaxAge != 1800 {
		t.Errorf("state cookie MaxAge = %d, want 1800 — long enough to create a Discord account mid-flow", cookie.MaxAge)
	}
}

// The integration being down reads the same here as on the profile page.
func TestDiscordRegisterLinkUnavailable(t *testing.T) {
	mux := newSite(t, siteConfig{}) // no discordSvc

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, regRequest("/registrer/discord", &http.Cookie{
		Name: "itemize_registrering", Value: regCookieValue(t, member.ID, time.Now().Add(time.Hour)),
	}))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/registrert" {
		t.Errorf("redirected to %q, want /registrert", got)
	}
	kind, text := flashOf(t, rec)
	if kind != "error" || !contains(text, "ikke tilgjengelig") {
		t.Errorf("flash = %q %q; the visitor is not told the integration is down", kind, text)
	}
}

// Someone signed in belongs in the profile flow, whose outcomes land on a
// page their session can open.
func TestDiscordRegisterLinkRedirectsTheSignedIn(t *testing.T) {
	mux := newSite(t, siteConfig{discordSvc: &fakeDiscordLinker{available: true}})

	rec := get(t, mux, "/registrer/discord", member)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/api/discord/link" {
		t.Errorf("redirected to %q, want /api/discord/link", got)
	}
}

// The callback with neither a session nor a registration cookie must fail
// closed: no service call, no cookie touched — acting on anything here would
// mean acting on behalf of nobody in particular.
func TestDiscordCallbackAnonymousWithoutRegCookie(t *testing.T) {
	fake := &fakeDiscordLinker{available: true}
	mux := newSite(t, siteConfig{discordSvc: fake})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, regRequest("/api/discord/callback?state=x&code=y",
		&http.Cookie{Name: "itemize_discord_state", Value: "x"}))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/login" {
		t.Errorf("redirected to %q, want /login", got)
	}
	if len(fake.completed) != 0 {
		t.Errorf("Complete was called %d times for a visitor nobody can vouch for", len(fake.completed))
	}
	if cs := rec.Result().Cookies(); len(cs) != 0 {
		t.Errorf("%d cookies were set; a rejected visitor must leave zero side effects", len(cs))
	}
}

// A sealed session replayed as the registration cookie must not open: the two
// are sealed with the same key, and only the purpose tag keeps one kind of
// cookie from standing in for the other.
func TestDiscordCallbackRejectsReplayedSessionAsRegCookie(t *testing.T) {
	sealed, err := testSealer.Seal(&auth.Session{
		User:    *member,
		Issued:  time.Now(),
		Expires: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("sealing a session: %v", err)
	}

	fake := &fakeDiscordLinker{available: true}
	mux := newSite(t, siteConfig{discordSvc: fake})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, regRequest("/api/discord/callback?state=x&code=y",
		&http.Cookie{Name: "itemize_registrering", Value: sealed},
		&http.Cookie{Name: "itemize_discord_state", Value: "x"}))

	if got := rec.Header().Get("Location"); got != "/login" {
		t.Errorf("redirected to %q, want /login — a session cookie opened as a registration cookie", got)
	}
	if len(fake.completed) != 0 {
		t.Errorf("Complete was called %d times on a replayed session cookie", len(fake.completed))
	}
}

// regCallback drives the callback as an anonymous registrant: a valid
// registration cookie, a state cookie, and whatever query the case needs.
func regCallback(t *testing.T, mux *http.ServeMux, query, stateCookie string) *httptest.ResponseRecorder {
	t.Helper()
	cookies := []*http.Cookie{{
		Name: "itemize_registrering", Value: regCookieValue(t, member.ID, time.Now().Add(time.Hour)),
	}}
	if stateCookie != "" {
		cookies = append(cookies, &http.Cookie{Name: "itemize_discord_state", Value: stateCookie})
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, regRequest("/api/discord/callback?"+query, cookies...))
	return rec
}

// A forged or stale state is rejected for a registrant just as for a member,
// but the message points at the fallback that always works: log in and link
// from the profile. The registration cookie survives, so the button on
// /registrert still works for another attempt.
func TestDiscordCallbackRegistrantStateMismatch(t *testing.T) {
	fake := &fakeDiscordLinker{available: true}
	mux := newSite(t, siteConfig{discordSvc: fake})

	rec := regCallback(t, mux, "state=bbb&code=x", "aaa")

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/registrert" {
		t.Errorf("redirected to %q, want /registrert — the profile page needs a session this visitor does not have", got)
	}
	kind, text := flashOf(t, rec)
	if kind != "error" || !contains(text, "kunne ikke bekreftes") {
		t.Errorf("flash = %q %q; a forged callback would be indistinguishable from success", kind, text)
	}
	if !contains(text, "profilsiden") {
		t.Errorf("flash %q does not point at the profile-page fallback", text)
	}
	stateCookieExpired(t, rec)
	if c := cookieNamed(rec, "itemize_registrering"); c != nil && c.MaxAge < 0 {
		t.Error("the registration cookie was expired on failure, so retrying from /registrert is impossible")
	}
	if len(fake.completed) != 0 {
		t.Errorf("Complete was called %d times despite the state mismatch", len(fake.completed))
	}
}

// Pressing cancel on Discord's consent screen ends the flow with a friendly
// pointer, not a telling-off: the page the registrant returns to just offered
// them the link, so silence would read as something having gone wrong.
func TestDiscordCallbackRegistrantCancelled(t *testing.T) {
	fake := &fakeDiscordLinker{available: true}
	mux := newSite(t, siteConfig{discordSvc: fake})

	rec := regCallback(t, mux, "error=access_denied", "some-state")

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/registrert" {
		t.Errorf("redirected to %q, want /registrert", got)
	}
	kind, text := flashOf(t, rec)
	if kind != "info" || text != "Du kan koble til Discord senere fra profilsiden din." {
		t.Errorf("flash = %q %q, want the later-from-the-profile pointer", kind, text)
	}
	if len(fake.completed) != 0 {
		t.Errorf("Complete was called %d times for a declined authorization", len(fake.completed))
	}
	stateCookieExpired(t, rec)
}

// The success path: the state matches, Complete runs for the registrant's
// user id, and the registration cookie is spent — its capability is fulfilled
// and must not linger.
func TestDiscordCallbackRegistrantSuccess(t *testing.T) {
	fake := &fakeDiscordLinker{available: true}
	mux := newSite(t, siteConfig{discordSvc: fake})

	rec := regCallback(t, mux, "state=s1&code=the-code", "s1")

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/registrert" {
		t.Errorf("redirected to %q, want /registrert", got)
	}
	if len(fake.completed) != 1 {
		t.Fatalf("Complete was called %d times, want 1", len(fake.completed))
	}
	call := fake.completed[0]
	if call.userID != member.ID {
		t.Errorf("Complete ran for user %q, want the registrant %q from the sealed cookie", call.userID, member.ID)
	}
	if call.code != "the-code" {
		t.Errorf("Complete got code %q, want the one Discord sent back", call.code)
	}
	if call.redirectURI != "https://itemize.no/api/discord/callback" {
		t.Errorf("Complete got redirect URI %q; Discord rejects any mismatch with the authorize request", call.redirectURI)
	}
	kind, text := flashOf(t, rec)
	if kind != "success" || !contains(text, "koblet") {
		t.Errorf("flash = %q %q, want the link confirmed", kind, text)
	}
	stateCookieExpired(t, rec)

	reg := cookieNamed(rec, "itemize_registrering")
	if reg == nil || reg.MaxAge >= 0 {
		t.Error("the registration cookie was not expired after success; its capability should be spent")
	}
}

// The confirmation page only offers the Discord button to someone who
// demonstrably just registered — with the integration on and the sealed
// cookie valid. Anyone else typing the URL gets the plain confirmation.
func TestRegistrertOffersDiscordOnlyWithValidCookie(t *testing.T) {
	const button = `href="/registrer/discord"`

	t.Run("valid cookie shows the offer", func(t *testing.T) {
		mux := newSite(t, siteConfig{discordSvc: &fakeDiscordLinker{available: true}})

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, regRequest("/registrert", &http.Cookie{
			Name: "itemize_registrering", Value: regCookieValue(t, member.ID, time.Now().Add(time.Hour)),
		}))

		body := rec.Body.String()
		if !contains(body, button) || !contains(body, "Koble til Discord nå") {
			t.Error("the confirmation page does not offer the Discord link to a fresh registrant")
		}
	})

	t.Run("no cookie, no offer", func(t *testing.T) {
		mux := newSite(t, siteConfig{discordSvc: &fakeDiscordLinker{available: true}})

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, regRequest("/registrert"))

		if contains(rec.Body.String(), button) {
			t.Error("the page offers a flow that would bounce a visitor with no registration cookie straight back")
		}
	})

	t.Run("integration off, no offer", func(t *testing.T) {
		mux := newSite(t, siteConfig{})

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, regRequest("/registrert", &http.Cookie{
			Name: "itemize_registrering", Value: regCookieValue(t, member.ID, time.Now().Add(time.Hour)),
		}))

		if contains(rec.Body.String(), button) {
			t.Error("the page offers a flow the unconfigured integration would immediately refuse")
		}
	})
}
