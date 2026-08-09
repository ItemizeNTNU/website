package auth

// Adversarial tests for the HS256 ID-token verifier and for the claim checks
// go-oidc layers on top of it. Everything here is offline: the verifier is
// constructed directly with oidc.NewVerifier, so no provider is contacted and
// no clock is real — oidc.Config.Now is frozen instead.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// itoa renders a Unix timestamp for embedding in a hand-written claim set.
func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// hmacSecret is the shared secret these tests sign with. Distinct from the
// session-sealing secret in auth_test.go so that a mix-up between the two shows
// up as a failure rather than accidentally passing.
const hmacSecret = "id-token-hmac-secret-0123456789ab"

func b64seg(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

// mintJWT signs the given header and payload with secret. The declared alg and
// the algorithm actually used are deliberately decoupled: forging an
// "alg": "none" or RS256-labelled token that nonetheless carries a valid HMAC
// is exactly the attack the verifier has to refuse, and a helper that kept them
// in step could not express it.
func mintJWT(t *testing.T, secret, header, payload string) string {
	t.Helper()
	h, p := b64seg(header), b64seg(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(h + "." + p))
	return h + "." + p + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// signOver produces the correct signature segment for two already-encoded
// segments, letting a test sign material that is not valid base64 or not JSON.
func signOver(secret, encodedHeader, encodedPayload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(encodedHeader + "." + encodedPayload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func hs256Header(alg string) string { return `{"alg":"` + alg + `","typ":"JWT"}` }

// ── Signature integrity ───────────────────────────────────────────────────

// A payload that has been edited after signing must not verify. If it did,
// anyone who could intercept a token could promote themselves to Styret by
// rewriting the roles claim.
func TestHS256RejectsTamperedPayload(t *testing.T) {
	ks := hmacKeySet{secret: []byte(hmacSecret)}
	honest := mintJWT(t, hmacSecret, hs256Header("HS256"), `{"sub":"fa-1","roles":["Medlem"]}`)
	parts := strings.Split(honest, ".")

	forged := parts[0] + "." + b64seg(`{"sub":"fa-1","roles":["Styret"]}`) + "." + parts[2]
	if _, err := ks.VerifySignature(context.Background(), forged); err == nil {
		t.Error("a token whose payload was rewritten after signing verified; " +
			"any member could grant themselves the board role")
	}
}

// Flipping bits in the signature must fail. This also covers the case of an
// attacker who knows the payload they want and is guessing at the MAC.
func TestHS256RejectsMutatedSignature(t *testing.T) {
	ks := hmacKeySet{secret: []byte(hmacSecret)}
	honest := mintJWT(t, hmacSecret, hs256Header("HS256"), `{"sub":"fa-1"}`)
	parts := strings.Split(honest, ".")

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]func() string{
		"first byte flipped": func() string {
			m := append([]byte(nil), sig...)
			m[0] ^= 0xFF
			return base64.RawURLEncoding.EncodeToString(m)
		},
		"last byte flipped": func() string {
			m := append([]byte(nil), sig...)
			m[len(m)-1] ^= 0x01
			return base64.RawURLEncoding.EncodeToString(m)
		},
		"truncated to 16 bytes": func() string {
			return base64.RawURLEncoding.EncodeToString(sig[:16])
		},
		"extended with a trailing byte": func() string {
			return base64.RawURLEncoding.EncodeToString(append(append([]byte(nil), sig...), 0x00))
		},
		"empty": func() string { return "" },
		"all zeroes": func() string {
			return base64.RawURLEncoding.EncodeToString(make([]byte, sha256.Size))
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			forged := parts[0] + "." + parts[1] + "." + mutate()
			if _, err := ks.VerifySignature(context.Background(), forged); err == nil {
				t.Errorf("a token with a %s signature was accepted, so the MAC is not "+
					"actually gating anything", name)
			}
		})
	}
}

// Only the configured secret may verify. A key that is a prefix, a suffix, one
// byte different, or empty must all fail — otherwise rotating the shared secret
// would not actually invalidate anything.
func TestHS256RejectsEveryOtherKey(t *testing.T) {
	honest := mintJWT(t, hmacSecret, hs256Header("HS256"), `{"sub":"fa-1"}`)

	wrong := map[string]string{
		"empty":                 "",
		"one byte shorter":      hmacSecret[:len(hmacSecret)-1],
		"one byte longer":       hmacSecret + "x",
		"one character changed": strings.Replace(hmacSecret, "a", "b", 1),
		"unrelated same length": strings.Repeat("f", len(hmacSecret)),
	}
	for name, secret := range wrong {
		t.Run(name, func(t *testing.T) {
			ks := hmacKeySet{secret: []byte(secret)}
			if _, err := ks.VerifySignature(context.Background(), honest); err == nil {
				t.Errorf("a token signed with the real secret verified under a %s key", name)
			}
		})
	}
}

// An empty configured secret is a misconfiguration, but it must not degrade
// into "accept anything" — HMAC with an empty key is still a real MAC.
func TestHS256WithEmptySecretStillRequiresACorrectMAC(t *testing.T) {
	ks := hmacKeySet{secret: nil}
	if _, err := ks.VerifySignature(context.Background(),
		mintJWT(t, hmacSecret, hs256Header("HS256"), `{"sub":"fa-1"}`)); err == nil {
		t.Error("an empty verification key accepted a token signed with a different key")
	}
	if _, err := ks.VerifySignature(context.Background(),
		mintJWT(t, "", hs256Header("HS256"), `{"sub":"fa-1"}`)); err != nil {
		t.Errorf("HMAC with an empty key should still verify its own output: %v", err)
	}
}

// ── Algorithm confusion ───────────────────────────────────────────────────

// The verifier must assert HS256 rather than dispatch on the token's own
// header. Every entry here carries a *valid* HMAC over its segments, so the
// only thing that can reject them is the algorithm assertion — which is the
// point: a verifier that trusted the header would accept all of them.
func TestHS256RefusesAnyHeaderThatIsNotExactlyHS256(t *testing.T) {
	ks := hmacKeySet{secret: []byte(hmacSecret)}

	headers := map[string]string{
		"alg none":                `{"alg":"none"}`,
		"alg None":                `{"alg":"None"}`,
		"alg NONE":                `{"alg":"NONE"}`,
		"alg empty string":        `{"alg":""}`,
		"alg absent":              `{"typ":"JWT"}`,
		"alg null":                `{"alg":null}`,
		"alg RS256":               `{"alg":"RS256"}`,
		"alg ES256":               `{"alg":"ES256"}`,
		"alg HS384":               `{"alg":"HS384"}`,
		"alg HS512":               `{"alg":"HS512"}`,
		"alg lowercase hs256":     `{"alg":"hs256"}`,
		"alg with trailing space": `{"alg":"HS256 "}`,
		"alg with leading space":  `{"alg":" HS256"}`,
		// Not a string at all. A verifier that scanned the raw header bytes for
		// the substring "HS256" rather than decoding it would be fooled.
		"alg is an array containing HS256": `{"alg":["HS256"]}`,
		"alg is a nested object":           `{"alg":{"alg":"HS256"}}`,
		// encoding/json keeps the last value for a duplicated key, so this
		// header resolves to "none". A verifier that merely searched the raw
		// header bytes for "HS256" would accept it.
		"alg repeated, none second": `{"alg":"HS256","alg":"none"}`,
	}

	for name, header := range headers {
		t.Run(name, func(t *testing.T) {
			token := mintJWT(t, hmacSecret, header, `{"sub":"attacker","roles":["Styret"]}`)
			if _, err := ks.VerifySignature(context.Background(), token); err == nil {
				t.Errorf("a correctly MACed token with header %s was accepted; the verifier "+
					"is trusting the token's own algorithm claim", header)
			}
		})
	}
}

// The one header that must work, including when the provider adds fields we do
// not read. Refusing unknown header fields would break on any FusionAuth
// upgrade that starts emitting "kid".
func TestHS256AcceptsHS256WithUnknownHeaderFields(t *testing.T) {
	ks := hmacKeySet{secret: []byte(hmacSecret)}
	header := `{"typ":"JWT","kid":"abc123","alg":"HS256","cty":"JWT"}`

	payload, err := ks.VerifySignature(context.Background(),
		mintJWT(t, hmacSecret, header, `{"sub":"fa-1"}`))
	if err != nil {
		t.Fatalf("a valid HS256 token with extra header fields was rejected, which would "+
			"break login the moment FusionAuth adds a key id: %v", err)
	}
	if string(payload) != `{"sub":"fa-1"}` {
		t.Errorf("payload came back altered: %s", payload)
	}
}

// ── Shape ─────────────────────────────────────────────────────────────────

// Anything that is not exactly three dot-separated segments must be refused
// before any cryptography happens. A verifier that indexed into the parts
// without checking would panic on short input, turning a malformed cookie into
// a denial of service.
func TestHS256RejectsWrongSegmentCounts(t *testing.T) {
	ks := hmacKeySet{secret: []byte(hmacSecret)}

	for _, token := range []string{
		"",
		".",
		"..",
		"...",
		"....",
		"a",
		"a.b",
		"a.b.c.d",
		strings.Repeat(".", 100),
		b64seg(hs256Header("HS256")) + "." + b64seg(`{"sub":"x"}`), // signature dropped
	} {
		t.Run("token "+token, func(t *testing.T) {
			if _, err := ks.VerifySignature(context.Background(), token); err == nil {
				t.Errorf("%q was accepted as a JWT", token)
			}
		})
	}
}

// Each segment must be raw base64url. Standard-alphabet and padded encodings
// are not the same thing and must not be quietly tolerated.
func TestHS256RejectsInvalidBase64(t *testing.T) {
	ks := hmacKeySet{secret: []byte(hmacSecret)}
	goodHeader := b64seg(hs256Header("HS256"))
	goodPayload := b64seg(`{"sub":"fa-1"}`)

	tests := map[string]string{
		"header is not base64":             "!!!!." + goodPayload + "." + signOver(hmacSecret, "!!!!", goodPayload),
		"header uses padding":              "eyJhbGciOiJIUzI1NiJ9=." + goodPayload + ".sig",
		"header uses the + and / alphabet": "ab+/cd." + goodPayload + "." + signOver(hmacSecret, "ab+/cd", goodPayload),
		"signature is not base64":          goodHeader + "." + goodPayload + ".!!!!",
		"signature is padded":              goodHeader + "." + goodPayload + ".YWJj=",
	}
	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ks.VerifySignature(context.Background(), token); err == nil {
				t.Errorf("%s: token accepted", name)
			}
		})
	}
}

// A payload segment that is correctly signed but not decodable must still fail.
// This pins the ordering inside the verifier — signature first, decode second —
// which is what keeps an attacker from using decode errors as an oracle.
func TestHS256ChecksTheSignatureBeforeDecodingThePayload(t *testing.T) {
	ks := hmacKeySet{secret: []byte(hmacSecret)}
	header := b64seg(hs256Header("HS256"))
	badPayload := "!!!not-base64!!!"

	token := header + "." + badPayload + "." + signOver(hmacSecret, header, badPayload)
	if _, err := ks.VerifySignature(context.Background(), token); err == nil {
		t.Error("a token with an undecodable payload was accepted")
	} else if !strings.Contains(err.Error(), "payload") {
		t.Errorf("expected the failure to name the payload, got %v", err)
	}
}

func TestHS256RejectsUnreadableHeader(t *testing.T) {
	ks := hmacKeySet{secret: []byte(hmacSecret)}

	for name, header := range map[string]string{
		"not JSON at all": `this is not json`,
		"a JSON array":    `["HS256"]`,
		"a JSON string":   `"HS256"`,
		"alg is a number": `{"alg":256}`,
		"empty":           ``,
	} {
		t.Run(name, func(t *testing.T) {
			token := mintJWT(t, hmacSecret, header, `{"sub":"fa-1"}`)
			if _, err := ks.VerifySignature(context.Background(), token); err == nil {
				t.Errorf("a token whose header was %s was accepted", name)
			}
		})
	}
}

// The verifier's contract is signature checking only: it hands back the raw
// payload bytes and leaves claim parsing to go-oidc. A non-JSON payload
// therefore comes back without error. This is not a defect, but it is a
// boundary worth pinning — if this file ever grows a claim check, callers must
// not end up validating claims twice with different rules.
func TestHS256ReturnsThePayloadVerbatimWithoutParsingIt(t *testing.T) {
	ks := hmacKeySet{secret: []byte(hmacSecret)}

	for _, payload := range []string{`not json`, `[]`, `null`, `{}`, ``} {
		got, err := ks.VerifySignature(context.Background(),
			mintJWT(t, hmacSecret, hs256Header("HS256"), payload))
		if err != nil {
			t.Fatalf("a correctly signed token with payload %q was rejected here rather "+
				"than by the claim parser: %v", payload, err)
		}
		if string(got) != payload {
			t.Errorf("payload %q came back as %q", payload, got)
		}
	}
}

// ── Constant time ─────────────────────────────────────────────────────────

// Signature comparison must not short-circuit on the first differing byte: the
// timing difference is enough to forge a MAC one byte at a time. Timing cannot
// be asserted reliably in a unit test, so this reads the source instead. A
// refactor that swaps hmac.Equal for bytes.Equal or == is a security
// regression, and it must fail here rather than pass silently.
func TestHS256UsesAConstantTimeComparison(t *testing.T) {
	src, err := os.ReadFile("hs256.go")
	if err != nil {
		t.Fatal(err)
	}
	code := string(src)
	if !strings.Contains(code, "hmac.Equal(") {
		t.Error("hs256.go no longer calls hmac.Equal; signature comparison must be " +
			"constant-time or the MAC can be forged a byte at a time")
	}
	for _, leaky := range []string{"bytes.Equal(", "string(mac.Sum"} {
		if strings.Contains(code, leaky) {
			t.Errorf("hs256.go contains %q, which compares in variable time", leaky)
		}
	}
}

// ── Claim validation, as go-oidc performs it over this key set ────────────

// frozenVerifier builds the real ID-token verifier this package uses in
// production, with the clock pinned so expiry tests can never be flaky.
func frozenVerifier(issuer, clientID string, now time.Time) *oidc.IDTokenVerifier {
	return oidc.NewVerifier(issuer, hmacKeySet{secret: []byte(hmacSecret)}, &oidc.Config{
		ClientID:             clientID,
		SupportedSigningAlgs: []string{"HS256"},
		Now:                  func() time.Time { return now },
	})
}

func TestIDTokenTimeClaims(t *testing.T) {
	const issuer, clientID = "https://auth.example", "itemize-web"
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	verifier := frozenVerifier(issuer, clientID, now)

	// claimsJSON builds a minimal-but-valid token body, letting each case
	// override only the time claims it cares about.
	body := func(extra string) string {
		return `{"iss":"` + issuer + `","aud":"` + clientID + `","sub":"fa-1"` + extra + `}`
	}
	tests := []struct {
		name    string
		extra   string
		wantErr bool
		why     string
	}{
		{
			name:  "expires in an hour",
			extra: `,"exp":` + itoa(now.Add(time.Hour).Unix()) + `,"iat":` + itoa(now.Unix()),
			why:   "an ordinary fresh token must log the member in",
		},
		{
			name:    "expired a second ago",
			extra:   `,"exp":` + itoa(now.Add(-time.Second).Unix()),
			wantErr: true,
			why:     "an expired ID token must not establish a session",
		},
		{
			name:  "expires exactly now",
			extra: `,"exp":` + itoa(now.Unix()),
			// go-oidc uses Expiry.Before(now), so the boundary second is still
			// valid. Pinned deliberately: if this ever flips, logins will start
			// failing intermittently for tokens issued with a zero lifetime.
			why: "the exp boundary is inclusive",
		},
		{
			name:    "no exp claim at all",
			extra:   ``,
			wantErr: true,
			why: "a token without exp decodes to the zero time, which must read as " +
				"expired rather than as never expiring",
		},
		{
			name:    "exp far in the past",
			extra:   `,"exp":1`,
			wantErr: true,
			why:     "a decade-old token must not be replayable",
		},
		{
			name:  "nbf four minutes in the future",
			extra: `,"exp":` + itoa(now.Add(time.Hour).Unix()) + `,"nbf":` + itoa(now.Add(4*time.Minute).Unix()),
			why:   "go-oidc allows five minutes of clock skew on nbf; a slightly fast provider must still work",
		},
		{
			name:    "nbf six minutes in the future",
			extra:   `,"exp":` + itoa(now.Add(time.Hour).Unix()) + `,"nbf":` + itoa(now.Add(6*time.Minute).Unix()),
			wantErr: true,
			why:     "beyond the skew allowance a not-yet-valid token must be refused",
		},
		{
			name:  "nbf in the past",
			extra: `,"exp":` + itoa(now.Add(time.Hour).Unix()) + `,"nbf":` + itoa(now.Add(-time.Hour).Unix()),
			why:   "a token that became valid an hour ago is fine",
		},
		{
			name:  "iat in the future",
			extra: `,"exp":` + itoa(now.Add(time.Hour).Unix()) + `,"iat":` + itoa(now.Add(time.Hour).Unix()),
			// Characterisation, not endorsement: go-oidc does not check iat at
			// all. Recorded so that anyone reasoning about replay windows knows
			// iat is decorative here and only exp and nbf are enforced.
			why: "iat is not validated by go-oidc",
		},
		{
			name:  "iat missing",
			extra: `,"exp":` + itoa(now.Add(time.Hour).Unix()),
			why:   "iat is optional in practice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := mintJWT(t, hmacSecret, hs256Header("HS256"), body(tt.extra))
			_, err := verifier.Verify(context.Background(), token)
			if tt.wantErr && err == nil {
				t.Errorf("token was accepted but should not have been (%s)", tt.why)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("token was rejected but should have been accepted (%s): %v", tt.why, err)
			}
		})
	}
}

func TestIDTokenIssuerAndAudience(t *testing.T) {
	const issuer, clientID = "https://auth.example", "itemize-web"
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	verifier := frozenVerifier(issuer, clientID, now)
	exp := itoa(now.Add(time.Hour).Unix())

	tests := []struct {
		name    string
		body    string
		wantErr bool
		why     string
	}{
		{
			name: "correct issuer and audience",
			body: `{"iss":"` + issuer + `","aud":"` + clientID + `","sub":"fa-1","exp":` + exp + `}`,
			why:  "our own provider's token must work",
		},
		{
			name:    "issuer is a different provider",
			body:    `{"iss":"https://evil.example","aud":"` + clientID + `","sub":"fa-1","exp":` + exp + `}`,
			wantErr: true,
			why:     "a token minted elsewhere must never be accepted, however well signed",
		},
		{
			name:    "issuer differs only by a trailing slash",
			body:    `{"iss":"` + issuer + `/","aud":"` + clientID + `","sub":"fa-1","exp":` + exp + `}`,
			wantErr: true,
			why:     "issuer matching is exact; this is the misconfiguration New's error message warns about",
		},
		{
			name:    "issuer missing",
			body:    `{"aud":"` + clientID + `","sub":"fa-1","exp":` + exp + `}`,
			wantErr: true,
			why:     "an absent issuer must not pass as a match",
		},
		{
			name:    "audience is another application on the same tenant",
			body:    `{"iss":"` + issuer + `","aud":"the-wiki","sub":"fa-1","exp":` + exp + `}`,
			wantErr: true,
			why: "FusionAuth hosts the wiki on the same tenant; a token issued to it must not " +
				"be replayable against the website",
		},
		{
			name: "audience is a list containing us",
			body: `{"iss":"` + issuer + `","aud":["the-wiki","` + clientID + `"],"sub":"fa-1","exp":` + exp + `}`,
			why:  "a multi-audience token that names us is valid",
		},
		{
			name:    "audience is a list not containing us",
			body:    `{"iss":"` + issuer + `","aud":["the-wiki","something-else"],"sub":"fa-1","exp":` + exp + `}`,
			wantErr: true,
			why:     "membership in the audience list must be checked, not merely its presence",
		},
		{
			name:    "audience missing",
			body:    `{"iss":"` + issuer + `","sub":"fa-1","exp":` + exp + `}`,
			wantErr: true,
			why:     "a token with no audience is not addressed to us",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := verifier.Verify(context.Background(),
				mintJWT(t, hmacSecret, hs256Header("HS256"), tt.body))
			if tt.wantErr && err == nil {
				t.Errorf("token was accepted but should not have been (%s)", tt.why)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("token was rejected but should have been accepted (%s): %v", tt.why, err)
			}
		})
	}
}

// go-oidc filters on the declared algorithm before the key set is consulted, so
// this is a second, independent barrier against algorithm confusion. Both must
// hold: SupportedSigningAlgs here, and the assertion inside hmacKeySet.
func TestVerifierRejectsUnsupportedAlgorithmsBeforeReachingTheKeySet(t *testing.T) {
	const issuer, clientID = "https://auth.example", "itemize-web"
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	verifier := frozenVerifier(issuer, clientID, now)

	body := `{"iss":"` + issuer + `","aud":"` + clientID + `","sub":"fa-1","exp":` +
		itoa(now.Add(time.Hour).Unix()) + `}`

	for _, alg := range []string{"none", "RS256", "HS512", "ES256"} {
		t.Run(alg, func(t *testing.T) {
			if _, err := verifier.Verify(context.Background(),
				mintJWT(t, hmacSecret, hs256Header(alg), body)); err == nil {
				t.Errorf("the verifier accepted a token declaring alg=%q", alg)
			}
		})
	}
}
