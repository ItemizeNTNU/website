package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

type ctxKey int

const requestIDKey ctxKey = iota

// RequestIDFrom returns the identifier assigned to this request, or "" if the
// RequestID middleware did not run.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// RequestID assigns each request a short random identifier, exposes it in the
// context and echoes it in X-Request-Id. It belongs outermost so that every
// log line and panic report can be correlated.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b [8]byte
		rand.Read(b[:]) // never fails; documented to always succeed
		id := hex.EncodeToString(b[:])

		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// Logger writes one structured line per request. It sits outside Recover so
// that a recovered panic still produces an access-log entry with its 500.
func Logger(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &recorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			level := slog.LevelInfo
			if rec.Status() >= 500 {
				level = slog.LevelError
			}
			log.LogAttrs(r.Context(), level, "request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.Status()),
				slog.Int64("bytes", rec.written),
				slog.Duration("took", time.Since(start).Round(time.Microsecond)),
				slog.String("request_id", RequestIDFrom(r.Context())),
			)
		})
	}
}

// Recover turns a panic into a 500 instead of a dropped connection.
//
// errorPage renders the user-facing body; it is only called when nothing has
// been written yet, since a panic mid-response cannot be papered over.
func Recover(log *slog.Logger, errorPage func(http.ResponseWriter, *http.Request, int)) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &recorder{ResponseWriter: w}
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				// A client disconnecting mid-write is normal, not a bug.
				if v == http.ErrAbortHandler {
					panic(v)
				}
				log.Error("panic recovered",
					"err", v,
					"path", r.URL.Path,
					"request_id", RequestIDFrom(r.Context()),
					"stack", string(debug.Stack()),
				)
				if rec.status == 0 {
					errorPage(rec, r, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

// SecurityHeaders sets the response headers the old Express server never did.
//
// The CSP allows no inline script or style, which is why all CSS lives in the
// stylesheet rather than in per-page <style> blocks. Discord's CDN is allowed
// as an image source because linked members' avatars are served from it.
//
// HSTS is only sent when the site is actually served over TLS — emitting it
// from a plaintext development server pins localhost to https in the browser
// for a year.
func SecurityHeaders(https bool) Middleware {
	csp := strings.Join([]string{
		"default-src 'none'",
		"script-src 'self'",
		"style-src 'self'",
		"img-src 'self' data: https://cdn.discordapp.com",
		"font-src 'self'",
		"connect-src 'self'",
		"form-action 'self'",
		"base-uri 'none'",
		"frame-ancestors 'none'",
	}, "; ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Security-Policy", csp)
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=()")
			if https {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// TrailingSlash redirects /path/ to /path so each page has one canonical URL.
//
// ServeMux redirects the other way for subtree patterns but never this way,
// and the previous site's navigation linked to trailing-slash paths — so
// without this every inbound link from an old bookmark 404s.
func TrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if len(p) > 1 && strings.HasSuffix(p, "/") {
			target := *r.URL
			target.Path = collapseLeadingSlashes(strings.TrimRight(p, "/"))
			if target.Path == "" {
				target.Path = "/"
			}
			http.Redirect(w, r, target.RequestURI(), http.StatusMovedPermanently)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// collapseLeadingSlashes reduces a run of leading slashes to one.
//
// Without this the middleware is an open redirect. A request for
// "//evil.example/" is parsed server-side with the whole thing as the path —
// ParseRequestURI does not treat a leading "//" as an authority the way
// url.Parse does — so trimming the trailing slash yields "//evil.example",
// which as a Location header is a protocol-relative URL. The browser resolves
// it against the current scheme and leaves the site.
//
// This runs before the mux, so it cannot rely on ServeMux's own path
// cleaning: by the time that happens the redirect has already been sent.
func collapseLeadingSlashes(p string) string {
	i := 0
	for i < len(p) && p[i] == '/' {
		i++
	}
	if i <= 1 {
		return p
	}
	return p[i-1:]
}

// isNonPage reports whether a path serves something other than a rendered
// page — assets and the health probe — so per-page concerns can skip it.
func isNonPage(path string) bool {
	if path == "/healthz" {
		return true
	}
	if strings.HasPrefix(path, "/assets/") {
		return true
	}
	switch path {
	case "/icon.svg", "/logo.png", "/logo-192.png", "/logo-512.png",
		"/manifest.json", "/robots.txt", "/service-worker.js":
		return true
	}
	return false
}

// cookieRecipe is a Norwegian chocolate chip cookie recipe, base64 of
// ISO-8859-1 bytes. Carried over verbatim from the previous site as an easter
// egg for anyone who looks at their cookie jar.
//
// Do not re-encode this as UTF-8: the Latin-1 bytes are what make "smør" and
// "råsukker" decode correctly, and "modernising" it turns the recipe into
// mojibake.
const cookieRecipe = `MjI1ZyByb210ZW1wZXJlcnQgbWVpZXJpc234cgoyMDBnIHN1a2tlcgoyMDBnIHLlcvhyc3Vra2VyCjIgc3RrIGVnZwo0MDBnIGh2ZXRlbWVsCjAuNSB0cyBiYWtlcHVsdmVyCjEgdHMgbmF0cm9uCjEgdHMgc2FsdAoyIHRzIHZhbmlsamVzdWtrZXIKMzUwZyBzam9rb2xhZGUgKG34cmsgZWxsZXIgaGFsdm34cmspCgoKMS4gRm9ydmFybSBvdm5lbiB0aWwgMTkwQy4gUGlzayByb210ZW1wZXJlcnQgc234ciwgc3Vra2VyIG9nIHLlcvhyc3Vra2VyIGh2aXR0IGkgZW4ga2r4a2tlbm1hc2tpbiBlbGxlciBtZWQgZW4gZWxla3RyaXNrIHZpc3AuCgoyLiBIYSBpIGV0dCBlZ2cgb20gZ2FuZ2VuLCBvZyBwaXNrIGdvZHQgbWVsbG9tIGh2ZXJ0IGVnZy4gVGlsc2V0dCBtZWwsIGJha2VwdWx2ZXIsIG5hdHJvbiwgc2FsdCBvZyB2YW5pbGplc3Vra2VyIG9nIGtq-HIgdGlsIGdvZHQgYmxhbmRldC4KCjMuIEdyb3ZoYWtrIHNqb2tvbGFkZSBvZyB2ZW5kIGlubi4gU3BhciBldmVudHVlbHQgbm9lIHRpbCBweW50IHDlIHRvcHBlbi4KCjQuIFNldHQgMS0yIHNwaXNlc2tqZWVyIGRlaWcgcOUgc3Rla2VicmV0dCBkZWtrZXQgbWVkIGJha2VwYXBpci4gU_hyZyBmb3IgYXQgZGV0IGVyIGdvZHQgbWVkIHJvbSBtZWxsb20gaHZlciBramVrcyBkYSBkZSBmbHl0ZXIgdXRvdmVyLCBjYS4gNiBwZXIgYnJldHQuCgo1LiBTdGlrayBub2VuIHNqb2tvbGFkZWJpdGVyIHDlIHRvcHBlbiBhdiBodmVyIGtqZWtzIG9nIHN0ZWsgaSBvdm5lbiBpIGNhLiAxMi0xNSBtaW51dHRlciBlbGxlciB0aWwgZ3lsbmUuIEF2a2r4bCBsaXR0IHDlIHBsYXRlbiBm-HIgZGUgZmx5dHRlcyBvdmVyIHDlIHJpc3Qu`

// CookieRecipe sets the easter-egg cookie on page responses.
//
// Assets and the health endpoint are skipped: the value is about 1.6 KB, and
// attaching it to every stylesheet, font and probe would be pure waste. It is
// also skipped when the client already has it.
func CookieRecipe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isNonPage(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie("cookies"); err != nil || c.Value != cookieRecipe {
			http.SetCookie(w, &http.Cookie{
				Name:     "cookies",
				Value:    cookieRecipe,
				Path:     "/",
				SameSite: http.SameSiteLaxMode,
			})
		}
		next.ServeHTTP(w, r)
	})
}
