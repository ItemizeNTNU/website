package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// SessionCookie is the name of the session cookie.
//
// Deliberately different from the previous site's "appSession": that cookie is
// sealed by a different library with a different scheme, and a name collision
// would mean every returning visitor's browser hands us something we cannot
// read. With a new name the old one is simply ignored — and expired
// explicitly, so it does not linger.
const SessionCookie = "itemize_session"

// LegacySessionCookie is the cookie the Sapper application used.
const LegacySessionCookie = "appSession"

// maxSessionAge caps how long a session lives regardless of what the identity
// provider said, so a stolen cookie cannot outlive its usefulness.
const maxSessionAge = 7 * 24 * time.Hour

// Session is what the sealed cookie carries.
//
// Deliberately not the access or ID token: nothing in this program needs them
// once login is done — server-to-server calls use the API key — and leaving
// them out means a leaked cookie yields a name and an email rather than a
// credential that works against FusionAuth.
type Session struct {
	User
	Issued  time.Time `json:"iat"`
	Expires time.Time `json:"exp"`
}

// Expired reports whether the session has passed its expiry.
func (s *Session) Expired() bool { return time.Now().After(s.Expires) }

// Sealer encrypts and decrypts session cookies.
//
// AES-256-GCM, which authenticates as well as encrypts: a tampered cookie
// fails to open rather than decoding into something unexpected. The key is
// derived by hashing the configured secret so that any secret length produces
// a valid 32-byte key — the length floor is enforced in config, since hashing
// a short secret would otherwise hide how weak it was.
type Sealer struct {
	aead   cipher.AEAD
	secure bool
}

// NewSealer derives the encryption key from secret. secure controls whether
// the cookie carries the Secure attribute; it must be false for plain-HTTP
// development or the browser will refuse to store it.
func NewSealer(secret string, secure bool) (*Sealer, error) {
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("building session cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("building session cipher: %w", err)
	}
	return &Sealer{aead: aead, secure: secure}, nil
}

// Seal encrypts a value into a cookie-safe string.
func (s *Sealer) Seal(v any) (string, error) {
	plaintext, err := json.Marshal(v)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	// The nonce is prepended to the ciphertext; GCM needs it to decrypt and it
	// is not secret.
	sealed := s.aead.Seal(nonce, nonce, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Open decrypts a value produced by Seal.
func (s *Sealer) Open(value string, into any) error {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return errors.New("cookie is not valid base64")
	}
	if len(raw) < s.aead.NonceSize() {
		return errors.New("cookie is too short to be valid")
	}
	nonce, ciphertext := raw[:s.aead.NonceSize()], raw[s.aead.NonceSize():]

	plaintext, err := s.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// Wrong key, tampering, or a cookie from a previous deployment. All
		// three are handled the same way: treat the visitor as anonymous.
		return errors.New("cookie could not be decrypted")
	}
	return json.Unmarshal(plaintext, into)
}

// Write stores the session in a cookie on the response.
func (s *Sealer) Write(w http.ResponseWriter, sess *Session) error {
	value, err := s.Seal(sess)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    value,
		Path:     "/",
		Expires:  sess.Expires,
		MaxAge:   int(time.Until(sess.Expires).Seconds()),
		HttpOnly: true,
		Secure:   s.secure,
		// Lax, not Strict. Strict would drop the cookie on the top-level
		// redirect back from FusionAuth, so the visitor would arrive logged in
		// and immediately appear logged out.
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// Read returns the session from the request, or nil when there is none, it has
// expired, or it cannot be decrypted.
func (s *Sealer) Read(r *http.Request) *Session {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return nil
	}
	var sess Session
	if err := s.Open(c.Value, &sess); err != nil {
		return nil
	}
	if sess.Expired() {
		return nil
	}
	return &sess
}

// Clear removes the session cookie, along with the one the previous site set
// so it does not sit in the jar indefinitely.
func (s *Sealer) Clear(w http.ResponseWriter) {
	for _, name := range []string{SessionCookie, LegacySessionCookie} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   s.secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

// NewSession builds a session for a user, clamping its lifetime to
// maxSessionAge.
func NewSession(u User, expires time.Time) *Session {
	now := time.Now()
	if limit := now.Add(maxSessionAge); expires.IsZero() || expires.After(limit) {
		expires = limit
	}
	return &Session{User: u, Issued: now, Expires: expires}
}
