package auth

import (
	"crypto/subtle"
	"net/http"
	"net/url"
)

// Inject reads the session cookie and puts the user in the request context.
//
// It never fails a request. A cookie that cannot be opened — wrong key, a
// deployment with a rotated secret, tampering — is cleared and the visitor is
// simply anonymous, which is what they effectively are.
func (a *Authenticator) Inject(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := a.sealer.Read(r)
		if sess == nil {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), &sess.User)))
	})
}

// Denier renders the page shown when access is refused. The web package
// supplies its error page so a 403 looks like the rest of the site.
type Denier func(w http.ResponseWriter, r *http.Request, status int)

// RequireLogin sends anonymous visitors to log in, returning them here
// afterwards.
func RequireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if FromRequest(r) == nil {
			http.Redirect(w, r,
				"/login?return_to="+url.QueryEscape(r.URL.RequestURI()),
				http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRole gates a page on a role. Anonymous visitors are sent to log in —
// they may well have the role once signed in — while a signed-in visitor
// without it is told plainly that they do not.
func RequireRole(role string, deny Denier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := FromRequest(r)
			switch {
			case user == nil:
				http.Redirect(w, r,
					"/login?return_to="+url.QueryEscape(r.URL.RequestURI()),
					http.StatusFound)
			case !user.HasRole(role):
				deny(w, r, http.StatusForbidden)
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

// RequireLoginAPI is RequireLogin for JSON endpoints.
//
// The status codes and messages match the previous API exactly, including its
// use of 401 rather than 403 for a missing role — clients key off the message
// text, so changing either would be a silent break.
func RequireLoginAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if FromRequest(r) == nil {
			writeJSONError(w, http.StatusUnauthorized, "You are not logged in")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRoleAPI is RequireRole for JSON endpoints.
func RequireRoleAPI(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := FromRequest(r)
			switch {
			case user == nil:
				writeJSONError(w, http.StatusUnauthorized, "You are not logged in")
			case !user.HasRole(role):
				writeJSONError(w, http.StatusUnauthorized, "Permission denied")
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// Small enough to hand-write, and doing so keeps this package free of a
	// dependency on the api package, which depends on this one.
	_, _ = w.Write([]byte(`{"message":` + quoteJSON(msg) + `}`))
}

func quoteJSON(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"', '\\':
			out = append(out, '\\', c)
		case '\n':
			out = append(out, '\\', 'n')
		default:
			out = append(out, c)
		}
	}
	return string(append(out, '"'))
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// ConstantTimeEqual compares two tokens without leaking how much of a guess
// was correct. Exported for callers that carry their own state parameter.
func ConstantTimeEqual(a, b string) bool { return constantTimeEqual(a, b) }
