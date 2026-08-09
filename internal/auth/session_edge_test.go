package auth

// Edge cases for the sealed session cookie: the attributes it goes out with,
// what happens to a cookie that has been cut about, and the wire format that
// every already-issued cookie in the wild depends on.

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestSealer builds a sealer and restores the package-level secureCookies
// flag afterwards. NewSealer sets that global as a side effect, so without this
// the CSRF tests would see whichever value the last session test happened to
// leave behind.
func newTestSealer(t *testing.T, secret string, secure bool) *Sealer {
	t.Helper()
	previous := secureCookies
	t.Cleanup(func() { SetSecureCookies(previous) })

	s, err := NewSealer(secret, secure)
	if err != nil {
		t.Fatalf("building a sealer: %v", err)
	}
	return s
}

// cookieNamed returns the cookie set on the response, failing when it is
// absent.
func cookieNamed(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no %q cookie on the response; the visitor's browser would be left "+
		"with whatever it had before", name)
	return nil
}

// ── Cookie attributes ─────────────────────────────────────────────────────

// Each attribute here is load-bearing, and losing any one of them is a security
// regression that no functional test would notice:
//
//   - HttpOnly keeps the cookie out of reach of injected script, so an XSS bug
//     cannot be escalated into session theft.
//   - Secure keeps it off plaintext requests to the same host.
//   - SameSite=Lax is the primary CSRF defence; the double-submit token in
//     csrf.go is only the backup.
//   - Path=/ is what makes one session cover the whole site.
func TestSessionCookieCarriesItsSecurityAttributes(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)
	expires := time.Now().Add(2 * time.Hour)

	rec := httptest.NewRecorder()
	if err := sealer.Write(rec, NewSession(User{ID: "fa-1"}, expires)); err != nil {
		t.Fatal(err)
	}
	c := cookieNamed(t, rec, SessionCookie)

	if !c.HttpOnly {
		t.Error("the session cookie is no longer HttpOnly, so any script injected into " +
			"a page can read it and impersonate the member")
	}
	if !c.Secure {
		t.Error("the session cookie is no longer Secure, so it will be sent over plain " +
			"HTTP to the same host and can be captured in transit")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite is %v, want Lax; Strict breaks the redirect back from "+
			"FusionAuth and None removes the main CSRF defence", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("Path is %q, want \"/\"; a narrower path leaves the member signed out "+
			"on most of the site", c.Path)
	}
	if c.MaxAge <= 0 {
		t.Errorf("Max-Age is %d; a non-positive value tells the browser to discard the "+
			"cookie immediately", c.MaxAge)
	}
	if c.MaxAge > int(maxSessionAge.Seconds()) {
		t.Errorf("Max-Age is %d, beyond the %v cap the session itself enforces",
			c.MaxAge, maxSessionAge)
	}
	if c.Expires.IsZero() {
		t.Error("no Expires attribute, so browsers that ignore Max-Age treat this as a " +
			"session cookie")
	}
}

// Development runs over plain HTTP, where a Secure cookie is simply never
// stored — the site would appear to accept a login and then immediately forget
// it. The flag must therefore follow the deployment, not be hardcoded.
func TestSessionCookieDropsSecureForPlainHTTPDevelopment(t *testing.T) {
	sealer := newTestSealer(t, testSecret, false)

	rec := httptest.NewRecorder()
	if err := sealer.Write(rec, NewSession(User{ID: "fa-1"}, time.Now().Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	if cookieNamed(t, rec, SessionCookie).Secure {
		t.Error("a development sealer marked the cookie Secure; a browser on http://localhost " +
			"discards it and login silently never sticks")
	}

	// NewSealer also has to tell the CSRF cookie which way to go, because that
	// one cannot work it out from a request.
	if secureCookies {
		t.Error("NewSealer(…, false) left secureCookies true, so the CSRF cookie would be " +
			"dropped in development and every form post would 403")
	}
}

// Logging out has to remove the cookie the browser actually holds, and the one
// the previous Sapper site left behind, or the jar keeps a stale credential.
func TestClearExpiresBothSessionCookies(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)

	rec := httptest.NewRecorder()
	sealer.Clear(rec)

	for _, name := range []string{SessionCookie, LegacySessionCookie} {
		t.Run(name, func(t *testing.T) {
			c := cookieNamed(t, rec, name)
			if c.Value != "" {
				t.Errorf("cleared cookie still carries a value %q", c.Value)
			}
			if c.MaxAge >= 0 {
				t.Errorf("Max-Age is %d; only a negative value deletes the cookie", c.MaxAge)
			}
			if c.Path != "/" {
				t.Errorf("Path is %q; a deletion only matches a cookie set on the same path, "+
					"so the session would survive the logout", c.Path)
			}
			if !c.HttpOnly {
				t.Error("the deletion is not HttpOnly, which does not match the cookie being deleted")
			}
			if c.Secure != sealer.secure {
				t.Error("the deletion's Secure flag does not match the cookie being deleted")
			}
		})
	}
}

// ── Round trip through a real request ─────────────────────────────────────

// The whole point of the sealed cookie is that it survives the browser. This
// takes what Write emits and feeds it back through Read the way a browser
// would, rather than calling Seal and Open directly.
func TestSessionSurvivesTheBrowserRoundTrip(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)

	want := NewSession(User{
		ID:       "fa-1",
		Name:     "Kari",
		FullName: "Kari Nordmann",
		Email:    "kari@example.no",
		ImageURL: "https://itemize.no/kari.png",
		Roles:    []string{"Medlem", RoleStyret},
	}, time.Now().Add(time.Hour))

	rec := httptest.NewRecorder()
	if err := sealer.Write(rec, want); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		r.AddCookie(c)
	}

	got := sealer.Read(r)
	if got == nil {
		t.Fatal("a session written a moment ago could not be read back, so nobody would " +
			"ever stay logged in")
	}
	if got.ID != want.ID || got.Name != want.Name || got.FullName != want.FullName ||
		got.Email != want.Email || got.ImageURL != want.ImageURL {
		t.Errorf("fields were lost in the round trip: %+v", got.User)
	}
	if !got.IsStyret() {
		t.Error("roles were lost in the round trip, so a board member would arrive without " +
			"access to event administration")
	}
}

// A sealed value goes straight into a Set-Cookie header. net/http silently
// refuses to emit a cookie whose value contains a character not allowed there,
// which would present as "login does nothing" with no error anywhere.
func TestSealedValueIsSafeInACookieHeader(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)

	for i := 0; i < 50; i++ {
		sealed, err := sealer.Seal(NewSession(User{
			ID:       "fa-1",
			FullName: "Bjørn Ærlig Ødegård",
		}, time.Now().Add(time.Hour)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.ContainsAny(sealed, ` ",;\`) {
			t.Fatalf("sealed value %q contains a character net/http will not put in a "+
				"Set-Cookie header", sealed)
		}
	}
}

// Every Seal must use a fresh nonce. Reusing a GCM nonce under the same key
// breaks the cipher outright, and identical ciphertexts would also let anyone
// watching two requests tell that they carry the same session.
func TestSealUsesAFreshNonceEveryTime(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)
	sess := NewSession(User{ID: "fa-1"}, time.Now().Add(time.Hour))

	seen := make(map[string]bool, 64)
	for i := 0; i < 64; i++ {
		sealed, err := sealer.Seal(sess)
		if err != nil {
			t.Fatal(err)
		}
		if seen[sealed] {
			t.Fatal("two seals of the same session produced identical ciphertext, so the " +
				"GCM nonce is being reused — that breaks the cipher, not just privacy")
		}
		seen[sealed] = true
	}
}

// ── Damaged and forged cookies ────────────────────────────────────────────

// Truncation is what a proxy, a cookie-size limit or a careless copy-paste
// produces. Every prefix of a valid cookie must fail to open; none may decrypt
// into a partial session.
func TestTruncatedCookieNeverOpens(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)

	sealed, err := sealer.Seal(NewSession(User{ID: "fa-1", Roles: []string{RoleStyret}},
		time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}

	for n := 0; n < len(sealed); n++ {
		var got Session
		if err := sealer.Open(sealed[:n], &got); err == nil {
			t.Fatalf("a cookie truncated to %d of %d characters still opened, yielding %+v",
				n, len(sealed), got)
		}
	}
}

// The same for a cookie with characters appended: GCM must authenticate the
// whole of what it is given.
func TestExtendedCookieNeverOpens(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)

	sealed, err := sealer.Seal(NewSession(User{ID: "fa-1"}, time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"A", "AAAA", strings.Repeat("A", 64)} {
		var got Session
		if err := sealer.Open(sealed+suffix, &got); err == nil {
			t.Errorf("a cookie with %d extra characters appended still opened", len(suffix))
		}
	}
}

// Flipping a bit anywhere — nonce or ciphertext — must be caught. Sampling the
// whole range rather than only the last byte, which is all the existing
// tampering test covers.
func TestEveryByteOfTheCookieIsAuthenticated(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)

	sealed, err := sealer.Seal(NewSession(User{ID: "fa-1", Roles: []string{"Medlem"}},
		time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil {
		t.Fatal(err)
	}

	for i := range raw {
		mutated := append([]byte(nil), raw...)
		mutated[i] ^= 0x01

		var got Session
		if err := sealer.Open(base64.RawURLEncoding.EncodeToString(mutated), &got); err == nil {
			t.Fatalf("flipping bit 0 of byte %d of %d produced a cookie that still opened, "+
				"as %+v — the cookie is not fully authenticated", i, len(raw), got)
		}
	}
}

// Read must treat anything it cannot make sense of as "no session" rather than
// as an error the caller has to handle, because Inject relies on that to keep
// a broken cookie from failing a request.
func TestReadReturnsNilForAnythingUnusable(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)
	other := newTestSealer(t, "ffffffffffffffffffffffffffffffff", true)

	expired, err := sealer.Seal(&Session{User: User{ID: "fa-1"}, Expires: time.Now().Add(-time.Nanosecond)})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := other.Seal(NewSession(User{ID: "fa-1"}, time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		cookie *http.Cookie
		why    string
	}{
		{"no cookie at all", nil, "an anonymous visitor"},
		{"the wrong cookie name", &http.Cookie{Name: "something_else", Value: foreign},
			"only our own cookie may be read"},
		{"an empty value", &http.Cookie{Name: SessionCookie, Value: ""},
			"an empty cookie is not a session"},
		{"not base64", &http.Cookie{Name: SessionCookie, Value: "!!!not base64!!!"},
			"garbage must not reach the cipher"},
		{"shorter than a nonce", &http.Cookie{Name: SessionCookie, Value: "YWJj"},
			"a value too short to contain a nonce must be rejected before slicing it"},
		{"sealed under another key", &http.Cookie{Name: SessionCookie, Value: foreign},
			"rotating the secret must invalidate every existing session"},
		{"expired one nanosecond ago", &http.Cookie{Name: SessionCookie, Value: expired},
			"the expiry boundary must be enforced on read, not only on write"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.cookie != nil {
				r.AddCookie(tt.cookie)
			}
			if got := sealer.Read(r); got != nil {
				t.Errorf("Read accepted %s and returned %+v — %s", tt.name, got.User, tt.why)
			}
		})
	}
}

// A session sealed correctly but holding something that is not a Session must
// fail rather than yield a zero-valued user, which would read as "signed in as
// nobody".
func TestOpenRejectsAPayloadOfTheWrongShape(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)

	sealed, err := sealer.Seal("just a string")
	if err != nil {
		t.Fatal(err)
	}
	var got Session
	if err := sealer.Open(sealed, &got); err == nil {
		t.Error("a sealed JSON string opened into a Session; a cookie from a different " +
			"code path must not silently become a session")
	}
}

func TestSealReportsValuesItCannotEncode(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)
	if _, err := sealer.Seal(func() {}); err == nil {
		t.Error("Seal accepted a value encoding/json cannot marshal; the caller would go " +
			"on to set an empty cookie")
	}
}

// ── Wire format ───────────────────────────────────────────────────────────

// The JSON keys are the wire format of every cookie already in a member's
// browser. Renaming a field does not invalidate those cookies — they still
// decrypt — it just decodes them into a User with an empty ID and no roles.
// The member stays "logged in" as nobody, which is worse than being logged out.
func TestSessionJSONKeysAreFrozen(t *testing.T) {
	raw, err := json.Marshal(&Session{
		User: User{
			ID: "fa-1", Name: "Kari", FullName: "Kari Nordmann",
			Email: "kari@example.no", ImageURL: "https://itemize.no/k.png",
			Roles: []string{RoleStyret},
		},
		Issued:  time.Unix(0, 0).UTC(),
		Expires: time.Unix(0, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}

	want := []string{"sub", "n", "fn", "e", "img", "r", "iat", "exp"}
	for _, key := range want {
		if _, ok := decoded[key]; !ok {
			t.Errorf("the sealed session no longer contains the key %q; every cookie already "+
				"issued would decode into a user with that field empty", key)
		}
	}
	if len(decoded) != len(want) {
		t.Errorf("the sealed session now has %d keys, want %d (%v); adding fields grows every "+
			"cookie, and a credential added here would leak with the cookie",
			len(decoded), len(want), decoded)
	}
}

// Browsers refuse cookies over roughly 4096 bytes, and drop them without
// telling anyone. A realistic session must stay comfortably inside that, which
// is the practical argument for keeping tokens out of Session.
func TestARealisticSessionFitsInACookie(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)

	sealed, err := sealer.Seal(NewSession(User{
		ID:       "11111111-2222-4333-8444-999999999999",
		Name:     "Bjørn",
		FullName: "Bjørn Ærlig Ødegård-Sørensen",
		Email:    "bjorn.aerlig.odegard-sorensen@stud.ntnu.no",
		ImageURL: "https://auth.itemize.no/api/user/profile-picture/11111111-2222-4333-8444-999999999999",
		Roles:    []string{"Medlem", RoleStyret, "Arrangement", "Infra"},
	}, time.Now().Add(maxSessionAge)))
	if err != nil {
		t.Fatal(err)
	}

	const browserCookieLimit = 4096
	if len(sealed) > browserCookieLimit/2 {
		t.Errorf("a realistic session seals to %d bytes, more than half the %d-byte cookie "+
			"limit browsers enforce; something large has been added to Session and login "+
			"will start failing silently for members with longer names",
			len(sealed), browserCookieLimit)
	}
}

// Nothing in the sealer caps the size of what it seals — the limit lives in the
// browser. A session that is too large still round-trips here, so this failure
// mode cannot be caught by the server and has to be prevented by keeping
// Session small (the test above).
func TestOversizedSessionsStillRoundTripServerSide(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)

	huge := NewSession(User{ID: "fa-1", FullName: strings.Repeat("æ", 8000)},
		time.Now().Add(time.Hour))
	sealed, err := sealer.Seal(huge)
	if err != nil {
		t.Fatalf("sealing a large session failed: %v", err)
	}
	var got Session
	if err := sealer.Open(sealed, &got); err != nil {
		t.Fatalf("a large session did not survive the round trip: %v", err)
	}
	if got.FullName != huge.FullName {
		t.Error("a large session came back altered")
	}
}

// ── Lifetime ──────────────────────────────────────────────────────────────

func TestNewSessionClampsTheLifetime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		expires time.Time
		wantCap bool
		why     string
	}{
		{"an hour from now", now.Add(time.Hour), false,
			"a short-lived ID token must keep its own expiry"},
		{"exactly at the cap", now.Add(maxSessionAge), false,
			"the boundary itself is allowed"},
		{"a year from now", now.Add(365 * 24 * time.Hour), true,
			"the provider must not be able to grant a session longer than our cap"},
		{"the zero time", time.Time{}, true,
			"an ID token with no expiry must fall back to the cap, not to the zero time, " +
				"which would be instantly expired"},
		{"in the past", now.Add(-time.Hour), false,
			"an already-expired token must not be silently extended to the cap"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewSession(User{ID: "fa-1"}, tt.expires)

			limit := time.Now().Add(maxSessionAge)
			if got.Expires.After(limit.Add(time.Second)) {
				t.Errorf("expiry %v exceeds the %v cap: %s", got.Expires, maxSessionAge, tt.why)
			}
			if tt.wantCap && got.Expires.Before(limit.Add(-time.Minute)) {
				t.Errorf("expiry %v was not raised to the cap: %s", got.Expires, tt.why)
			}
			if !tt.wantCap && !got.Expires.Equal(tt.expires) {
				t.Errorf("expiry %v was changed from %v: %s", got.Expires, tt.expires, tt.why)
			}
			if got.Issued.IsZero() {
				t.Error("Issued was left at the zero time")
			}
		})
	}
}

// The boundary is checked with After, so a session expiring in the future is
// live and one that expired a nanosecond ago is not. Written with explicit
// offsets rather than sleeps so it cannot become flaky.
func TestExpiredBoundary(t *testing.T) {
	tests := map[string]struct {
		offset time.Duration
		want   bool
	}{
		"a nanosecond ago": {-time.Nanosecond, true},
		"a minute ago":     {-time.Minute, true},
		"a minute ahead":   {time.Minute, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := &Session{Expires: time.Now().Add(tt.offset)}
			if got := s.Expired(); got != tt.want {
				t.Errorf("a session expiring %s reported Expired() = %v", name, got)
			}
		})
	}
	// The zero time is in the past, so a session with no expiry set at all is
	// dead rather than immortal. That is the safe direction, and worth pinning.
	if !(&Session{}).Expired() {
		t.Error("a session with no expiry set counted as live, which would make a " +
			"malformed cookie into a session that never ends")
	}
}

// Writing a session that has already expired must not produce a cookie the
// browser will hold on to.
func TestWritingAnExpiredSessionEmitsADeletion(t *testing.T) {
	sealer := newTestSealer(t, testSecret, true)

	rec := httptest.NewRecorder()
	if err := sealer.Write(rec, &Session{User: User{ID: "fa-1"}, Expires: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if c := cookieNamed(t, rec, SessionCookie); c.MaxAge >= 0 {
		t.Errorf("Max-Age is %d for an already-expired session; the browser would keep a "+
			"cookie the server will refuse", c.MaxAge)
	}
}
