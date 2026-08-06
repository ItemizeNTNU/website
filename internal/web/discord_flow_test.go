package web_test

// The account-linking flow, up to the state check. Nothing after a successful
// state comparison is exercised here: Complete talks to the hardcoded
// discord.APIBase, and there is no seam to fake that out — deliberately out
// of scope.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ItemizeNTNU/website/internal/config"
	"github.com/ItemizeNTNU/website/internal/discord"
	"github.com/ItemizeNTNU/website/internal/users"
)

// availableDiscordSvc builds a fully configured DiscordService whose
// FusionAuth side is the given fake. discord.New returns nil unless every
// config field is set, and DiscordService.Available also requires a
// configured FusionAuth client.
func availableDiscordSvc(t *testing.T, fusionHandler http.HandlerFunc) (*users.DiscordService, siteConfig) {
	t.Helper()
	client := discord.New(config.Discord{
		ClientID:     "cid",
		ClientSecret: "cs",
		BotToken:     "Bot x",
		GuildID:      "1",
		MemberRoleID: "2",
	}, discardLogger())
	if client == nil {
		t.Fatal("discord.New returned nil despite a fully populated config")
	}
	fusion := fakeFusion(t, fusionHandler)
	svc := users.NewDiscordService(client, fusion, discardLogger())
	return svc, siteConfig{fusion: fusion, discordSvc: svc}
}

func TestDiscordLinkUnavailable(t *testing.T) {
	mux := newSite(t, siteConfig{}) // no discordSvc

	rec := get(t, mux, "/api/discord/link", member)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want 303 back to the profile", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/profil" {
		t.Errorf("redirected to %q, want /profil", got)
	}
	kind, text := flashOf(t, rec)
	if kind != "error" || !contains(text, "ikke tilgjengelig") {
		t.Errorf("flash = %q %q; the member is not told the integration is down", kind, text)
	}
}

// Starting the flow must bind an unpredictable state to the browser: the
// value sent to Discord and the value in the cookie are the same secret, and
// the callback compares them.
func TestDiscordLinkStartsOAuthWithBoundState(t *testing.T) {
	_, cfg := availableDiscordSvc(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("starting the link flow contacted FusionAuth (%s)", r.URL.Path)
	})
	mux := newSite(t, cfg)

	rec := get(t, mux, "/api/discord/link", member)
	if rec.Code != http.StatusFound {
		t.Fatalf("got %d, want 302 to Discord", rec.Code)
	}

	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://discord.com/oauth2/authorize") {
		t.Fatalf("redirected to %q, want the Discord authorize endpoint", loc)
	}
	parsed, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("the authorize URL does not parse: %v", err)
	}
	state := parsed.Query().Get("state")
	if state == "" {
		t.Fatal("no state parameter; the callback would accept any forged completion")
	}

	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "itemize_discord_state" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no state cookie was set, so the callback can never verify the round trip")
	}
	if cookie.Value != state {
		t.Errorf("cookie state %q differs from the URL's %q; every callback would be rejected", cookie.Value, state)
	}
	if !cookie.HttpOnly {
		t.Error("the state cookie is readable by script")
	}
	if cookie.MaxAge != 600 {
		t.Errorf("state cookie MaxAge = %d, want 600", cookie.MaxAge)
	}
	if cookie.Path != "/" {
		t.Errorf("state cookie Path = %q, want / so the /api callback can read it", cookie.Path)
	}
}

// callbackRequest builds a callback GET carrying an optional state cookie.
func callbackRequest(query, cookieValue string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/discord/callback?"+query, nil)
	if cookieValue != "" {
		r.AddCookie(&http.Cookie{Name: "itemize_discord_state", Value: cookieValue})
	}
	return asUser(r, member)
}

// stateCookieExpired returns the state cookie set on the response, which the
// callback must always expire — one attempt per cookie.
func stateCookieExpired(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name != "itemize_discord_state" {
			continue
		}
		if c.MaxAge >= 0 {
			t.Errorf("the state cookie was not expired (MaxAge=%d); a failed attempt could be replayed", c.MaxAge)
		}
		return
	}
	t.Error("the callback did not touch the state cookie at all")
}

// Pressing cancel on Discord's consent screen is not an error worth a
// message — the member gets their profile back, quietly.
func TestDiscordCallbackCancelled(t *testing.T) {
	_, cfg := availableDiscordSvc(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a cancelled callback contacted FusionAuth (%s)", r.URL.Path)
	})
	mux := newSite(t, cfg)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, callbackRequest("error=access_denied", "some-state"))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/profil" {
		t.Errorf("redirected to %q, want /profil", got)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "flash" {
			t.Errorf("a flash %q was queued for a deliberate cancel — the member would be told off for changing their mind", c.Value)
		}
	}
	stateCookieExpired(t, rec)
}

func TestDiscordCallbackStateMismatch(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		cookie string
	}{
		{"cookie and query disagree", "state=bbb&code=x", "aaa"},
		{"no cookie at all", "state=bbb&code=x", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, cfg := availableDiscordSvc(t, func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("a rejected callback contacted FusionAuth (%s)", r.URL.Path)
			})
			mux := newSite(t, cfg)

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, callbackRequest(tt.query, tt.cookie))

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("got %d, want 303", rec.Code)
			}
			kind, text := flashOf(t, rec)
			if kind != "error" || !contains(text, "kunne ikke bekreftes") {
				t.Errorf("flash = %q %q; a forged callback would be indistinguishable from success", kind, text)
			}
			stateCookieExpired(t, rec)
		})
	}
}

// Refresh and unlink on an account that was never linked: FusionAuth answers,
// but its record has no discord block.
func TestDiscordRefreshAndUnlinkWithoutLink(t *testing.T) {
	for _, path := range []string{"/profil/discord/oppdater", "/profil/discord/koble-fra"} {
		t.Run(path, func(t *testing.T) {
			_, cfg := availableDiscordSvc(t, func(w http.ResponseWriter, r *http.Request) {
				// A user record with no data.discord block.
				_, _ = w.Write([]byte(`{"user":{"id":"` + member.ID + `"}}`))
			})
			mux := newSite(t, cfg)

			rec := postForm(t, mux, path, nil, member)
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("got %d, want 303", rec.Code)
			}
			kind, text := flashOf(t, rec)
			if kind != "error" || text != "Du har ingen Discord-konto koblet." {
				t.Errorf("flash = %q %q, want the missing link named plainly", kind, text)
			}
		})
	}
}

func TestDiscordRefreshUnavailable(t *testing.T) {
	mux := newSite(t, siteConfig{}) // no discordSvc

	rec := postForm(t, mux, "/profil/discord/oppdater", nil, member)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want 303", rec.Code)
	}
	kind, text := flashOf(t, rec)
	if kind != "error" || !contains(text, "ikke tilgjengelig") {
		t.Errorf("flash = %q %q; the member is not told the integration is down", kind, text)
	}
}
