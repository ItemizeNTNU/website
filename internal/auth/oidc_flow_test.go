package auth

// End-to-end tests for the OpenID Connect login flow, driven against a fake
// provider on a local httptest server. Nothing here reaches FusionAuth or any
// other host: discovery, the token exchange and the JWKS endpoint are all
// served by newFakeIDP, so the suite runs offline and at full speed.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/ItemizeNTNU/website/internal/config"
)

const (
	testClientID     = "itemize-web"
	testClientSecret = "client-secret-0123456789abcdefgh"
	testBaseURL      = "https://itemize.no"
)

func nullLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// ── A fake identity provider ──────────────────────────────────────────────

// fakeIDP serves just enough of FusionAuth for this package: the discovery
// document, an empty JWKS, and a token endpoint whose behaviour each test can
// bend. The authorization endpoint is never actually fetched — the browser step
// is simulated by reading the redirect and constructing the callback by hand.
type fakeIDP struct {
	srv *httptest.Server

	// idToken is what the token endpoint returns as "id_token". Tests set it
	// after starting the login, once the nonce is known.
	idToken string
	// omitIDToken makes the token response leave out id_token entirely, which
	// is what a misconfigured application without the openid scope produces.
	omitIDToken bool
	// tokenStatus, when non-zero, replaces the whole token response with that
	// status and an OAuth error body.
	tokenStatus int

	mu         sync.Mutex
	tokenForm  url.Values
	tokenCalls int
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()
	idp := &fakeIDP{}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                idp.srv.URL,
			"authorization_endpoint":                idp.srv.URL + "/oauth2/authorize",
			"token_endpoint":                        idp.srv.URL + "/oauth2/token",
			"jwks_uri":                              idp.srv.URL + "/.well-known/jwks.json",
			"userinfo_endpoint":                     idp.srv.URL + "/oauth2/userinfo",
			"id_token_signing_alg_values_supported": []string{"HS256", "RS256"},
		})
	})
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Deliberately empty: under RS256 there is no key here that could
		// verify an HS256 token, which is what the RS256 test relies on.
		_, _ = w.Write([]byte(`{"keys":[]}`))
	})
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()

		idp.mu.Lock()
		idp.tokenForm = r.PostForm
		idp.tokenCalls++
		status := idp.tokenStatus
		idp.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}

		body := map[string]any{
			"access_token": "an-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}
		if !idp.omitIDToken {
			body["id_token"] = idp.idToken
		}
		_ = json.NewEncoder(w).Encode(body)
	})

	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)
	return idp
}

func (f *fakeIDP) lastTokenRequest(t *testing.T) url.Values {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tokenCalls == 0 {
		t.Fatal("the token endpoint was never called, so no code was ever exchanged")
	}
	return f.tokenForm
}

// claimsFor is a complete, valid claim set for the nonce a login just issued.
// Each test copies it and breaks exactly one thing.
func (f *fakeIDP) claimsFor(nonce string) map[string]any {
	return map[string]any{
		"iss":      f.srv.URL,
		"aud":      testClientID,
		"sub":      "11111111-2222-4333-8444-999999999999",
		"exp":      time.Now().Add(time.Hour).Unix(),
		"iat":      time.Now().Unix(),
		"nonce":    nonce,
		"name":     "Bjørn",
		"fullName": "Bjørn Ærlig Ødegård",
		"email":    "bjorn@stud.ntnu.no",
		"imageUrl": "https://auth.example/avatar.png",
		"roles":    []string{"Medlem", RoleStyret},
	}
}

func signClaims(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return mintJWT(t, secret, hs256Header("HS256"), string(payload))
}

// newAuthenticator wires an Authenticator to the fake provider, exercising New
// — including its discovery call and its choice of verifier — rather than
// building the struct by hand.
func newAuthenticator(t *testing.T, idp *fakeIDP, alg, idTokenHMACSecret string) (*Authenticator, *Sealer) {
	t.Helper()

	host, err := url.Parse(idp.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	base, err := url.Parse(testBaseURL)
	if err != nil {
		t.Fatal(err)
	}

	sealer := newTestSealer(t, testSecret, true)
	a, err := New(context.Background(), &config.Config{
		BaseURL: base,
		FusionAuth: config.FusionAuth{
			Host:              host,
			ClientID:          testClientID,
			ClientSecret:      testClientSecret,
			IDTokenAlg:        alg,
			IDTokenHMACSecret: idTokenHMACSecret,
		},
	}, sealer, nullLogger())
	if err != nil {
		t.Fatalf("building the authenticator against the fake provider: %v", err)
	}
	return a, sealer
}

// startLogin runs Login and returns where the visitor was sent and the flow
// cookie their browser would now hold.
func startLogin(t *testing.T, a *Authenticator, returnTo string) (*url.URL, *http.Cookie) {
	t.Helper()

	target := "/login"
	if returnTo != "" {
		target += "?return_to=" + url.QueryEscape(returnTo)
	}
	rec := httptest.NewRecorder()
	a.Login(rec, httptest.NewRequest(http.MethodGet, target, nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("Login returned %d rather than a redirect to the provider: %s",
			rec.Code, rec.Body.String())
	}
	authURL, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("the authorization URL is not a URL: %v", err)
	}

	for _, c := range rec.Result().Cookies() {
		if c.Name == flowCookie {
			return authURL, c
		}
	}
	t.Fatal("Login set no flow cookie, so the callback could never be verified")
	return nil, nil
}

// callback runs the callback the way the browser would: with the flow cookie
// attached and the provider's parameters in the query string.
func callback(t *testing.T, a *Authenticator, flow *http.Cookie, query url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/callback?"+query.Encode(), nil)
	if flow != nil {
		r.AddCookie(flow)
	}
	rec := httptest.NewRecorder()
	a.Callback(rec, r)
	return rec
}

// sessionFrom reads back whatever session the response established, or nil.
func sessionFrom(t *testing.T, sealer *Sealer, rec *httptest.ResponseRecorder) *Session {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookie && c.Value != "" {
			r.AddCookie(c)
		}
	}
	return sealer.Read(r)
}

// ── Login ─────────────────────────────────────────────────────────────────

// The authorization request has to carry everything the provider needs, and
// three of these parameters are security-critical: state is the CSRF defence
// for the callback, nonce binds the ID token to this particular login, and the
// PKCE challenge stops an intercepted code from being redeemed by anyone else.
func TestLoginBuildsTheAuthorizationRequest(t *testing.T) {
	idp := newFakeIDP(t)
	a, sealer := newAuthenticator(t, idp, "HS256", "")

	authURL, flowCookie := startLogin(t, a, "/profil")
	q := authURL.Query()

	if got := authURL.Scheme + "://" + authURL.Host + authURL.Path; got != idp.srv.URL+"/oauth2/authorize" {
		t.Errorf("the visitor was sent to %q rather than the provider's authorization "+
			"endpoint", got)
	}
	if got := q.Get("client_id"); got != testClientID {
		t.Errorf("client_id = %q, want %q", got, testClientID)
	}
	if got := q.Get("redirect_uri"); got != testBaseURL+"/callback" {
		t.Errorf("redirect_uri = %q; it must match the callback route exactly or the "+
			"provider refuses the request", got)
	}
	if got := q.Get("response_type"); got != "code" {
		t.Errorf("response_type = %q, want code; anything else is the implicit flow", got)
	}
	for _, scope := range []string{oidc.ScopeOpenID, "profile", "email"} {
		if !strings.Contains(q.Get("scope"), scope) {
			t.Errorf("scope %q is missing %q; without it the ID token arrives without the "+
				"claims the site renders", q.Get("scope"), scope)
		}
	}
	if q.Get("state") == "" {
		t.Error("no state parameter, so the callback has nothing to check against and " +
			"becomes forgeable")
	}
	if q.Get("nonce") == "" {
		t.Error("no nonce parameter, so an ID token from an unrelated login could be replayed")
	}
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		t.Errorf("PKCE challenge is %q with method %q; without S256 an intercepted "+
			"authorization code can be redeemed by whoever intercepted it",
			q.Get("code_challenge"), q.Get("code_challenge_method"))
	}

	// The cookie is the only place state, nonce and verifier are kept — the
	// flow is stateless by design — so it must agree with the URL.
	var flow flowState
	if err := sealer.Open(flowCookie.Value, &flow); err != nil {
		t.Fatalf("the flow cookie could not be opened: %v", err)
	}
	if flow.State != q.Get("state") {
		t.Error("the state in the cookie does not match the state sent to the provider, so " +
			"every callback would be rejected")
	}
	if flow.Nonce != q.Get("nonce") {
		t.Error("the nonce in the cookie does not match the one sent to the provider")
	}
	if flow.ReturnTo != "/profil" {
		t.Errorf("ReturnTo = %q, want /profil", flow.ReturnTo)
	}
	if flow.Expires.IsZero() {
		t.Error("the flow has no expiry, so an abandoned login attempt stays usable forever")
	}
}

// The flow cookie carries the state, nonce and PKCE verifier. It must be
// unreadable by script and short-lived: a leaked verifier plus an intercepted
// code is a complete account takeover.
func TestLoginFlowCookieAttributes(t *testing.T) {
	idp := newFakeIDP(t)
	a, _ := newAuthenticator(t, idp, "HS256", "")

	_, c := startLogin(t, a, "")

	if !c.HttpOnly {
		t.Error("the flow cookie is not HttpOnly; script on the page could read the PKCE " +
			"verifier and the state parameter")
	}
	if !c.Secure {
		t.Error("the flow cookie is not Secure on a TLS deployment")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v; Strict would drop the cookie on the redirect back from the "+
			"provider and no login could ever complete", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("Path = %q, want \"/\"", c.Path)
	}
	if c.MaxAge != int(flowTTL.Seconds()) {
		t.Errorf("Max-Age = %d, want %d — a login attempt must not outlive its window",
			c.MaxAge, int(flowTTL.Seconds()))
	}
}

// Every login attempt gets its own state, nonce and verifier. A value reused
// across logins would make the state parameter useless as a CSRF token.
func TestLoginIssuesFreshSecretsEveryTime(t *testing.T) {
	idp := newFakeIDP(t)
	a, sealer := newAuthenticator(t, idp, "HS256", "")

	states := map[string]bool{}
	nonces := map[string]bool{}
	verifiers := map[string]bool{}

	for i := 0; i < 20; i++ {
		_, c := startLogin(t, a, "")
		var flow flowState
		if err := sealer.Open(c.Value, &flow); err != nil {
			t.Fatal(err)
		}
		if states[flow.State] || nonces[flow.Nonce] || verifiers[flow.Verifier] {
			t.Fatal("a login attempt reused the state, nonce or PKCE verifier of an earlier " +
				"one; a predictable state parameter defeats the callback's CSRF check")
		}
		states[flow.State], nonces[flow.Nonce], verifiers[flow.Verifier] = true, true, true

		if len(flow.State) < 40 || len(flow.Nonce) < 40 {
			t.Fatalf("state %q or nonce %q is short enough to guess", flow.State, flow.Nonce)
		}
	}
}

// return_to is attacker-controlled — it is a query parameter on a URL anyone
// can send a member — so it is sanitised before it is sealed, not after it is
// read back.
func TestLoginSanitisesReturnToBeforeSealingIt(t *testing.T) {
	idp := newFakeIDP(t)
	a, sealer := newAuthenticator(t, idp, "HS256", "")

	tests := map[string]string{
		"/profil":                      "/profil",
		"/arrangementer?old=1":         "/arrangementer?old=1",
		"":                             "/",
		"//evil.example":               "/",
		"///evil.example":              "/",
		"https://evil.example/phish":   "/",
		"http://evil.example":          "/",
		"javascript:alert(1)":          "/",
		"//evil.example/%2e%2e":        "/",
		`\\evil.example`:               "/",
		"/\\evil.example":              "/%5Cevil.example",
		"data:text/html,<script>":      "/",
		"itemize://evil":               "/",
		"/redirect?to=https://evil.no": "/redirect?to=https://evil.no",
	}

	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			_, c := startLogin(t, a, in)
			var flow flowState
			if err := sealer.Open(c.Value, &flow); err != nil {
				t.Fatal(err)
			}
			if flow.ReturnTo != want {
				t.Errorf("return_to=%q was sealed as %q, want %q", in, flow.ReturnTo, want)
			}
			// The property that actually matters, whatever the exact string:
			// the visitor must never be sent to another origin. A URL sitting
			// in a query parameter is harmless — only the path itself decides
			// where the browser goes.
			if strings.HasPrefix(flow.ReturnTo, "//") {
				t.Errorf("return_to=%q produced %q, a protocol-relative URL browsers follow "+
					"off-site — the phishing page then wears our domain in the link the "+
					"member clicked", in, flow.ReturnTo)
			}
			if u, err := url.Parse(flow.ReturnTo); err != nil || u.IsAbs() || u.Host != "" {
				t.Errorf("return_to=%q produced %q, which points at another origin",
					in, flow.ReturnTo)
			}
		})
	}
}

// safeReturnTo is the whole of that defence, so it is worth testing directly as
// well as through Login, including inputs url.Parse rejects outright.
func TestSafeReturnToNeverLeavesTheSite(t *testing.T) {
	for _, in := range []string{
		"", "/", "//evil.example", "////evil.example", "https://evil.example",
		"//evil.example\\@itemize.no", "/\t//evil.example", "/\n//evil.example",
		"http:evil.example", "https:/evil.example", "/%2f%2fevil.example",
		"/..//evil.example", "\\/evil.example", "/@evil.example",
		strings.Repeat("/", 200) + "evil.example",
	} {
		t.Run(in, func(t *testing.T) {
			got := safeReturnTo(in)
			if !strings.HasPrefix(got, "/") {
				t.Fatalf("safeReturnTo(%q) = %q, which is not a path at all", in, got)
			}
			if strings.HasPrefix(got, "//") {
				t.Fatalf("safeReturnTo(%q) = %q, a protocol-relative URL that browsers "+
					"follow to another host", in, got)
			}
			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("safeReturnTo(%q) = %q, which is not parseable: %v", in, got, err)
			}
			if u.IsAbs() || u.Host != "" {
				t.Fatalf("safeReturnTo(%q) = %q, which points at %q", in, got, u.Host)
			}
		})
	}
}

// ── The callback, completing normally ─────────────────────────────────────

func TestCallbackCompletesTheLogin(t *testing.T) {
	idp := newFakeIDP(t)
	a, sealer := newAuthenticator(t, idp, "HS256", "")

	authURL, flow := startLogin(t, a, "/profil")
	idp.idToken = signClaims(t, testClientSecret, idp.claimsFor(authURL.Query().Get("nonce")))

	rec := callback(t, a, flow, url.Values{
		"state": {authURL.Query().Get("state")},
		"code":  {"the-authorization-code"},
	})

	if rec.Code != http.StatusFound {
		t.Fatalf("the callback returned %d rather than a redirect: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/profil" {
		t.Errorf("the member landed on %q rather than where they started", got)
	}

	sess := sessionFrom(t, sealer, rec)
	if sess == nil {
		t.Fatal("no readable session was established, so the member would arrive logged out")
	}
	if sess.ID != "11111111-2222-4333-8444-999999999999" {
		t.Errorf("sub = %q", sess.ID)
	}
	// Norwegian names are the common case here, not an exotic one.
	if sess.FullName != "Bjørn Ærlig Ødegård" || sess.Name != "Bjørn" {
		t.Errorf("the name claims were mangled: name=%q fullName=%q", sess.Name, sess.FullName)
	}
	if sess.Email != "bjorn@stud.ntnu.no" || sess.ImageURL != "https://auth.example/avatar.png" {
		t.Errorf("email or avatar was lost: %+v", sess.User)
	}
	if !sess.IsStyret() {
		t.Error("the roles claim did not reach the session, so a board member would arrive " +
			"without access to event administration")
	}
	if sess.Expires.After(time.Now().Add(maxSessionAge + time.Minute)) {
		t.Error("the session outlives our own cap")
	}
}

// The authorization code is exchanged with the PKCE verifier, not just the
// client secret. Without it a code intercepted from the redirect — in a browser
// history, a referrer header, a shared log — could be redeemed by anyone.
func TestCallbackRedeemsTheCodeWithThePKCEVerifier(t *testing.T) {
	idp := newFakeIDP(t)
	a, _ := newAuthenticator(t, idp, "HS256", "")

	authURL, flow := startLogin(t, a, "")
	idp.idToken = signClaims(t, testClientSecret, idp.claimsFor(authURL.Query().Get("nonce")))

	callback(t, a, flow, url.Values{
		"state": {authURL.Query().Get("state")},
		"code":  {"the-authorization-code"},
	})

	form := idp.lastTokenRequest(t)
	if got := form.Get("code"); got != "the-authorization-code" {
		t.Errorf("the provider was asked to redeem %q rather than the code from the callback", got)
	}
	if got := form.Get("grant_type"); got != "authorization_code" {
		t.Errorf("grant_type = %q, want authorization_code", got)
	}

	verifier := form.Get("code_verifier")
	if verifier == "" {
		t.Fatal("no code_verifier was sent, so PKCE provides no protection at all whatever " +
			"challenge was advertised")
	}
	sum := sha256.Sum256([]byte(verifier))
	if got, want := base64.RawURLEncoding.EncodeToString(sum[:]),
		authURL.Query().Get("code_challenge"); got != want {
		t.Errorf("the verifier sent to the token endpoint hashes to %q but the challenge "+
			"advertised at login was %q; the provider would refuse the exchange", got, want)
	}
}

// ── The callback, refusing ────────────────────────────────────────────────

// Every way a callback can be wrong, and the one property that matters for all
// of them: no session cookie is written. A refusal that still established a
// session would be worse than no check at all.
func TestCallbackRefusesEveryMalformedOrForgedCallback(t *testing.T) {
	tests := []struct {
		name string
		// mutate is given the honest query, the flow cookie and the claim set
		// the provider is about to sign, and breaks exactly one thing.
		mutate func(t *testing.T, idp *fakeIDP, q url.Values, claims map[string]any) *http.Cookie
		why    string
	}{
		{
			name: "a flow cookie that is not ours",
			mutate: func(_ *testing.T, _ *fakeIDP, _ url.Values, _ map[string]any) *http.Cookie {
				return &http.Cookie{Name: flowCookie, Value: "not-a-sealed-cookie"}
			},
			why: "only a cookie this server sealed may drive a login",
		},
		{
			name: "an expired flow",
			mutate: func(t *testing.T, _ *fakeIDP, q url.Values, _ map[string]any) *http.Cookie {
				sealer := newTestSealer(t, testSecret, true)
				sealed, err := sealer.Seal(flowState{
					State:   q.Get("state"),
					Expires: time.Now().Add(-time.Second),
				})
				if err != nil {
					t.Fatal(err)
				}
				return &http.Cookie{Name: flowCookie, Value: sealed}
			},
			why: "a login attempt left open past its window must be restarted",
		},
		{
			name: "the state does not match",
			mutate: func(_ *testing.T, _ *fakeIDP, q url.Values, _ map[string]any) *http.Cookie {
				q.Set("state", "a-state-the-attacker-chose")
				return nil // keep the honest cookie; replaced below
			},
			why: "state mismatch is login CSRF: an attacker completing their own login in " +
				"the member's browser, leaving the member signed in as the attacker",
		},
		{
			name: "no state at all",
			mutate: func(_ *testing.T, _ *fakeIDP, q url.Values, _ map[string]any) *http.Cookie {
				q.Del("state")
				return nil
			},
			why: "an absent state must not compare equal to the one in the cookie",
		},
		{
			name: "the token exchange fails",
			mutate: func(_ *testing.T, idp *fakeIDP, _ url.Values, _ map[string]any) *http.Cookie {
				idp.tokenStatus = http.StatusBadRequest
				return nil
			},
			why: "a code the provider will not redeem is not proof of anything",
		},
		{
			name: "the response carries no id_token",
			mutate: func(_ *testing.T, idp *fakeIDP, _ url.Values, _ map[string]any) *http.Cookie {
				idp.omitIDToken = true
				return nil
			},
			why: "an access token alone says nothing about who the visitor is",
		},
		{
			name: "the id_token is signed with the wrong key",
			mutate: func(t *testing.T, idp *fakeIDP, _ url.Values, claims map[string]any) *http.Cookie {
				idp.idToken = signClaims(t, "not-the-shared-secret-at-all", claims)
				return nil
			},
			why: "a token minted by anyone but the provider must be refused",
		},
		{
			name: "the id_token declares alg none",
			mutate: func(t *testing.T, idp *fakeIDP, _ url.Values, claims map[string]any) *http.Cookie {
				payload, err := json.Marshal(claims)
				if err != nil {
					t.Fatal(err)
				}
				idp.idToken = mintJWT(t, testClientSecret, hs256Header("none"), string(payload))
				return nil
			},
			why: "algorithm confusion must not survive the whole flow either",
		},
		{
			name: "the id_token is from another issuer",
			mutate: func(t *testing.T, idp *fakeIDP, _ url.Values, claims map[string]any) *http.Cookie {
				claims["iss"] = "https://evil.example"
				idp.idToken = signClaims(t, testClientSecret, claims)
				return nil
			},
			why: "the issuer is checked so a token from a different tenant cannot be replayed",
		},
		{
			name: "the id_token is addressed to another application",
			mutate: func(t *testing.T, idp *fakeIDP, _ url.Values, claims map[string]any) *http.Cookie {
				claims["aud"] = "the-wiki"
				idp.idToken = signClaims(t, testClientSecret, claims)
				return nil
			},
			why: "FusionAuth serves other applications on this tenant and their tokens must " +
				"not work here",
		},
		{
			name: "the id_token has expired",
			mutate: func(t *testing.T, idp *fakeIDP, _ url.Values, claims map[string]any) *http.Cookie {
				claims["exp"] = time.Now().Add(-time.Hour).Unix()
				idp.idToken = signClaims(t, testClientSecret, claims)
				return nil
			},
			why: "an expired token must not be exchangeable for a fresh session",
		},
		{
			name: "the id_token has no expiry",
			mutate: func(t *testing.T, idp *fakeIDP, _ url.Values, claims map[string]any) *http.Cookie {
				delete(claims, "exp")
				idp.idToken = signClaims(t, testClientSecret, claims)
				return nil
			},
			why: "a token with no expiry must not be treated as never expiring",
		},
		{
			name: "the nonce does not match this login",
			mutate: func(t *testing.T, idp *fakeIDP, _ url.Values, claims map[string]any) *http.Cookie {
				claims["nonce"] = "a-nonce-from-a-different-login"
				idp.idToken = signClaims(t, testClientSecret, claims)
				return nil
			},
			why: "the nonce binds the token to this attempt; without the check a token " +
				"captured from an earlier login could be injected into a new one",
		},
		{
			name: "the id_token carries no nonce",
			mutate: func(t *testing.T, idp *fakeIDP, _ url.Values, claims map[string]any) *http.Cookie {
				delete(claims, "nonce")
				idp.idToken = signClaims(t, testClientSecret, claims)
				return nil
			},
			why: "an absent nonce must not compare equal to the one we sent",
		},
		{
			name: "the roles claim is neither a string nor a list",
			mutate: func(t *testing.T, idp *fakeIDP, _ url.Values, claims map[string]any) *http.Cookie {
				claims["roles"] = map[string]any{"Styret": true}
				idp.idToken = signClaims(t, testClientSecret, claims)
				return nil
			},
			why: "a roles claim we cannot read must fail the login rather than silently " +
				"produce a member with no roles",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idp := newFakeIDP(t)
			a, sealer := newAuthenticator(t, idp, "HS256", "")

			authURL, flow := startLogin(t, a, "/profil")
			q := url.Values{
				"state": {authURL.Query().Get("state")},
				"code":  {"the-authorization-code"},
			}
			claims := idp.claimsFor(authURL.Query().Get("nonce"))
			idp.idToken = signClaims(t, testClientSecret, claims)

			if replacement := tt.mutate(t, idp, q, claims); replacement != nil {
				flow = replacement
			}

			rec := callback(t, a, flow, q)

			if sess := sessionFrom(t, sealer, rec); sess != nil {
				t.Fatalf("a session was established for %+v even though %s", sess.User, tt.why)
			}
			if rec.Code == http.StatusFound && rec.Header().Get("Location") == "/profil" {
				t.Errorf("the visitor was redirected to the page they asked for as though "+
					"the login had succeeded; %s", tt.why)
			}
		})
	}
}

// The "no flow cookie" case above cannot express itself through the mutate
// return value, so it gets its own test rather than a special case.
func TestCallbackWithoutAFlowCookieIsRefused(t *testing.T) {
	idp := newFakeIDP(t)
	a, sealer := newAuthenticator(t, idp, "HS256", "")

	rec := callback(t, a, nil, url.Values{"state": {"anything"}, "code": {"anything"}})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
	if sessionFrom(t, sealer, rec) != nil {
		t.Error("a callback with no flow cookie established a session")
	}
	if idp.tokenCalls != 0 {
		t.Error("the code was exchanged before the flow cookie was checked; an attacker " +
			"could make us call the provider without any login of ours in progress")
	}
}

// The provider declining — the member pressed cancel, or their account is
// locked — is not an error on our side. They go back to the front page rather
// than seeing a stack of Norwegian error text, and no session is created.
func TestCallbackHandlesTheProviderDeclining(t *testing.T) {
	idp := newFakeIDP(t)
	a, sealer := newAuthenticator(t, idp, "HS256", "")

	authURL, flow := startLogin(t, a, "/profil")
	rec := callback(t, a, flow, url.Values{
		"state": {authURL.Query().Get("state")},
		"error": {"access_denied"},
	})

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Errorf("got %d to %q, want a redirect to the front page",
			rec.Code, rec.Header().Get("Location"))
	}
	if sessionFrom(t, sealer, rec) != nil {
		t.Error("a declined login established a session")
	}
	if idp.tokenCalls != 0 {
		t.Error("a declined login still tried to redeem a code")
	}
}

// An error parameter must not be able to skip the state check. If it could, an
// attacker could send a member to /callback?error=x and consume their pending
// login without knowing the state.
func TestCallbackChecksStateBeforeHonouringTheErrorParameter(t *testing.T) {
	idp := newFakeIDP(t)
	a, sealer := newAuthenticator(t, idp, "HS256", "")

	_, flow := startLogin(t, a, "/profil")
	rec := callback(t, a, flow, url.Values{
		"state": {"the-wrong-state"},
		"error": {"access_denied"},
	})

	if rec.Code == http.StatusFound {
		t.Error("a callback with a mismatched state was accepted because it carried an " +
			"error parameter; the state check must come first")
	}
	if sessionFrom(t, sealer, rec) != nil {
		t.Error("a session was established")
	}
}

// One attempt per cookie. Whatever the outcome, the flow cookie is expired on
// the way out, so a leaked callback URL cannot be replayed against a still-live
// verifier.
func TestCallbackAlwaysExpiresTheFlowCookie(t *testing.T) {
	outcomes := map[string]func(t *testing.T, idp *fakeIDP, authURL *url.URL) url.Values{
		"success": func(t *testing.T, idp *fakeIDP, authURL *url.URL) url.Values {
			idp.idToken = signClaims(t, testClientSecret, idp.claimsFor(authURL.Query().Get("nonce")))
			return url.Values{"state": {authURL.Query().Get("state")}, "code": {"c"}}
		},
		"state mismatch": func(_ *testing.T, _ *fakeIDP, _ *url.URL) url.Values {
			return url.Values{"state": {"wrong"}, "code": {"c"}}
		},
		"provider declined": func(_ *testing.T, _ *fakeIDP, authURL *url.URL) url.Values {
			return url.Values{"state": {authURL.Query().Get("state")}, "error": {"access_denied"}}
		},
		"token exchange failed": func(_ *testing.T, idp *fakeIDP, authURL *url.URL) url.Values {
			idp.tokenStatus = http.StatusBadRequest
			return url.Values{"state": {authURL.Query().Get("state")}, "code": {"c"}}
		},
	}

	for name, build := range outcomes {
		t.Run(name, func(t *testing.T) {
			idp := newFakeIDP(t)
			a, _ := newAuthenticator(t, idp, "HS256", "")

			authURL, flow := startLogin(t, a, "/profil")
			rec := callback(t, a, flow, build(t, idp, authURL))

			var cleared *http.Cookie
			for _, c := range rec.Result().Cookies() {
				if c.Name == flowCookie {
					cleared = c
				}
			}
			if cleared == nil {
				t.Fatal("the flow cookie was not expired, so the state, nonce and PKCE " +
					"verifier stay live in the browser for another ten minutes")
			}
			if cleared.Value != "" || cleared.MaxAge >= 0 {
				t.Errorf("the flow cookie was reissued rather than deleted: value %q, "+
					"Max-Age %d", cleared.Value, cleared.MaxAge)
			}
		})
	}
}

// ── Verifier selection ────────────────────────────────────────────────────

// Configured for RS256, an HS256 token must be refused however it is signed:
// go-oidc looks for a key in the provider's JWKS, and there is no symmetric key
// there by definition. This is the branch that takes over once FusionAuth is
// switched away from HS256, and it must not fall back to the shared secret.
func TestRS256ConfigurationRefusesHS256Tokens(t *testing.T) {
	idp := newFakeIDP(t)
	a, sealer := newAuthenticator(t, idp, "RS256", "")

	authURL, flow := startLogin(t, a, "/profil")
	idp.idToken = signClaims(t, testClientSecret, idp.claimsFor(authURL.Query().Get("nonce")))

	rec := callback(t, a, flow, url.Values{
		"state": {authURL.Query().Get("state")},
		"code":  {"c"},
	})

	if sessionFrom(t, sealer, rec) != nil {
		t.Error("an HS256 token was accepted by a deployment configured for RS256; the " +
			"shared secret would still be able to mint sessions after the migration")
	}
}

// FusionAuth's HMAC key defaults to a separately generated value rather than
// the client secret the specification names. When one is configured it must be
// the key that is used, and the client secret must stop working — otherwise a
// misconfiguration would go unnoticed until the day the client secret rotates.
func TestTheConfiguredHMACSecretIsTheOneUsed(t *testing.T) {
	const hmacKey = "a-separately-generated-hmac-key-0"

	t.Run("signed with the configured HMAC key", func(t *testing.T) {
		idp := newFakeIDP(t)
		a, sealer := newAuthenticator(t, idp, "HS256", hmacKey)

		authURL, flow := startLogin(t, a, "/profil")
		idp.idToken = signClaims(t, hmacKey, idp.claimsFor(authURL.Query().Get("nonce")))

		rec := callback(t, a, flow, url.Values{
			"state": {authURL.Query().Get("state")}, "code": {"c"}})
		if sessionFrom(t, sealer, rec) == nil {
			t.Errorf("a token signed with the configured HMAC key was refused, so every "+
				"login would fail: %s", rec.Body.String())
		}
	})

	t.Run("signed with the client secret instead", func(t *testing.T) {
		idp := newFakeIDP(t)
		a, sealer := newAuthenticator(t, idp, "HS256", hmacKey)

		authURL, flow := startLogin(t, a, "/profil")
		idp.idToken = signClaims(t, testClientSecret, idp.claimsFor(authURL.Query().Get("nonce")))

		rec := callback(t, a, flow, url.Values{
			"state": {authURL.Query().Get("state")}, "code": {"c"}})
		if sessionFrom(t, sealer, rec) != nil {
			t.Error("the client secret still verified tokens while a separate HMAC key was " +
				"configured; the wrong key is being used")
		}
	})
}

// ── Logout and routing ────────────────────────────────────────────────────

func TestLogoutClearsTheSessionAndReturnsHome(t *testing.T) {
	idp := newFakeIDP(t)
	a, sealer := newAuthenticator(t, idp, "HS256", "")

	sealed, err := sealer.Seal(NewSession(User{ID: "fa-1", Roles: []string{RoleStyret}},
		time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodPost, "/logout", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookie, Value: sealed})

	rec := httptest.NewRecorder()
	a.Logout(rec, r)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Errorf("got %d to %q, want a redirect to the front page",
			rec.Code, rec.Header().Get("Location"))
	}

	names := map[string]*http.Cookie{}
	for _, c := range rec.Result().Cookies() {
		names[c.Name] = c
	}
	for _, name := range []string{SessionCookie, LegacySessionCookie} {
		c, ok := names[name]
		if !ok {
			t.Errorf("logout left %q in place; the member would still be signed in", name)
			continue
		}
		if c.Value != "" || c.MaxAge >= 0 {
			t.Errorf("%q was reissued rather than deleted", name)
		}
	}
	if sessionFrom(t, sealer, rec) != nil {
		t.Error("a session survived the logout")
	}
}

// The routes are mounted explicitly here rather than by OIDC middleware, so a
// typo would present as a 404 on login with nothing else to catch it. Logout
// answers both methods: the header uses a form POST, but a plain link must work
// without JavaScript too.
func TestRoutesAreMounted(t *testing.T) {
	idp := newFakeIDP(t)
	a, _ := newAuthenticator(t, idp, "HS256", "")

	mux := http.NewServeMux()
	a.Routes(mux)

	for _, tt := range []struct{ method, path string }{
		{http.MethodGet, "/login"},
		{http.MethodGet, "/callback"},
		{http.MethodGet, "/logout"},
		{http.MethodPost, "/logout"},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, nil)
			_, pattern := mux.Handler(r)
			if pattern == "" {
				t.Errorf("%s %s is not routed anywhere", tt.method, tt.path)
			}
		})
	}
}

// ── Startup ───────────────────────────────────────────────────────────────

// Discovery failure is fatal on purpose: a deployment that comes up unable to
// authenticate anybody should not be mistaken for a healthy one. The retry loop
// must still honour a cancelled context rather than sleeping through a
// shutdown.
func TestNewGivesUpWhenTheContextIsCancelled(t *testing.T) {
	idp := newFakeIDP(t)

	host, err := url.Parse(idp.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	base, err := url.Parse(testBaseURL)
	if err != nil {
		t.Fatal(err)
	}
	sealer := newTestSealer(t, testSecret, true)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		_, err := New(ctx, &config.Config{
			BaseURL: base,
			FusionAuth: config.FusionAuth{
				Host: host, ClientID: testClientID, ClientSecret: testClientSecret,
				IDTokenAlg: "HS256",
			},
		}, sealer, nullLogger())
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("New succeeded against a cancelled context")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("New did not return on a cancelled context; startup would hang through the " +
			"whole retry schedule instead of shutting down")
	}
}

// ── Claim mapping ─────────────────────────────────────────────────────────

// verifiedToken turns a claim set into the *oidc.IDToken userFromToken expects,
// going through the real verifier so the claim bytes are exactly what the flow
// would hand it.
func verifiedToken(t *testing.T, claims map[string]any) *oidc.IDToken {
	t.Helper()

	const issuer = "https://auth.example"
	claims["iss"] = issuer
	claims["aud"] = testClientID
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().Add(time.Hour).Unix()
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	verifier := oidc.NewVerifier(issuer, hmacKeySet{secret: []byte(hmacSecret)}, &oidc.Config{
		ClientID:             testClientID,
		SupportedSigningAlgs: []string{"HS256"},
	})
	idToken, err := verifier.Verify(context.Background(),
		mintJWT(t, hmacSecret, hs256Header("HS256"), string(payload)))
	if err != nil {
		t.Fatalf("building a token for the claim-mapping test: %v", err)
	}
	return idToken
}

// The non-standard claims — roles, fullName, imageUrl — come from FusionAuth's
// ID token populate lambda. When that lambda is missing or partial the claims
// simply are not there, and the site has to keep working rather than fail the
// login: a member with no roles is a normal state, not an error.
func TestUserFromTokenMapsClaims(t *testing.T) {
	a := &Authenticator{log: nullLogger()}

	tests := []struct {
		name    string
		claims  map[string]any
		want    User
		wantErr bool
		why     string
	}{
		{
			name: "everything present",
			claims: map[string]any{
				"sub": "fa-1", "name": "Bjørn", "fullName": "Bjørn Ærlig Ødegård",
				"email": "bjorn@stud.ntnu.no", "imageUrl": "https://a/b.png",
				"roles": []string{"Medlem", RoleStyret},
			},
			want: User{ID: "fa-1", Name: "Bjørn", FullName: "Bjørn Ærlig Ødegård",
				Email: "bjorn@stud.ntnu.no", ImageURL: "https://a/b.png",
				Roles: []string{"Medlem", RoleStyret}},
			why: "the ordinary case, with a Norwegian name",
		},
		{
			name:   "only the standard claims",
			claims: map[string]any{"sub": "fa-1", "email": "kari@example.no"},
			want:   User{ID: "fa-1", Email: "kari@example.no"},
			why: "without the populate lambda only sub and email arrive; the login must " +
				"still succeed and the interface fall back to the email as a name",
		},
		{
			name:   "a single role as a bare string",
			claims: map[string]any{"sub": "fa-1", "roles": RoleStyret},
			want:   User{ID: "fa-1", Roles: []string{RoleStyret}},
			why: "some lambda configurations emit one role as a plain string; a []string " +
				"field alone would drop it and the board would lose access",
		},
		{
			name:   "an empty roles list",
			claims: map[string]any{"sub": "fa-1", "roles": []string{}},
			want:   User{ID: "fa-1"},
			why:    "no roles is a normal state for a member",
		},
		{
			name:   "an empty roles string",
			claims: map[string]any{"sub": "fa-1", "roles": ""},
			want:   User{ID: "fa-1"},
			why:    "an empty string must not become a role named the empty string",
		},
		{
			name:   "a role containing Norwegian letters",
			claims: map[string]any{"sub": "fa-1", "roles": []string{"Økonomi"}},
			want:   User{ID: "fa-1", Roles: []string{"Økonomi"}},
			why:    "role names are free text in FusionAuth and are often Norwegian",
		},
		{
			name:    "roles as an object",
			claims:  map[string]any{"sub": "fa-1", "roles": map[string]any{"Styret": true}},
			wantErr: true,
			why: "a shape we cannot read must fail the login rather than quietly produce a " +
				"member with no roles, which presents as a permissions bug",
		},
		{
			name:    "roles as a number",
			claims:  map[string]any{"sub": "fa-1", "roles": 7},
			wantErr: true,
			why:     "same",
		},
		{
			name:   "no subject",
			claims: map[string]any{"email": "kari@example.no"},
			want:   User{Email: "kari@example.no"},
			why: "recorded rather than endorsed: an ID token with no sub yields a user " +
				"with an empty ID, and every FusionAuth call keyed on it will fail later",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := a.userFromToken(verifiedToken(t, tt.claims))

			if tt.wantErr {
				if err == nil {
					t.Fatalf("no error, and the login would proceed as %+v — %s", got, tt.why)
				}
				return
			}
			if err != nil {
				t.Fatalf("the login failed on a claim set that must work (%s): %v", tt.why, err)
			}
			if got.ID != tt.want.ID || got.Name != tt.want.Name || got.FullName != tt.want.FullName ||
				got.Email != tt.want.Email || got.ImageURL != tt.want.ImageURL {
				t.Errorf("got %+v, want %+v — %s", got, tt.want, tt.why)
			}
			if len(got.Roles) != len(tt.want.Roles) {
				t.Fatalf("got roles %v, want %v — %s", got.Roles, tt.want.Roles, tt.why)
			}
			for i := range got.Roles {
				if got.Roles[i] != tt.want.Roles[i] {
					t.Errorf("got roles %v, want %v", got.Roles, tt.want.Roles)
				}
			}
		})
	}
}

// parseRoles rejects shapes it cannot read rather than returning an empty list,
// so a lambda change shows up as a failed login rather than as a board that
// quietly lost its permissions.
func TestParseRolesRejectsUnreadableShapes(t *testing.T) {
	for _, raw := range []string{`{}`, `{"Styret":true}`, `7`, `true`, `[1,2]`, `["ok",2]`} {
		t.Run(raw, func(t *testing.T) {
			if got, err := parseRoles(json.RawMessage(raw)); err == nil {
				t.Errorf("parseRoles(%s) = %v with no error; an unreadable roles claim must "+
					"fail the login rather than present later as a permissions bug", raw, got)
			}
		})
	}

	// JSON null is the one non-list, non-string value that is not an error:
	// it unmarshals into a nil []string, which is the same as having no roles.
	if got, err := parseRoles(json.RawMessage(`null`)); err != nil || got != nil {
		t.Errorf("parseRoles(null) = %v, %v; a null roles claim means no roles", got, err)
	}
}
