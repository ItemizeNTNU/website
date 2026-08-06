package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// hmacKeySet verifies HS256-signed ID tokens.
//
// TODO(auth): delete this file once FusionAuth signs our ID tokens with RS256.
// See docs/auth.md — the switch is a per-application setting, so it can be made
// without affecting the wiki or any other application on the same tenant.
// Once it is made, SigningAlg becomes "RS256" and go-oidc's own JWKS-backed
// verifier takes over; nothing else in this package changes.
//
// Why this exists at all: go-oidc verifies against the provider's published
// JWKS, which by definition contains only public keys. HS256 is symmetric —
// the same secret signs and verifies — so there is nothing in the JWKS to
// verify against and the library has no path for it. The interface it needs is
// a single method, which is what this implements.
//
// The security trade-off is worth stating plainly: under HS256 anyone holding
// the shared secret can *mint* ID tokens, not merely check them. Under RS256
// only FusionAuth holds the private key. That is the whole argument for
// migrating.
type hmacKeySet struct {
	secret []byte
}

// VerifySignature checks an HS256 JWT and returns its payload.
//
// Two things here are the entire security of the function:
//
//   - The algorithm is asserted to be HS256 rather than read from the token
//     and dispatched on. Trusting a token's own "alg" header is how algorithm
//     confusion attacks work: an attacker sends alg=none, or re-signs an
//     RS256 token as HS256 using the public key as the HMAC secret, and a
//     verifier that follows the header accepts it.
//   - The comparison is hmac.Equal, which runs in constant time. A plain byte
//     comparison leaks how much of a forged signature was correct, which is
//     enough to construct one a byte at a time.
func (k hmacKeySet) VerifySignature(_ context.Context, jwt string) ([]byte, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed JWT: expected 3 parts, got %d", len(parts))
	}

	rawHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("malformed JWT header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(rawHeader, &header); err != nil {
		return nil, fmt.Errorf("unreadable JWT header: %w", err)
	}
	if header.Alg != "HS256" {
		return nil, fmt.Errorf("unexpected signing algorithm %q, only HS256 is accepted here", header.Alg)
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("malformed JWT signature: %w", err)
	}

	mac := hmac.New(sha256.New, k.secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(mac.Sum(nil), signature) {
		return nil, errors.New("JWT signature does not verify")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("malformed JWT payload: %w", err)
	}
	return payload, nil
}
