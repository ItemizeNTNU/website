package discord

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// The authorization link is the one URL a member's browser follows, so every
// parameter in it has to be exactly right: a mismatched redirect_uri is
// rejected by Discord before the consent screen, and a missing response_type
// silently returns the wrong grant.
func TestAuthorizeURLParameters(t *testing.T) {
	c := &Client{cfg: testConfig()}
	raw := c.AuthorizeURL("s3cr3t-state", "https://itemize.no/api/discord/callback")

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("the authorize link is not a URL (%v): %s", err, raw)
	}
	if u.Scheme != "https" || u.Host != "discord.com" {
		t.Errorf("the link points at %s://%s, not Discord", u.Scheme, u.Host)
	}
	// The authorization endpoint sits outside the versioned API path, unlike
	// every other call in this package.
	if u.Path != "/oauth2/authorize" {
		t.Errorf("path = %q, want /oauth2/authorize", u.Path)
	}

	want := map[string]string{
		"client_id":     "client-1",
		"redirect_uri":  "https://itemize.no/api/discord/callback",
		"response_type": "code",
		"scope":         "identify",
		"state":         "s3cr3t-state",
		// Without this, a member who already authorized is bounced straight
		// through and can never re-link to a different account.
		"prompt": "consent",
	}
	q := u.Query()
	for key, value := range want {
		if got := q.Get(key); got != value {
			t.Errorf("%s = %q, want %q", key, got, value)
		}
	}
	// Anything beyond identify asks for access the site never uses, and every
	// extra scope is one more thing on the consent screen to say no to.
	if len(q) != len(want) {
		t.Errorf("the link carries %d parameters, want %d: %v", len(q), len(want), q)
	}
	if scope := q.Get("scope"); strings.Contains(scope, " ") || strings.Contains(scope, "+") {
		t.Errorf("scope = %q, want identify alone", scope)
	}
	// The client secret must never leave the server.
	if strings.Contains(raw, "secret-1") {
		t.Fatalf("the client secret is in the authorize link: %s", raw)
	}
}

// state is unpredictable and bound to the session, which means it is whatever
// the session layer generated — base64 with padding and slashes, most likely.
// Losing a character to encoding makes the callback fail its own state check
// and rejects every legitimate login.
func TestAuthorizeURLEncodesTheState(t *testing.T) {
	c := &Client{cfg: testConfig()}

	states := []string{
		"",
		"plain",
		"a+b/c=d",                   // unpadded and padded base64 alphabet
		"has spaces and &ersands",   //
		"?query=like#fragment-like", //
		"æøå 🚩",                     // not what the session layer emits, but must survive anyway
		strings.Repeat("x", 512),
	}

	for _, state := range states {
		t.Run("state="+state, func(t *testing.T) {
			u, err := url.Parse(c.AuthorizeURL(state, "https://itemize.no/cb?next=/profil"))
			if err != nil {
				t.Fatalf("the authorize link is not a URL: %v", err)
			}
			if got := u.Query().Get("state"); got != state {
				t.Errorf("state came back as %q, want %q — the callback would reject "+
					"its own state and no login would ever complete", got, state)
			}
			// A redirect_uri that loses its own query string no longer matches
			// what is registered in the developer portal.
			if got := u.Query().Get("redirect_uri"); got != "https://itemize.no/cb?next=/profil" {
				t.Errorf("redirect_uri came back as %q", got)
			}
		})
	}
}

// Declining on the consent screen sends the browser back with no code at all.
// That is a decision, not a failure, and it must be distinguishable so the
// callback can say "you declined" rather than "something went wrong".
func TestExchangeWithoutACodeIsADenial(t *testing.T) {
	c, fake := newFakeDiscord(t, jsonReply(200, `{"access_token":"tok"}`))

	u, err := c.Exchange(context.Background(), "", "https://itemize.no/cb")
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("got %v, want ErrDenied", err)
	}
	if u != nil {
		t.Errorf("got an account back for a declined authorization: %+v", u)
	}
	if n := fake.count(); n != 0 {
		t.Errorf("a declined authorization still sent %d requests to Discord", n)
	}
}

// The token endpoint authenticates with the client credentials in a form body,
// not with the bot token every other call uses. Sending the bot token here, or
// JSON instead of a form, is a 401 that looks like a bad secret.
func TestExchangeSendsFormEncodedClientCredentials(t *testing.T) {
	c, fake := newFakeDiscord(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v10/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"user-token","token_type":"Bearer"}`))
		default:
			_, _ = w.Write([]byte(`{"id":"80351110224678912","username":"kari","global_name":"Kari"}`))
		}
	})

	u, err := c.Exchange(context.Background(), "the-code", "https://itemize.no/cb")
	if err != nil {
		t.Fatalf("a valid exchange failed: %v", err)
	}
	if u == nil || u.ID != "80351110224678912" {
		t.Fatalf("got %+v, want the account behind the token", u)
	}

	got := fake.requests()
	if len(got) != 2 {
		t.Fatalf("got %d requests, want the token exchange and the account lookup", len(got))
	}

	token := got[0]
	if token.Method != http.MethodPost {
		t.Errorf("token request method = %q, want POST", token.Method)
	}
	if token.Path != "/api/v10/oauth2/token" {
		t.Errorf("token path = %q", token.Path)
	}
	if ct := token.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want a form; Discord's token endpoint does not accept JSON", ct)
	}
	// The bot token has no business here, and sending it would leak the bot's
	// credentials to an endpoint that does not need them.
	if auth := token.Header.Get("Authorization"); auth != "" {
		t.Errorf("the token request carried Authorization %q; the credentials go in the body", auth)
	}
	if token.Header.Get("User-Agent") != userAgent {
		t.Errorf("User-Agent = %q, want the descriptive bot agent", token.Header.Get("User-Agent"))
	}

	form, err := url.ParseQuery(string(token.Body))
	if err != nil {
		t.Fatalf("the token body is not a form (%v): %s", err, token.Body)
	}
	for key, want := range map[string]string{
		"client_id":     "client-1",
		"client_secret": "secret-1",
		"grant_type":    "authorization_code",
		"code":          "the-code",
		"redirect_uri":  "https://itemize.no/cb",
	} {
		if got := form.Get(key); got != want {
			t.Errorf("form %s = %q, want %q", key, got, want)
		}
	}

	// The account lookup uses the member's token, not the bot's. Using the bot
	// token here would return the bot's own account for every member.
	me := got[1]
	if me.Method != http.MethodGet || me.Path != "/api/v10/users/@me" {
		t.Errorf("account lookup = %s %s, want GET /api/v10/users/@me", me.Method, me.Path)
	}
	if auth := me.Header.Get("Authorization"); auth != "Bearer user-token" {
		t.Errorf("Authorization = %q, want the member's bearer token", auth)
	}
}

// Every way the token exchange can fail has to stop the flow rather than
// produce a half-built account. A blank id stored against a real member is
// worse than a visible error.
func TestExchangeTokenEndpointFailures(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus int
		wantText   string
	}{
		{
			// The code is single-use and expires in a minute; a reloaded
			// callback lands here routinely.
			name: "a reused or expired code", status: 400,
			body:       `{"error":"invalid_grant","error_description":"Invalid \"code\" in request."}`,
			wantStatus: 400,
		},
		{"a rejected client secret", 401, `{"error":"invalid_client"}`, 401, ""},
		{"the app was disabled", 403, `{"message":"Missing Access","code":50001}`, 403, "Missing Access"},
		{"Discord is down", 503, ``, 503, ""},
		{"rate limited", 429, `{"message":"You are being rate limited.","code":0}`, 429, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newFakeDiscord(t, jsonReply(tt.status, tt.body))

			u, err := c.Exchange(context.Background(), "the-code", "https://itemize.no/cb")
			if u != nil {
				t.Errorf("got an account back from a failed exchange: %+v", u)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("got %T (%v), want *APIError", err, err)
			}
			if apiErr.Status != tt.wantStatus {
				t.Errorf("Status = %d, want %d", apiErr.Status, tt.wantStatus)
			}
			if tt.wantText != "" && !strings.Contains(apiErr.Error(), tt.wantText) {
				t.Errorf("Error() = %q, want it to mention %q", apiErr.Error(), tt.wantText)
			}
		})
	}
}

// The token endpoint is not retried on 429 the way the bot calls are — it is
// a direct http.Do, not a call through do. This pins that, so a change that
// adds retrying there is a deliberate one.
func TestExchangeDoesNotRetryOnRateLimit(t *testing.T) {
	c, fake := newFakeDiscord(t, rateLimited)

	if _, err := c.Exchange(context.Background(), "the-code", "https://itemize.no/cb"); err == nil {
		t.Fatal("a rate-limited exchange was reported as success")
	}
	if n := fake.count(); n != 1 {
		t.Errorf("the token endpoint saw %d requests, want 1", n)
	}
}

// A 2xx whose body is not the token response is a failure. Continuing with an
// empty access token would produce a 401 on the account lookup instead, and
// the log would blame the wrong call.
func TestExchangeRejectsUnusableTokenResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"no access token at all", `{"token_type":"Bearer"}`, "access token"},
		{"an empty access token", `{"access_token":"","token_type":"Bearer"}`, "access token"},
		{"an empty object", `{}`, "access token"},
		{"truncated JSON", `{"access_token":"tok`, ""},
		{"an HTML error page", `<html>502</html>`, ""},
		{"nothing at all", ``, ""},
		{"an array where an object belongs", `["tok"]`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, fake := newFakeDiscord(t, jsonReply(200, tt.body))

			u, err := c.Exchange(context.Background(), "the-code", "https://itemize.no/cb")
			if err == nil {
				t.Fatal("an unusable token response was accepted; the account lookup " +
					"would fail next with an error blaming the wrong call")
			}
			if u != nil {
				t.Errorf("got an account back: %+v", u)
			}
			if tt.want != "" && !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to mention %q", err, tt.want)
			}
			// The account lookup must not have happened.
			if n := fake.count(); n != 1 {
				t.Errorf("Discord saw %d requests, want only the token exchange", n)
			}
		})
	}
}

// The account lookup is the step that decides which Discord account gets
// linked. An id-less response has to be an error: storing an empty identifier
// would link the member to nothing and quietly break role management.
func TestExchangeRejectsAnAccountWithoutAnID(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"an empty object", `{}`},
		{"a username but no id", `{"username":"kari","global_name":"Kari"}`},
		{"an explicitly empty id", `{"id":"","username":"kari"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newFakeDiscord(t, sequence(
				jsonReply(200, `{"access_token":"tok","token_type":"Bearer"}`),
				jsonReply(200, tt.body),
			))

			u, err := c.Exchange(context.Background(), "the-code", "https://itemize.no/cb")
			if err == nil {
				t.Fatal("an account with no id was accepted; the member would be " +
					"linked to nothing and lose role management")
			}
			if u != nil {
				t.Errorf("got %+v alongside the error", u)
			}
			if !strings.Contains(err.Error(), "id") {
				t.Errorf("error = %q, want it to say the id was missing", err)
			}
		})
	}
}

// Failures on the second leg must be reported as such, not as a denial or a
// blank account.
func TestExchangeAccountLookupFailures(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, _ := newFakeDiscord(t, sequence(
				jsonReply(200, `{"access_token":"tok","token_type":"Bearer"}`),
				jsonReply(status, `{"message":"nope","code":0}`),
			))

			u, err := c.Exchange(context.Background(), "the-code", "https://itemize.no/cb")
			if u != nil {
				t.Errorf("got an account back from a failed lookup: %+v", u)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Status != status {
				t.Fatalf("got %v, want an APIError carrying %d", err, status)
			}
		})
	}
}

// Malformed JSON from the account endpoint is a failure, not an empty account.
func TestExchangeRejectsMalformedAccountJSON(t *testing.T) {
	for _, body := range []string{`{"id":"803511102`, `not json`, `[]`, ``} {
		t.Run(body, func(t *testing.T) {
			c, _ := newFakeDiscord(t, sequence(
				jsonReply(200, `{"access_token":"tok","token_type":"Bearer"}`),
				jsonReply(200, body),
			))

			if _, err := c.Exchange(context.Background(), "the-code", "https://itemize.no/cb"); err == nil {
				t.Fatal("an unparseable account response was accepted")
			}
		})
	}
}

// Both OAuth legs read the body themselves rather than going through do, so
// each needs its own guard against a connection that dies mid-response. A
// partial read must never be mistaken for a complete answer.
func TestExchangeSurvivesADroppedConnection(t *testing.T) {
	t.Run("during the token exchange", func(t *testing.T) {
		c, _ := newFakeDiscord(t, truncatedReply)

		if _, err := c.Exchange(context.Background(), "the-code", "https://itemize.no/cb"); err == nil {
			t.Fatal("a truncated token response was accepted")
		}
	})

	t.Run("during the account lookup", func(t *testing.T) {
		c, _ := newFakeDiscord(t, sequence(
			jsonReply(200, `{"access_token":"tok","token_type":"Bearer"}`),
			truncatedReply,
		))

		u, err := c.Exchange(context.Background(), "the-code", "https://itemize.no/cb")
		if err == nil {
			t.Fatal("a truncated account response was accepted; the member would be " +
				"linked to whatever partial account arrived")
		}
		if u != nil {
			t.Errorf("got %+v alongside the error", u)
		}
	})
}

// When Discord cannot be reached the error has to say which leg failed. "The
// exchange" and "the account lookup" point at different problems.
func TestExchangeNamesTheFailingLeg(t *testing.T) {
	t.Run("the token exchange", func(t *testing.T) {
		c, fake := newFakeDiscord(t, nil)
		fake.stop()

		_, err := c.Exchange(context.Background(), "the-code", "https://itemize.no/cb")
		if err == nil {
			t.Fatal("an unreachable Discord was reported as success")
		}
		if !strings.Contains(err.Error(), "exchanging the code") {
			t.Errorf("error = %q, want it to name the token exchange", err)
		}
	})

	t.Run("a cancelled caller", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		c, fake := newFakeDiscord(t, jsonReply(200, `{"access_token":"tok"}`))

		_, err := c.Exchange(ctx, "the-code", "https://itemize.no/cb")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
		if n := fake.count(); n != 0 {
			t.Errorf("a cancelled exchange sent %d requests", n)
		}
	})
}

// A linked account is rendered from whatever Discord returns, including names
// in scripts the site never anticipated. Anything mangled here is written
// verbatim into the member's profile.
func TestGetUserDecodesRealisticAccounts(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		wantDisplay     string
		wantAvatarStart string
	}{
		{
			name:            "a migrated account with a global name",
			body:            `{"id":"80351110224678912","username":"kari","global_name":"Kari Nordmann","discriminator":"0","avatar":"abc123"}`,
			wantDisplay:     "Kari Nordmann",
			wantAvatarStart: "https://cdn.discordapp.com/avatars/80351110224678912/abc123.png",
		},
		{
			name:            "Norwegian letters survive",
			body:            `{"id":"80351110224678912","username":"kari","global_name":"Kåre Øksnes Ånestad","discriminator":"0"}`,
			wantDisplay:     "Kåre Øksnes Ånestad",
			wantAvatarStart: "https://cdn.discordapp.com/embed/avatars/",
		},
		{
			name:            "emoji in a display name",
			body:            `{"id":"80351110224678912","username":"kari","global_name":"kari 🚩🎉","discriminator":"0"}`,
			wantDisplay:     "kari 🚩🎉",
			wantAvatarStart: "https://cdn.discordapp.com/embed/avatars/",
		},
		{
			name:            "an escaped surrogate pair",
			body:            `{"id":"80351110224678912","username":"kari","global_name":"kari \ud83d\udea9","discriminator":"0"}`,
			wantDisplay:     "kari 🚩",
			wantAvatarStart: "https://cdn.discordapp.com/embed/avatars/",
		},
		{
			// Discord's animated avatars carry an a_ prefix that has to reach
			// the CDN path intact or the image 404s.
			name:            "an animated avatar hash",
			body:            `{"id":"80351110224678912","username":"kari","avatar":"a_1a2b3c4d5e6f","discriminator":"0"}`,
			wantDisplay:     "kari",
			wantAvatarStart: "https://cdn.discordapp.com/avatars/80351110224678912/a_1a2b3c4d5e6f.png",
		},
		{
			name:            "a legacy account still has its discriminator",
			body:            `{"id":"80351110224678912","username":"kari","discriminator":"0042"}`,
			wantDisplay:     "kari#0042",
			wantAvatarStart: "https://cdn.discordapp.com/embed/avatars/",
		},
		{
			// Everything but the id is optional as far as the decoder is
			// concerned, and a sparse account must still render something.
			name:            "nothing but an id",
			body:            `{"id":"80351110224678912"}`,
			wantDisplay:     "",
			wantAvatarStart: "https://cdn.discordapp.com/embed/avatars/",
		},
		{
			name:            "fields Discord added since",
			body:            `{"id":"80351110224678912","username":"kari","discriminator":"0","banner":null,"accent_color":123,"clan":{"tag":"x"}}`,
			wantDisplay:     "kari",
			wantAvatarStart: "https://cdn.discordapp.com/embed/avatars/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newFakeDiscord(t, jsonReply(200, tt.body))

			u, err := c.GetUser(context.Background(), testSnowflake)
			if err != nil {
				t.Fatalf("a valid account was rejected: %v", err)
			}
			if got := u.DisplayName(); got != tt.wantDisplay {
				t.Errorf("display name = %q, want %q — this is what is stored and shown", got, tt.wantDisplay)
			}
			if got := u.AvatarURL(); !strings.HasPrefix(got, tt.wantAvatarStart) {
				t.Errorf("avatar = %q, want it to start with %q", got, tt.wantAvatarStart)
			}
		})
	}
}

// GetUser uses the bot's own credentials rather than a member token, because
// it runs on refresh long after the OAuth round trip is over.
func TestGetUserUsesTheBotToken(t *testing.T) {
	c, fake := newFakeDiscord(t, jsonReply(200, `{"id":"`+testSnowflake+`"}`))

	if _, err := c.GetUser(context.Background(), testSnowflake); err != nil {
		t.Fatal(err)
	}
	got := fake.first()
	if auth := got.Header.Get("Authorization"); auth != "Bot bot-1" {
		t.Errorf("Authorization = %q, want the bot token; a bearer token would be "+
			"expired by the time refresh runs", auth)
	}
	if want := "/api/v10/users/" + testSnowflake; got.Path != want {
		t.Errorf("path = %q, want %q", got.Path, want)
	}
}

// An account that was deleted between the link being stored and the refresh is
// a 404. It must reach the caller rather than becoming a blank account.
func TestGetUserFailures(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, _ := newFakeDiscord(t, jsonReply(status, `{"message":"Unknown User","code":10013}`))

			u, err := c.GetUser(context.Background(), testSnowflake)
			if u != nil {
				t.Errorf("got an account back from a %d: %+v", status, u)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Status != status {
				t.Fatalf("got %v, want an APIError carrying %d", err, status)
			}
		})
	}
}
