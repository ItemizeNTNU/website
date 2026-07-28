package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testSecret = "0123456789abcdef0123456789abcdef"

// ── Session sealing ───────────────────────────────────────────────────────

func TestSessionRoundTrip(t *testing.T) {
	sealer, err := NewSealer(testSecret, true)
	if err != nil {
		t.Fatal(err)
	}

	want := NewSession(User{
		ID: "fa-1", Name: "Kari", FullName: "Kari Nordmann",
		Email: "kari@example.no", Roles: []string{RoleStyret},
	}, time.Now().Add(time.Hour))

	sealed, err := sealer.Seal(want)
	if err != nil {
		t.Fatal(err)
	}

	var got Session
	if err := sealer.Open(sealed, &got); err != nil {
		t.Fatalf("opening a cookie we just sealed: %v", err)
	}
	if got.ID != want.ID || got.FullName != want.FullName || !got.IsStyret() {
		t.Errorf("session did not survive the round trip: %+v", got)
	}
}

// A cookie sealed under one key must not open under another, or rotating the
// secret would leave every existing session valid.
func TestSessionRejectsWrongKey(t *testing.T) {
	a, _ := NewSealer(testSecret, true)
	b, _ := NewSealer("ffffffffffffffffffffffffffffffff", true)

	sealed, err := a.Seal(NewSession(User{ID: "fa-1"}, time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	var got Session
	if err := b.Open(sealed, &got); err == nil {
		t.Error("a cookie sealed with a different key was accepted")
	}
}

// GCM authenticates as well as encrypts, so a modified cookie must fail to
// open rather than decoding into something unexpected.
func TestSessionRejectsTampering(t *testing.T) {
	sealer, _ := NewSealer(testSecret, true)
	sealed, err := sealer.Seal(NewSession(User{ID: "fa-1"}, time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}

	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xFF
	tampered := base64.RawURLEncoding.EncodeToString(raw)

	var got Session
	if err := sealer.Open(tampered, &got); err == nil {
		t.Error("a tampered cookie was accepted")
	}
}

func TestSessionRejectsGarbage(t *testing.T) {
	sealer, _ := NewSealer(testSecret, true)
	for _, value := range []string{"", "not-base64!!", "c2hvcnQ"} {
		var got Session
		if err := sealer.Open(value, &got); err == nil {
			t.Errorf("%q was accepted as a session", value)
		}
	}
}

func TestExpiredSessionIsIgnored(t *testing.T) {
	sealer, _ := NewSealer(testSecret, true)

	expired := &Session{User: User{ID: "fa-1"}, Expires: time.Now().Add(-time.Minute)}
	sealed, err := sealer.Seal(expired)
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookie, Value: sealed})
	if sealer.Read(r) != nil {
		t.Error("an expired session was accepted")
	}
}

// The identity provider's expiry must not be able to outlive our own cap.
func TestSessionLifetimeIsCapped(t *testing.T) {
	sess := NewSession(User{ID: "fa-1"}, time.Now().Add(365*24*time.Hour))
	if sess.Expires.After(time.Now().Add(maxSessionAge + time.Minute)) {
		t.Errorf("session expiry %v exceeds the cap", sess.Expires)
	}
}

// ── HS256 verification ────────────────────────────────────────────────────

func signHS256(t *testing.T, secret, alg, payload string) string {
	t.Helper()
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"` + alg + `","typ":"JWT"}`))
	body := enc.EncodeToString([]byte(payload))

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(header + "." + body))
	return header + "." + body + "." + enc.EncodeToString(mac.Sum(nil))
}

func TestHMACVerifierAcceptsValidToken(t *testing.T) {
	ks := hmacKeySet{secret: []byte(testSecret)}
	token := signHS256(t, testSecret, "HS256", `{"sub":"fa-1"}`)

	payload, err := ks.VerifySignature(context.Background(), token)
	if err != nil {
		t.Fatalf("a correctly signed token was rejected: %v", err)
	}
	if !strings.Contains(string(payload), `"sub":"fa-1"`) {
		t.Errorf("payload = %s", payload)
	}
}

func TestHMACVerifierRejectsBadSignature(t *testing.T) {
	ks := hmacKeySet{secret: []byte(testSecret)}
	token := signHS256(t, "the-wrong-secret", "HS256", `{"sub":"fa-1"}`)

	if _, err := ks.VerifySignature(context.Background(), token); err == nil {
		t.Error("a token signed with the wrong secret was accepted")
	}
}

// Algorithm confusion is the attack this verifier exists to resist: a token
// declaring alg=none, or an RS256 token re-signed as HS256, must be refused
// rather than dispatched on its own header.
func TestHMACVerifierRejectsOtherAlgorithms(t *testing.T) {
	ks := hmacKeySet{secret: []byte(testSecret)}

	for _, alg := range []string{"none", "NONE", "RS256", "HS384", "hs256"} {
		t.Run(alg, func(t *testing.T) {
			token := signHS256(t, testSecret, alg, `{"sub":"attacker"}`)
			if _, err := ks.VerifySignature(context.Background(), token); err == nil {
				t.Errorf("a token declaring alg=%q was accepted", alg)
			}
		})
	}
}

func TestHMACVerifierRejectsMalformedTokens(t *testing.T) {
	ks := hmacKeySet{secret: []byte(testSecret)}

	for _, token := range []string{"", "one.two", "a.b.c.d", "!!!.???.###"} {
		if _, err := ks.VerifySignature(context.Background(), token); err == nil {
			t.Errorf("malformed token %q was accepted", token)
		}
	}
}

// ── Roles ─────────────────────────────────────────────────────────────────

// FusionAuth emits a single role as a bare string under some lambda
// configurations; a []string field alone would reject that outright and leave
// the member with no roles at all.
func TestParseRoles(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"a list", `["Styret","Medlem"]`, []string{"Styret", "Medlem"}},
		{"a bare string", `"Styret"`, []string{"Styret"}},
		{"an empty list", `[]`, nil},
		{"an empty string", `""`, nil},
		{"absent", ``, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRoles(json.RawMessage(tt.raw))
			if err != nil {
				t.Fatalf("parseRoles(%s): %v", tt.raw, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestHasRole(t *testing.T) {
	var anonymous *User
	if anonymous.IsStyret() {
		t.Error("a nil user must not hold any role")
	}
	member := &User{Roles: []string{"Medlem"}}
	if member.IsStyret() {
		t.Error("a member without the role must not pass as Styret")
	}
	board := &User{Roles: []string{"Medlem", RoleStyret}}
	if !board.IsStyret() {
		t.Error("a board member must pass as Styret")
	}
}

// ── Gating ────────────────────────────────────────────────────────────────

func TestGatingMatrix(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	deny := func(w http.ResponseWriter, r *http.Request, status int) { w.WriteHeader(status) }

	anonymous := (*User)(nil)
	member := &User{ID: "fa-1", Roles: []string{"Medlem"}}
	board := &User{ID: "fa-2", Roles: []string{RoleStyret}}

	tests := []struct {
		name    string
		handler http.Handler
		user    *User
		want    int
	}{
		{"login page, anonymous", RequireLogin(ok), anonymous, http.StatusFound},
		{"login page, member", RequireLogin(ok), member, http.StatusOK},
		{"board page, anonymous", RequireRole(RoleStyret, deny)(ok), anonymous, http.StatusFound},
		{"board page, member", RequireRole(RoleStyret, deny)(ok), member, http.StatusForbidden},
		{"board page, board", RequireRole(RoleStyret, deny)(ok), board, http.StatusOK},

		{"api login, anonymous", RequireLoginAPI(ok), anonymous, http.StatusUnauthorized},
		{"api login, member", RequireLoginAPI(ok), member, http.StatusOK},
		{"api board, anonymous", RequireRoleAPI(RoleStyret)(ok), anonymous, http.StatusUnauthorized},
		// 401 rather than 403 for a missing role, matching the previous API.
		{"api board, member", RequireRoleAPI(RoleStyret)(ok), member, http.StatusUnauthorized},
		{"api board, board", RequireRoleAPI(RoleStyret)(ok), board, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/skjult", nil)
			if tt.user != nil {
				r = r.WithContext(WithUser(r.Context(), tt.user))
			}
			rec := httptest.NewRecorder()
			tt.handler.ServeHTTP(rec, r)

			if rec.Code != tt.want {
				t.Errorf("got %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

// ── Open redirect ─────────────────────────────────────────────────────────

// Without this check the login URL is an open redirect wearing our domain.
func TestSafeReturnTo(t *testing.T) {
	tests := map[string]string{
		"/profil":                      "/profil",
		"/arrangementer?old=1":         "/arrangementer?old=1",
		"":                             "/",
		"//evil.example":               "/",
		"https://evil.example":         "/",
		"http://evil.example/phishing": "/",
		"javascript:alert(1)":          "/",
		"////evil.example":             "/",
	}
	for in, want := range tests {
		if got := safeReturnTo(in); got != want {
			t.Errorf("safeReturnTo(%q) = %q, want %q", in, got, want)
		}
	}
}
