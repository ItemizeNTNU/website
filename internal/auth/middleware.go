package auth

import (
	"crypto/subtle"
	"encoding/json"
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
			// Clear it, so the claim above holds. A cookie that cannot be
			// opened would otherwise be re-sent on every request for as long as
			// the browser keeps it — the visitor is anonymous either way, but
			// each request carries a kilobyte of dead weight forever. Only when
			// one was actually sent: a first-time visitor has nothing to clear
			// and must not be handed a Set-Cookie they never asked for.
			//
			// Written before the handler runs, deliberately. The login callback
			// is behind this middleware too, and a member logging in after a key
			// rotation arrives with a stale cookie on the very request that
			// establishes the new one. A browser applies Set-Cookie in order, so
			// the clear has to come first or it would undo the login.
			if _, err := r.Cookie(SessionCookie); err == nil {
				a.sealer.Clear(w)
			}
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

// jsonError is the shape of an error body. Declared here rather than taken
// from the api package, which depends on this one.
type jsonError struct {
	Message string `json:"message"`
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// encoding/json rather than hand-rolled quoting: the saving was one
	// allocation on an error path, and the cost was a body that stopped being
	// parseable the moment a message contained a tab, a carriage return or any
	// other control character — which is exactly what happens the first time
	// somebody passes a provider or database error through here.
	//
	// Marshalling a struct of one string cannot fail: invalid UTF-8 is replaced
	// rather than rejected, so there is no error to report and nowhere to
	// report it once the status has gone out.
	body, _ := json.Marshal(jsonError{Message: msg})
	_, _ = w.Write(body)
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// ConstantTimeEqual compares two tokens without leaking how much of a guess
// was correct. Exported for callers that carry their own state parameter.
func ConstantTimeEqual(a, b string) bool { return constantTimeEqual(a, b) }
