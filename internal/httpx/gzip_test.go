package httpx

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// pageBody is large enough that gzip buffers rather than emitting everything
// on the first Write. That matters: a middleware that forgets to Close the
// gzip writer still produces a plausible-looking response for a few bytes, and
// only loses the tail once the deflate window fills.
var pageBody = strings.Repeat("Itemize er linjeforeningen for informasjonssikkerhet ved NTNU. ", 500)

// gunzip decodes a response body, failing with the consequence a visitor would
// see rather than a bare decode error.
func gunzip(t *testing.T, b []byte) string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("response is labelled Content-Encoding: gzip but is not a gzip stream, so the browser shows nothing: %v", err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gzip stream is truncated after %d bytes — the page would render half-finished: %v", len(out), err)
	}
	if err := zr.Close(); err != nil {
		t.Fatalf("gzip stream has no valid trailer, so a strict client rejects the whole response: %v", err)
	}
	return string(out)
}

func gzipRequest(target, acceptEncoding string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	if acceptEncoding != "" {
		r.Header.Set("Accept-Encoding", acceptEncoding)
	}
	return r
}

// A shared cache keys on Accept-Encoding only when Vary says so. Get this
// wrong and a proxy hands a gzipped body to a client that asked for identity,
// which renders as binary garbage for everyone behind that cache.
func TestAcceptsGzip(t *testing.T) {
	tests := []struct {
		header string
		want   bool
	}{
		{"", false},
		{"gzip", true},
		{"GZIP", true},
		{"deflate, gzip", true},
		{"gzip, deflate, br", true},
		{"br;q=1.0, gzip;q=0.8", true},
		{"  gzip,deflate  ", true},
		// RFC 9110 permits whitespace around the ";" that introduces the
		// q-value. Only the space before the whole coding is trimmed here, so
		// a client writing it this way is served uncompressed. Pinned so the
		// deviation is visible rather than accidental.
		{"gzip ; q=1.0", false},
		{"identity", false},
		{"deflate", false},
		{"br", false},
		{"*", false}, // a wildcard is not an explicit gzip offer
		{"x-gzip", false},
		{"gzipper", false},
		{",,,", false},
		{";q=1", false},
		// RFC 9110 gives q=0 the meaning "not acceptable". The parser here
		// ignores q-values entirely, so this client is sent gzip anyway. Pinned
		// so the deviation is visible rather than accidental.
		{"gzip;q=0", true},
	}

	for _, tt := range tests {
		t.Run(strconv.Quote(tt.header), func(t *testing.T) {
			if got := acceptsGzip(gzipRequest("/", tt.header)); got != tt.want {
				t.Errorf("Accept-Encoding %q: acceptsGzip = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

// Running an already-compressed payload through gzip costs CPU on every
// request and usually makes the response marginally larger, so the classifier
// has to reject those media types even though they are perfectly valid input.
func TestCompressible(t *testing.T) {
	tests := []struct {
		contentType string
		want        bool
	}{
		{"text/html; charset=utf-8", true},
		{"text/css; charset=utf-8", true},
		{"text/javascript; charset=utf-8", true},
		{"text/plain", true},
		{"TEXT/HTML", true},
		{"  text/html  ; charset=utf-8", true},
		{"image/svg+xml", true},
		{"application/json", true},
		{"application/javascript", true},
		{"application/xml", true},
		{"application/xhtml+xml", true},
		{"application/manifest+json", true},
		{"application/rss+xml", true},
		{"application/atom+xml", true},
		{"image/png", false},
		{"image/jpeg", false},
		{"image/webp", false},
		{"font/woff2", false},
		{"audio/mpeg", false},
		{"video/mp4", false},
		{"application/octet-stream", false},
		{"application/pdf", false},
		{"application/zip", false},
		{"", false},
		{"; charset=utf-8", false},
	}

	for _, tt := range tests {
		t.Run(strconv.Quote(tt.contentType), func(t *testing.T) {
			if got := compressible(tt.contentType); got != tt.want {
				t.Errorf("compressible(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

// The end-to-end shape of the middleware: what gets encoded, what is left
// alone, and whether the Vary header that keeps caches honest is present.
func TestGzipMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		accept      string
		contentType string
		preEncoding string
		status      int
		wantEncoded bool
		wantVary    bool
	}{
		{
			name:        "html for a client that asked for gzip",
			accept:      "gzip",
			contentType: "text/html; charset=utf-8",
			wantEncoded: true,
			wantVary:    true,
		},
		{
			name:        "html for a client that did not ask",
			contentType: "text/html; charset=utf-8",
		},
		{
			name:        "client that only accepts identity",
			accept:      "identity",
			contentType: "text/html; charset=utf-8",
		},
		{
			name:        "png is already compressed",
			accept:      "gzip",
			contentType: "image/png",
			wantVary:    true,
		},
		{
			name:        "json is worth compressing",
			accept:      "gzip",
			contentType: "application/json",
			wantEncoded: true,
			wantVary:    true,
		},
		{
			name:        "svg is text underneath",
			accept:      "gzip",
			contentType: "image/svg+xml",
			wantEncoded: true,
			wantVary:    true,
		},
		{
			name:        "handler that set no content type",
			accept:      "gzip",
			contentType: "",
			wantVary:    true,
		},
		{
			// The health probe is polled constantly by the orchestrator and its
			// body is a handful of bytes; compressing it is pure overhead.
			name:        "health probe is exempt",
			path:        "/healthz",
			accept:      "gzip",
			contentType: "text/plain; charset=utf-8",
		},
		{
			// Static assets arrive pre-compressed from Assets.ServeHTTP.
			// Re-encoding them would produce a double-gzipped body no browser
			// can read.
			name:        "handler already encoded the payload",
			accept:      "gzip",
			contentType: "text/css; charset=utf-8",
			preEncoding: "gzip",
			wantVary:    true,
		},
		{
			name:        "204 has no body to compress",
			accept:      "gzip",
			contentType: "text/html; charset=utf-8",
			status:      http.StatusNoContent,
			wantVary:    true,
		},
		{
			name:        "304 has no body to compress",
			accept:      "gzip",
			contentType: "text/html; charset=utf-8",
			status:      http.StatusNotModified,
			wantVary:    true,
		},
		{
			name:        "error pages are compressed like any other page",
			accept:      "gzip",
			contentType: "text/html; charset=utf-8",
			status:      http.StatusNotFound,
			wantEncoded: true,
			wantVary:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.contentType != "" {
					w.Header().Set("Content-Type", tt.contentType)
				}
				if tt.preEncoding != "" {
					w.Header().Set("Content-Encoding", tt.preEncoding)
				}
				if tt.status != 0 {
					w.WriteHeader(tt.status)
				}
				io.WriteString(w, pageBody)
			}))

			path := tt.path
			if path == "" {
				path = "/om-itemize"
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, gzipRequest(path, tt.accept))

			enc := rec.Header().Get("Content-Encoding")
			switch {
			case tt.wantEncoded && enc != "gzip":
				t.Fatalf("Content-Encoding = %q, want gzip; the response is going out uncompressed", enc)
			case !tt.wantEncoded && tt.preEncoding == "" && enc != "":
				t.Fatalf("Content-Encoding = %q on a response that must not be re-encoded", enc)
			}

			if tt.wantEncoded {
				if got := gunzip(t, rec.Body.Bytes()); got != pageBody {
					t.Errorf("decompressed body is %d bytes, want the handler's %d", len(got), len(pageBody))
				}
				if rec.Body.Len() >= len(pageBody) {
					t.Errorf("compressed body is %d bytes against %d uncompressed; compression is not happening",
						rec.Body.Len(), len(pageBody))
				}
			} else if got := rec.Body.String(); got != pageBody {
				t.Errorf("body was altered on a response that should have passed through untouched (%d bytes, want %d)",
					len(got), len(pageBody))
			}

			vary := rec.Header().Values("Vary")
			hasVary := slicesContainsFold(vary, "Accept-Encoding")
			if hasVary != tt.wantVary {
				t.Errorf("Vary: %v (Accept-Encoding present = %v), want present = %v; a shared cache would serve the wrong encoding",
					vary, hasVary, tt.wantVary)
			}
		})
	}
}

func slicesContainsFold(values []string, want string) bool {
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), want) {
				return true
			}
		}
	}
	return false
}

// Content-Length describes the bytes on the wire. Leaving the identity length
// in place on a compressed response makes the client wait for bytes that never
// arrive, and the connection hangs until it times out.
func TestGzipRewritesLengthAndValidators(t *testing.T) {
	tests := []struct {
		name       string
		accept     string
		etag       string
		wantEtag   string
		wantLength bool
	}{
		{
			name:     "compressed response drops Content-Length and weakens the ETag",
			accept:   "gzip",
			etag:     `"a1b2c3"`,
			wantEtag: `W/"a1b2c3"`,
		},
		{
			name:     "an already weak ETag is not weakened twice",
			accept:   "gzip",
			etag:     `W/"a1b2c3"`,
			wantEtag: `W/"a1b2c3"`,
		},
		{
			name:       "uncompressed response keeps both untouched",
			accept:     "identity",
			etag:       `"a1b2c3"`,
			wantEtag:   `"a1b2c3"`,
			wantLength: true,
		},
		{
			name:   "no ETag to weaken",
			accept: "gzip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Content-Length", strconv.Itoa(len(pageBody)))
				if tt.etag != "" {
					w.Header().Set("ETag", tt.etag)
				}
				io.WriteString(w, pageBody)
			}))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, gzipRequest("/", tt.accept))

			gotLength := rec.Header().Get("Content-Length")
			if tt.wantLength && gotLength == "" {
				t.Error("Content-Length was dropped from an uncompressed response for no reason")
			}
			if !tt.wantLength && gotLength != "" {
				t.Errorf("Content-Length = %q describes the uncompressed body; the client would wait for bytes that never come", gotLength)
			}
			if got := rec.Header().Get("ETag"); got != tt.wantEtag {
				t.Errorf("ETag = %q, want %q; a strong validator over bytes that changed lets a cache serve a stale body", got, tt.wantEtag)
			}
		})
	}
}

// A handler is free to call WriteHeader explicitly, or to let the first Write
// imply a 200. Both have to reach the client with the right status, and the
// compression decision is made at that moment.
func TestGzipHeaderOrdering(t *testing.T) {
	t.Run("explicit WriteHeader before Write", func(t *testing.T) {
		h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, pageBody)
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, gzipRequest("/", "gzip"))

		if rec.Code != http.StatusCreated {
			t.Errorf("status %d, want the handler's 201", rec.Code)
		}
		if got := gunzip(t, rec.Body.Bytes()); got != pageBody {
			t.Error("body did not survive compression")
		}
	})

	t.Run("a second WriteHeader is ignored", func(t *testing.T) {
		h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, pageBody)
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, gzipRequest("/", "gzip"))

		if rec.Code != http.StatusNotFound {
			t.Errorf("status %d, want the first status written (404)", rec.Code)
		}
	})

	t.Run("a handler that writes nothing", func(t *testing.T) {
		h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, gzipRequest("/", "gzip"))

		if rec.Code != http.StatusOK {
			t.Errorf("status %d, want 200", rec.Code)
		}
		if enc := rec.Header().Get("Content-Encoding"); enc != "" {
			t.Errorf("Content-Encoding = %q on an empty response, which promises a gzip stream that was never written", enc)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("body has %d bytes, want none", rec.Body.Len())
		}
	})

	t.Run("headers set after the first write do not take effect", func(t *testing.T) {
		// The compression decision is locked in at WriteHeader, so a handler
		// that sets Content-Type late is served uncompressed. Worth pinning:
		// it is the reason every handler in this program sets it first.
		h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, pageBody)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, gzipRequest("/", "gzip"))

		if enc := rec.Header().Get("Content-Encoding"); enc != "" {
			t.Errorf("Content-Encoding = %q, want none: the type was unknown when the decision was made", enc)
		}
		if rec.Body.String() != pageBody {
			t.Error("body was altered")
		}
	})
}

// Flush has to push bytes through both the gzip writer and the writer beneath
// it. If only one of the two is flushed, a streaming response sits in a buffer
// and the client sees nothing until the handler returns.
func TestGzipFlushReachesTheClient(t *testing.T) {
	released := make(chan struct{})
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, "first half. ")
		w.(http.Flusher).Flush()
		close(released)
		io.WriteString(w, "second half.")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, gzipRequest("/", "gzip"))

	<-released
	if !rec.Flushed {
		t.Error("Flush never reached the underlying writer; a streamed response would stall until the handler returned")
	}
	if got := gunzip(t, rec.Body.Bytes()); got != "first half. second half." {
		t.Errorf("body after a mid-stream flush = %q, want both halves", got)
	}
}

// Wrapping a ResponseWriter hides the optional interfaces the real one
// implements. Unwrap is what http.ResponseController follows to reach them, so
// without it a handler setting a write deadline silently gets nothing.
func TestGzipUnwrapKeepsResponseControllerWorking(t *testing.T) {
	rec := httptest.NewRecorder()
	gw := &gzipResponseWriter{ResponseWriter: rec}
	if gw.Unwrap() != http.ResponseWriter(rec) {
		t.Error("Unwrap returned a different writer, so ResponseController would operate on the wrong one")
	}

	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, "hei")
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("ResponseController.Flush through the gzip wrapper failed: %v", err)
		}
	}))
	h.ServeHTTP(httptest.NewRecorder(), gzipRequest("/", "gzip"))
}

// The gzip writers are pooled, and a Reset that does not fully clear the
// previous request's state leaks bytes from one visitor's page into another's.
// Sequential reuse catches the plain version of that; the parallel run under
// -race catches a writer handed to two requests at once.
func TestGzipPooledWritersDoNotLeakBetweenRequests(t *testing.T) {
	serve := func(body string) *httptest.ResponseRecorder {
		h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, body)
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, gzipRequest("/", "gzip"))
		return rec
	}

	t.Run("sequential reuse", func(t *testing.T) {
		for i := range 25 {
			body := strings.Repeat("side "+strconv.Itoa(i)+" ", 200)
			if got := gunzip(t, serve(body).Body.Bytes()); got != body {
				t.Fatalf("request %d got a body belonging to another request", i)
			}
		}
	})

	t.Run("concurrent reuse", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := range 50 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				body := strings.Repeat("samtidig "+strconv.Itoa(i)+" ", 200)
				rec := serve(body)
				if got := gunzip(t, rec.Body.Bytes()); got != body {
					t.Errorf("concurrent request %d received %d bytes that are not its own", i, len(got))
				}
			}()
		}
		wg.Wait()
	})
}

// The deferred Close in Gzip is what finishes the stream. This is the canary
// for it: without that call the tail of every compressed page stays in the
// deflate buffer and the trailer is never written, so the browser gets a
// truncated document. If this ever stops failing, the Close assertions
// elsewhere in this file have quietly become vacuous.
func TestGzipWithoutCloseTheStreamIsIncomplete(t *testing.T) {
	rec := httptest.NewRecorder()
	gw := &gzipResponseWriter{ResponseWriter: rec}
	gw.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(gw, pageBody)

	zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		return // not even a readable header yet, which is incomplete enough
	}
	if out, err := io.ReadAll(zr); err == nil && string(out) == pageBody {
		t.Error("an unclosed gzip writer produced a complete stream; Close is no longer load-bearing and the tests that assert it prove nothing")
	}
}

// Close is called through defer, including when the handler panics on the way
// out. Calling it a second time must not double-return the writer to the pool,
// which would hand the same writer to two future requests.
func TestGzipCloseIsIdempotent(t *testing.T) {
	rec := httptest.NewRecorder()
	gw := &gzipResponseWriter{ResponseWriter: rec}
	gw.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(gw, pageBody)

	gw.Close()
	gw.Close() // must not panic or re-pool

	if got := gunzip(t, rec.Body.Bytes()); got != pageBody {
		t.Error("body did not survive a double Close")
	}
}

// Close on a response that was never compressed has nothing to flush; the
// deferred call in Gzip runs for every request, compressed or not.
func TestGzipCloseWithoutCompressionIsANoOp(t *testing.T) {
	rec := httptest.NewRecorder()
	gw := &gzipResponseWriter{ResponseWriter: rec}
	gw.Header().Set("Content-Type", "image/png")
	io.WriteString(gw, "binary")
	gw.Close()

	if rec.Body.String() != "binary" {
		t.Errorf("body = %q, want the bytes the handler wrote", rec.Body.String())
	}
}
