package web

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/ItemizeNTNU/website/internal/auth"
)

// flashCookie carries a message across the redirect that follows a form
// submission, so the result is reported without the page being a POST that
// re-submits on refresh.
const flashCookie = "flash"

func userFrom(r *http.Request) *auth.User { return auth.FromRequest(r) }

// SetFlash queues a message to be shown on the next page the visitor loads.
//
// The value is not authenticated. It does not need to be: nothing here is
// secret, only this server ever sets it, and the worst a visitor can do by
// forging their own is show themselves a message. Template escaping handles
// the rest.
func SetFlash(w http.ResponseWriter, kind, text string) {
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookie,
		Value:    url.QueryEscape(kind + ":" + text),
		Path:     "/",
		MaxAge:   60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// takeFlash reads any queued message. Clearing it is the caller's job via
// ClearFlash, because the cookie has to be expired on the response and this
// only has the request.
func takeFlash(r *http.Request) []Toast {
	c, err := r.Cookie(flashCookie)
	if err != nil {
		return nil
	}
	raw, err := url.QueryUnescape(c.Value)
	if err != nil {
		return nil
	}
	kind, text, ok := strings.Cut(raw, ":")
	if !ok || text == "" {
		return nil
	}
	switch kind {
	case "success", "error", "info", "warning":
	default:
		kind = "info"
	}
	return []Toast{{Kind: kind, Text: text}}
}

// ClearFlash expires the flash cookie. It is applied by middleware so that
// every rendered page consumes whatever message it displayed.
func ClearFlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie(flashCookie); err == nil {
			http.SetCookie(w, &http.Cookie{
				Name:     flashCookie,
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}
		next.ServeHTTP(w, r)
	})
}
