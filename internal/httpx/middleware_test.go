package httpx

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
