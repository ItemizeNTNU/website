package auth

import (
	"crypto/rand"
	"encoding/base64"
	"html/template"
	"net/http"
)

// csrfCookie holds the token. It is readable by script — that is what the
// double-submit pattern requires — so it carries nothing sensitive.
const csrfCookie = "itemize_csrf"

// CSRFField is the hidden input name carrying the token.
const CSRFField = "_csrf"

// maxFormBytes bounds a submitted form. The largest field on the site is an
// event description capped at 2000 characters.
const maxFormBytes = 64 << 10

// CSRF protects state-changing requests.
//
// The previous site sent JSON with fetch, which browsers will not issue
// cross-origin without a preflight the server never answered — so it was
// implicitly protected. Ordinary form posts have no such barrier, and this
// rewrite uses them precisely so the site works without JavaScript. The
// protection has to be explicit now.
//
// Two independent checks, either of which is sufficient on its own:
//
//   - Sec-Fetch-Site, sent by every current browser and not forgeable by page
//     script, must not say the request came from another site.
//   - A double-submitted token: a random value in a cookie must match the one
//     in the form body. An attacker's page can cause a request to be sent with
//     our cookies attached, but cannot read them to copy the value into it.
//
// SameSite=Lax on the session cookie already blocks the common case; this is
// defence in depth, and it is cheap.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
			next.ServeHTTP(w, r)
			return
		}

		if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
			http.Error(w, "Forespørselen ble avvist av sikkerhetsgrunner.", http.StatusForbidden)
			return
		}

		cookie, err := r.Cookie(csrfCookie)
		if err != nil || cookie.Value == "" {
			http.Error(w, "Skjemaet er utløpt. Last siden på nytt og prøv igjen.", http.StatusForbidden)
			return
		}
		// Bound the body before parsing. ParseForm otherwise buffers up to
		// Go's 10 MB default per request, which is a cheap way for one client
		// to occupy a lot of memory across many connections. No form on this
		// site is anywhere near this size.
		r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)

		// ParseForm has to run before reading the field, and doing it here
		// means the handler's later call is a no-op rather than a re-read.
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Skjemaet kunne ikke leses.", http.StatusBadRequest)
			return
		}
		if !constantTimeEqual(cookie.Value, r.PostFormValue(CSRFField)) {
			http.Error(w, "Skjemaet er utløpt. Last siden på nytt og prøv igjen.", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// secureCookies records whether this deployment is served over TLS.
//
// It cannot be derived from the request. r.URL.Scheme is empty on the server
// side — the request line carries a path, not an absolute URL — and r.TLS is
// nil whenever TLS is terminated by a proxy in front of us, which is how this
// site is actually deployed. Both checks therefore evaluate false in
// production, and the cookie went out without the Secure attribute: readable
// over any plaintext request to the same host.
//
// Set once at startup from BASE_URL, the same source the session cookie uses.
var secureCookies bool

// SetSecureCookies tells this package whether to mark cookies Secure.
func SetSecureCookies(secure bool) { secureCookies = secure }

// CSRFToken returns the token for this request, issuing one if needed.
//
// Called while rendering a page so the token is in place before any form on it
// is submitted.
func CSRFToken(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil && c.Value != "" {
		return c.Value
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	token := base64.RawURLEncoding.EncodeToString(b)

	http.SetCookie(w, &http.Cookie{
		Name:  csrfCookie,
		Value: token,
		Path:  "/",
		// Not HttpOnly: the double-submit pattern needs the value to be
		// readable, and it is not a secret — it is only useful to a page that
		// is already same-origin, which is exactly the case we permit.
		HttpOnly: false,
		Secure:   secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	return token
}

// CSRFInput renders the hidden field for a form.
func CSRFInput(token string) template.HTML {
	return template.HTML(`<input type="hidden" name="` + CSRFField +
		`" value="` + template.HTMLEscapeString(token) + `">`)
}
