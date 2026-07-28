package web

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/ItemizeNTNU/website/internal/auth"
)

// AssetResolver maps a logical asset name to its public URL.
type AssetResolver interface {
	URL(name string) string
	Has(name string) bool
	// Refresh picks up on-disk changes during development. It is a no-op in
	// production.
	Refresh() error
}

// Funcs builds the template function map. Keep this small: anything that needs
// real logic belongs in a handler, where it can be tested.
func Funcs(assets AssetResolver) template.FuncMap {
	return template.FuncMap{
		"asset":    assets.URL,
		"hasAsset": assets.Has,
		"eml":      EncodeEmail,
		"emlfallback": func(addr string) string {
			return obfuscateForHumans(addr)
		},
		"csrf":    auth.CSRFInput,
		"dict":    dict,
		"list":    func(v ...string) []string { return v },
		"hasRole": hasRole,
	}
}

// EncodeEmail rotates an address by thirteen places so the plain string never
// appears in the served HTML. app.js reverses it in the browser.
//
// ROT13 is not secrecy and is not meant to be — the point is only that the
// address does not match the pattern a harvester greps for.
func EncodeEmail(addr string) string {
	var b strings.Builder
	b.Grow(len(addr))
	for _, r := range addr {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune('a' + (r-'a'+13)%26)
		case r >= 'A' && r <= 'Z':
			b.WriteRune('A' + (r-'A'+13)%26)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// obfuscateForHumans renders an address for the <noscript> fallback: readable
// to a person, invisible to a regular expression looking for user@host.
func obfuscateForHumans(addr string) string {
	local, domain, ok := strings.Cut(addr, "@")
	if !ok {
		return addr
	}
	return local + " [at] " + strings.ReplaceAll(domain, ".", " [dot] ")
}

// dict builds a map from alternating key/value pairs, so a partial can be
// called with more than one argument.
func dict(values ...any) (map[string]any, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict needs an even number of arguments, got %d", len(values))
	}
	m := make(map[string]any, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict keys must be strings, got %T at %d", values[i], i)
		}
		m[key] = values[i+1]
	}
	return m, nil
}

func hasRole(roles []string, want string) bool {
	for _, r := range roles {
		if r == want {
			return true
		}
	}
	return false
}
