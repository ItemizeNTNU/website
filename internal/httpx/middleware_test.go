package httpx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func wireRequest(t *testing.T, target string) *http.Request {
	t.Helper()
	r, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
		"GET " + target + " HTTP/1.1\r\nHost: itemize.no\r\n\r\n")))
	if err != nil {
		t.Fatalf("%s: %v", target, err)
	}
	return r
}

// This middleware runs before the mux, so it sees the path exactly as it
// arrived — ServeMux's own cleaning has not happened yet, and cannot, because
// the redirect is sent first.
//
// A request for "//evil.example/" is parsed with the whole string as the path:
// ParseRequestURI does not treat a leading "//" as an authority the way
// url.Parse does. Trimming the trailing slash then yields "//evil.example",
// which in a Location header is protocol-relative — the browser resolves it
// against the current scheme and leaves the site entirely.
func TestTrailingSlashIsNotAnOpenRedirect(t *testing.T) {
	h := TrailingSlash(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	hostile := []string{
		"//evil.example/",
		"//evil.example//",
		"///evil.example/",
		"////evil.example/",
		"//evil.example/path/",
	}

	for _, target := range hostile {
		t.Run(target, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, wireRequest(t, target))

			loc := rec.Header().Get("Location")
			if strings.HasPrefix(loc, "//") {
				t.Errorf("Location %q is protocol-relative — this leaves the site", loc)
			}
			if loc != "" && !strings.HasPrefix(loc, "/") {
				t.Errorf("Location %q is not a same-site path", loc)
			}
		})
	}
}

func TestTrailingSlashStillRedirectsNormally(t *testing.T) {
	h := TrailingSlash(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	cases := map[string]string{
		"/om-itemize/":          "/om-itemize",
		"/arrangementer/?old=1": "/arrangementer?old=1",
	}
	for target, want := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, wireRequest(t, target))
		if rec.Code != http.StatusMovedPermanently {
			t.Errorf("%s: got %d, want 301", target, rec.Code)
		}
		if got := rec.Header().Get("Location"); got != want {
			t.Errorf("%s: Location = %q, want %q", target, got, want)
		}
	}
}

// The root must not be rewritten or redirected to itself in a loop.
func TestTrailingSlashLeavesRootAlone(t *testing.T) {
	h := TrailingSlash(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wireRequest(t, "/"))
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want the request to pass through", rec.Code)
	}
}

// logSink collects the structured records a middleware emits so a test can
// assert on the fields rather than on formatted text.
type logSink struct {
	buf bytes.Buffer
	log *slog.Logger
}

func newLogSink() *logSink {
	s := &logSink{}
	s.log = slog.New(slog.NewJSONHandler(&s.buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return s
}

// records decodes everything written so far. Failing here means the middleware
// produced no log line at all, which is worse than a wrong one: the request
// simply disappears from the record.
func (s *logSink) records(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(s.buf.Bytes()))
	for {
		var rec map[string]any
		if err := dec.Decode(&rec); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("log output is not valid JSON, so nothing downstream can parse it: %v", err)
		}
		out = append(out, rec)
	}
	return out
}

func (s *logSink) only(t *testing.T) map[string]any {
	t.Helper()
	recs := s.records(t)
	if len(recs) != 1 {
		t.Fatalf("got %d log records, want exactly one line per request", len(recs))
	}
	return recs[0]
}

// Every log line and panic report is correlated by this identifier. Without it
// in the context, an error in the logs cannot be tied to the request that
// caused it, which is the whole reason the middleware exists.
func TestRequestIDIsGeneratedAndPropagated(t *testing.T) {
	var fromContext string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fromContext = RequestIDFrom(r.Context())
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	header := rec.Header().Get("X-Request-Id")
	if header == "" {
		t.Fatal("no X-Request-Id header, so a visitor reporting a problem has no identifier to quote")
	}
	if len(header) != 16 {
		t.Errorf("X-Request-Id = %q (%d chars), want 16 hex characters", header, len(header))
	}
	if _, err := hex.DecodeString(header); err != nil {
		t.Errorf("X-Request-Id %q is not hex: %v", header, err)
	}
	if fromContext != header {
		t.Errorf("context carries %q but the client was told %q; the two cannot be matched up", fromContext, header)
	}
}

// Reusing an identifier across requests would make the logs actively
// misleading — two unrelated failures would look like one.
func TestRequestIDIsUniquePerRequest(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	seen := map[string]bool{}
	for i := range 200 {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		id := rec.Header().Get("X-Request-Id")
		if seen[id] {
			t.Fatalf("request %d reused the identifier %q", i, id)
		}
		seen[id] = true
	}
}

// A client-supplied X-Request-Id must not be echoed back: it is attacker
// controlled, ends up in the logs, and would let a caller collide two requests
// deliberately.
func TestRequestIDOverridesAClientSuppliedValue(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "injected\nSet-Cookie: a=b")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-Id"); strings.Contains(got, "injected") {
		t.Errorf("X-Request-Id = %q; a client-supplied value is being trusted", got)
	}
}

// A handler reached without the middleware — a test mux, or a background job
// reusing a bare context — must get an empty string rather than panic on a
// failed type assertion.
func TestRequestIDFromAContextWithoutOne(t *testing.T) {
	if got := RequestIDFrom(context.Background()); got != "" {
		t.Errorf("RequestIDFrom(background) = %q, want the empty string", got)
	}
	if got := RequestIDFrom(context.WithValue(context.Background(), requestIDKey, 42)); got != "" {
		t.Errorf("RequestIDFrom with a non-string value = %q, want the empty string", got)
	}
}

// The access log is the only record of what the server did. Each field is
// something an operator reads directly when a page is reported as broken.
func TestLoggerRecordsTheRequest(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		handler    http.HandlerFunc
		wantStatus float64
		wantBytes  float64
		wantLevel  string
	}{
		{
			name:   "an ordinary page",
			method: http.MethodGet,
			path:   "/om-itemize",
			handler: func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, "hei hei")
			},
			wantStatus: 200,
			wantBytes:  7,
			wantLevel:  "INFO",
		},
		{
			name:       "a handler that wrote nothing",
			method:     http.MethodGet,
			path:       "/",
			handler:    func(w http.ResponseWriter, r *http.Request) {},
			wantStatus: 200,
			wantLevel:  "INFO",
		},
		{
			name:   "a form submission",
			method: http.MethodPost,
			path:   "/registrer",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusSeeOther)
			},
			wantStatus: 303,
			wantLevel:  "INFO",
		},
		{
			name:   "a client error stays at info",
			method: http.MethodGet,
			path:   "/finnes-ikke",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "nope", http.StatusNotFound)
			},
			wantStatus: 404,
			wantBytes:  5,
			wantLevel:  "INFO",
		},
		{
			// A 5xx has to be loud enough to trip an alert; logging it at info
			// buries it among the successful requests.
			name:   "a server error is logged at error level",
			method: http.MethodGet,
			path:   "/arrangementer",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantStatus: 500,
			wantLevel:  "ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := newLogSink()
			h := Chain(tt.handler, RequestID, Logger(sink.log))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			line := sink.only(t)
			for field, want := range map[string]any{
				"msg":    "request",
				"method": tt.method,
				"path":   tt.path,
				"status": tt.wantStatus,
				"bytes":  tt.wantBytes,
				"level":  tt.wantLevel,
			} {
				if got := line[field]; got != want {
					t.Errorf("log field %q = %v, want %v", field, got, want)
				}
			}
			if id, _ := line["request_id"].(string); id != rec.Header().Get("X-Request-Id") {
				t.Errorf("log says request_id %q but the client was told %q", id, rec.Header().Get("X-Request-Id"))
			}
			if _, ok := line["took"]; !ok {
				t.Error("no duration in the log line, so a slow endpoint cannot be spotted")
			}
		})
	}
}

// Logger sits outside Recover so a recovered panic still leaves an access-log
// entry. Reverse them and the one request an operator most wants to find
// leaves no trace at all.
func TestLoggerRecordsRecoveredPanics(t *testing.T) {
	sink := newLogSink()
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("templates are on fire")
	}),
		RequestID,
		Logger(sink.log),
		Recover(sink.log, func(w http.ResponseWriter, r *http.Request, status int) {
			w.WriteHeader(status)
			io.WriteString(w, "beklager")
		}),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/arrangementer", nil))

	var access map[string]any
	for _, line := range sink.records(t) {
		if line["msg"] == "request" {
			access = line
		}
	}
	if access == nil {
		t.Fatal("a panicking request produced no access-log line")
	}
	if access["status"] != float64(http.StatusInternalServerError) {
		t.Errorf("access log reports status %v for a panicking request, want 500", access["status"])
	}
	if access["level"] != "ERROR" {
		t.Errorf("access log level = %v, want ERROR", access["level"])
	}
}

// A panic must become a 500 page rather than a dropped connection: the browser
// shows a network error for a dropped connection, which looks like the site is
// down rather than like one page is broken.
func TestRecoverTurnsPanicsIntoAnErrorPage(t *testing.T) {
	tests := []struct {
		name          string
		handler       http.HandlerFunc
		wantErrorPage bool
		wantStatus    int
		wantBody      string
	}{
		{
			name:          "panic before anything was written",
			handler:       func(w http.ResponseWriter, r *http.Request) { panic("boom") },
			wantErrorPage: true,
			wantStatus:    http.StatusInternalServerError,
			wantBody:      "feilside",
		},
		{
			name: "panic with a non-string value",
			handler: func(w http.ResponseWriter, r *http.Request) {
				panic(errors.New("nil map"))
			},
			wantErrorPage: true,
			wantStatus:    http.StatusInternalServerError,
			wantBody:      "feilside",
		},
		{
			name: "panic with a nil value",
			handler: func(w http.ResponseWriter, r *http.Request) {
				panic(nil)
			},
			wantErrorPage: true,
			wantStatus:    http.StatusInternalServerError,
			wantBody:      "feilside",
		},
		{
			// Half a page has already gone out; there is no way to replace it
			// with an error page, and trying would append error markup to a
			// partial document.
			name: "panic after the body has started",
			handler: func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, "halv side")
				panic("boom")
			},
			wantStatus: http.StatusOK,
			wantBody:   "halv side",
		},
		{
			name: "panic after the status was chosen",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
				panic("boom")
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name: "no panic at all",
			handler: func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, "alt bra")
			},
			wantStatus: http.StatusOK,
			wantBody:   "alt bra",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := newLogSink()
			pages := 0
			h := Recover(sink.log, func(w http.ResponseWriter, r *http.Request, status int) {
				pages++
				if status != http.StatusInternalServerError {
					t.Errorf("error page asked for status %d, want 500", status)
				}
				w.WriteHeader(status)
				io.WriteString(w, "feilside")
			})(tt.handler)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/arrangementer", nil))

			if (pages > 0) != tt.wantErrorPage {
				t.Errorf("error page rendered %d times, want rendered = %v", pages, tt.wantErrorPage)
			}
			if rec.Code != tt.wantStatus {
				t.Errorf("status %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

// The panic report is what an operator debugs from. Losing the stack or the
// request identifier turns a reproducible bug into a mystery.
func TestRecoverLogsEnoughToDebugFrom(t *testing.T) {
	sink := newLogSink()
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("templates are on fire")
	}),
		RequestID,
		Recover(sink.log, func(w http.ResponseWriter, r *http.Request, status int) { w.WriteHeader(status) }),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/arrangementer", nil))

	line := sink.only(t)
	if line["msg"] != "panic recovered" {
		t.Fatalf("log message = %v, want %q", line["msg"], "panic recovered")
	}
	if line["level"] != "ERROR" {
		t.Errorf("a panic was logged at %v, want ERROR", line["level"])
	}
	if got, _ := line["err"].(string); !strings.Contains(got, "templates are on fire") {
		t.Errorf("logged err = %q, want the panic value", got)
	}
	if got, _ := line["path"].(string); got != "/arrangementer" {
		t.Errorf("logged path = %q, want the request path", got)
	}
	if got, _ := line["request_id"].(string); got != rec.Header().Get("X-Request-Id") {
		t.Errorf("logged request_id = %q, want %q so the panic can be tied to the access log", got, rec.Header().Get("X-Request-Id"))
	}
	if got, _ := line["stack"].(string); !strings.Contains(got, "runtime/debug.Stack") {
		t.Errorf("logged stack = %.60q, want a real stack trace", got)
	}
}

// A client disconnecting mid-write is normal traffic, not a bug. net/http
// panics with ErrAbortHandler to tear the connection down quietly; swallowing
// it would fill the logs with noise and try to write an error page into a
// socket that is already gone.
func TestRecoverRepanicsOnAbortHandler(t *testing.T) {
	sink := newLogSink()
	pages := 0
	h := Recover(sink.log, func(w http.ResponseWriter, r *http.Request, status int) { pages++ })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic(http.ErrAbortHandler)
		}))

	defer func() {
		v := recover()
		if v != http.ErrAbortHandler {
			t.Errorf("recovered %v, want ErrAbortHandler to propagate so net/http can close the connection", v)
		}
		if pages != 0 {
			t.Error("an error page was rendered for a client that had already gone away")
		}
		if recs := sink.records(t); len(recs) != 0 {
			t.Errorf("a client disconnect produced %d log records, want none", len(recs))
		}
	}()

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	t.Error("ServeHTTP returned normally; ErrAbortHandler was swallowed")
}

// Every one of these headers is load-bearing. The CSP in particular is what
// makes the no-inline-script rule the templates follow actually enforced —
// without it, an injected <script> would run.
func TestSecurityHeaders(t *testing.T) {
	wantAlways := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"Permissions-Policy":     "geolocation=(), camera=(), microphone=()",
	}

	for _, https := range []bool{false, true} {
		t.Run("https="+strconv.FormatBool(https), func(t *testing.T) {
			h := SecurityHeaders(https)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

			for name, want := range wantAlways {
				if got := rec.Header().Get(name); got != want {
					t.Errorf("%s = %q, want %q", name, got, want)
				}
			}

			csp := rec.Header().Get("Content-Security-Policy")
			for _, directive := range []string{
				"default-src 'none'",
				"script-src 'self'",
				"style-src 'self'",
				"img-src 'self' data: https://cdn.discordapp.com",
				"font-src 'self'",
				"connect-src 'self'",
				"form-action 'self'",
				"base-uri 'none'",
				"frame-ancestors 'none'",
			} {
				if !strings.Contains(csp, directive) {
					t.Errorf("CSP is missing %q; got %q", directive, csp)
				}
			}
			if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
				t.Errorf("CSP allows inline code, which defeats the point of having one: %q", csp)
			}

			// Sending HSTS from a plaintext dev server pins localhost to https
			// in the developer's browser for a year, and it is not undone by
			// restarting the server.
			hsts := rec.Header().Get("Strict-Transport-Security")
			switch {
			case https && hsts != "max-age=31536000; includeSubDomains":
				t.Errorf("Strict-Transport-Security = %q over TLS, want a year with subdomains", hsts)
			case !https && hsts != "":
				t.Errorf("Strict-Transport-Security = %q was sent over plaintext", hsts)
			}
		})
	}
}

// Headers must be set before the handler runs, so they are already on the
// response by the time the handler calls WriteHeader.
func TestSecurityHeadersAreSetBeforeTheHandlerWrites(t *testing.T) {
	var seen string
	h := SecurityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = w.Header().Get("Content-Security-Policy")
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if seen == "" {
		t.Error("the handler saw no CSP; a handler that writes immediately would send the response without one")
	}
}

func TestCollapseLeadingSlashes(t *testing.T) {
	tests := map[string]string{
		"":                 "",
		"/":                "/",
		"/om-itemize":      "/om-itemize",
		"//":               "/",
		"///":              "/",
		"//evil.example":   "/evil.example",
		"///evil.example":  "/evil.example",
		"////a/b":          "/a/b",
		"/a//b":            "/a//b", // interior slashes are not our business
		"no-leading-slash": "no-leading-slash",
	}

	for in, want := range tests {
		t.Run(strconv.Quote(in), func(t *testing.T) {
			if got := collapseLeadingSlashes(in); got != want {
				t.Errorf("collapseLeadingSlashes(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

// The cookie is about 1.6 KB. Attaching it to every stylesheet, font and
// health probe would add that to requests that have nothing to do with pages.
func TestIsNonPage(t *testing.T) {
	tests := map[string]bool{
		"/healthz":               true,
		"/assets/app.abc123.css": true,
		"/assets/img/logo.png":   true,
		"/assets/":               true,
		"/icon.svg":              true,
		"/logo.png":              true,
		"/logo-192.png":          true,
		"/logo-512.png":          true,
		"/manifest.json":         true,
		"/robots.txt":            true,
		"/service-worker.js":     true,
		"/":                      false,
		"/om-itemize":            false,
		"/arrangementer":         false,
		"/assets":                false,
		"/healthz/":              false,
		"/not-assets/app.css":    false,
		"/img/logo.png":          false,
	}

	for path, want := range tests {
		t.Run(path, func(t *testing.T) {
			if got := isNonPage(path); got != want {
				t.Errorf("isNonPage(%q) = %v, want %v", path, got, want)
			}
		})
	}
}

func TestCookieRecipe(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		existing string
		wantSet  bool
	}{
		{name: "a page gets the cookie", path: "/om-itemize", wantSet: true},
		{name: "the front page gets the cookie", path: "/", wantSet: true},
		{name: "a stylesheet does not", path: "/assets/app.abc123.css"},
		{name: "the health probe does not", path: "/healthz"},
		{name: "a root file does not", path: "/robots.txt"},
		{name: "a visitor who already has it is left alone", path: "/", existing: cookieRecipe},
		{name: "a stale value is replaced", path: "/", existing: "old", wantSet: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := CookieRecipe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.existing != "" {
				req.AddCookie(&http.Cookie{Name: "cookies", Value: tt.existing})
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			var set *http.Cookie
			for _, c := range rec.Result().Cookies() {
				if c.Name == "cookies" {
					set = c
				}
			}
			if (set != nil) != tt.wantSet {
				t.Fatalf("cookie set = %v, want %v", set != nil, tt.wantSet)
			}
			if set == nil {
				return
			}
			if set.Value != cookieRecipe {
				t.Errorf("cookie value was mangled on the way out (%d bytes, want %d)", len(set.Value), len(cookieRecipe))
			}
			if set.Path != "/" {
				t.Errorf("cookie Path = %q, want / so it is visible on every page", set.Path)
			}
			if set.SameSite != http.SameSiteLaxMode {
				t.Errorf("cookie SameSite = %v, want Lax", set.SameSite)
			}
		})
	}
}

// The recipe is base64 of ISO-8859-1 bytes, carried over verbatim from the
// previous site. Re-encoding it as UTF-8 — the obvious "modernisation" — turns
// "smør" and "råsukker" into mojibake in every visitor's cookie jar.
//
// It is encoded with the URL-safe alphabet rather than the standard one, which
// is how it arrived from the 2022 site. That is worth pinning: the value
// cannot be read with a standard-alphabet decoder, so anyone poking at their
// cookie jar has to know to use the URL-safe one.
func TestCookieRecipeIsLatin1(t *testing.T) {
	if _, err := base64.StdEncoding.DecodeString(cookieRecipe); err == nil {
		t.Error("the value now decodes as standard base64; the URL-safe characters it inherited have been changed")
	}

	raw, err := base64.URLEncoding.DecodeString(cookieRecipe)
	if err != nil {
		t.Fatalf("the cookie value is not decodable at all, so the easter egg is just noise in the cookie jar: %v", err)
	}

	// Latin-1 decodes byte-for-byte to the matching code point.
	runes := make([]rune, len(raw))
	for i, b := range raw {
		runes[i] = rune(b)
	}
	recipe := string(runes)

	for _, want := range []string{"meierismør", "rårørsukker", "sjokolade", "hvetemel", "Forvarm ovnen"} {
		if !strings.Contains(recipe, want) {
			t.Errorf("the recipe does not decode to contain %q; it has been re-encoded as something other than ISO-8859-1", want)
		}
	}
	if utf8.Valid(raw) && strings.ContainsRune(recipe, 'ø') {
		t.Error("the payload is valid UTF-8 containing Latin-1 code points, so it has been double-encoded")
	}
}

// TrailingSlash sees the path before the mux does, so it also has to leave
// alone the paths that never have a trailing slash to strip.
func TestTrailingSlashPassesCanonicalPathsThrough(t *testing.T) {
	served := 0
	h := TrailingSlash(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served++
		w.WriteHeader(http.StatusOK)
	}))

	for _, target := range []string{"/", "/om-itemize", "/assets/app.abc.css", "/arrangementer?filter=kommende"} {
		t.Run(target, func(t *testing.T) {
			before := served
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, wireRequest(t, target))
			if served != before+1 {
				t.Errorf("the request was redirected instead of served; Location = %q", rec.Header().Get("Location"))
			}
		})
	}
}

// The query string has to survive the redirect, or a filtered listing silently
// loses its filter when reached through an old trailing-slash link.
func TestTrailingSlashKeepsTheQueryString(t *testing.T) {
	h := TrailingSlash(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	tests := map[string]string{
		"/arrangementer/?filter=kommende&side=2": "/arrangementer?filter=kommende&side=2",
		"/arrangementer//":                       "/arrangementer",
		"/a/b/c/":                                "/a/b/c",
		"/arrangementer/?":                       "/arrangementer?",
		// Nothing survives the trim here, and an empty Location would make the
		// browser reload the same path and loop forever.
		"//":   "/",
		"///":  "/",
		"////": "/",
	}
	for target, want := range tests {
		t.Run(target, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, wireRequest(t, target))
			if rec.Code != http.StatusMovedPermanently {
				t.Fatalf("got %d, want 301", rec.Code)
			}
			if got := rec.Header().Get("Location"); got != want {
				t.Errorf("Location = %q, want %q", got, want)
			}
		})
	}
}

// A panic before any output, with a gzip-capable client, must still produce a
// readable error page: nothing was compressed, so nothing may claim to be.
func TestRecoverInsideGzipProducesAReadablePage(t *testing.T) {
	sink := newLogSink()
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		panic("boom")
	}),
		Recover(sink.log, func(w http.ResponseWriter, r *http.Request, status int) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(status)
			io.WriteString(w, "<h1>Noe gikk galt</h1>")
		}),
		Gzip,
	)

	req := httptest.NewRequest(http.MethodGet, "/arrangementer", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500", rec.Code)
	}
	if enc := rec.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("Content-Encoding = %q on a body that was written outside the gzip wrapper; the browser would fail to decode it", enc)
	}
	if !strings.Contains(rec.Body.String(), "Noe gikk galt") {
		t.Errorf("the error page did not reach the client: %q", rec.Body.String())
	}
}
