package web_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ItemizeNTNU/website/assets"
	"github.com/ItemizeNTNU/website/internal/api"
	"github.com/ItemizeNTNU/website/internal/fusionauth"
	"github.com/ItemizeNTNU/website/internal/httpx"
	"github.com/ItemizeNTNU/website/internal/web"
)

// newMux builds the real routing table.
//
// ServeMux panics at registration when two patterns conflict — one matching a
// more specific path while the other matches more methods, say. That is a
// crash at container start, in production, on a Friday. Building the table
// here turns it into a test failure instead.
func newMux(t *testing.T) *http.ServeMux {
	t.Helper()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// Embedded rather than on-disk: the test binary's working directory is the
	// package, not the repository root.
	fsys := assets.FS(false)

	assetServer, err := httpx.NewAssets(fsys, false)
	if err != nil {
		t.Fatalf("building assets: %v", err)
	}
	site, err := web.NewServer(fsys, assetServer, nil, nil, fusionauth.New("https://auth.example", ""), nil, "https://itemize.no", log, false)
	if err != nil {
		t.Fatalf("building server: %v", err)
	}

	mux := http.NewServeMux()
	assetServer.Register(mux)
	site.Routes(mux)
	// The API registers a catch-all under /api/, which is the pattern most
	// likely to collide with the site's own.
	api.NewServer(nil, fusionauth.New("https://auth.example", ""), "https://itemize.no", log).Routes(mux)
	return mux
}

func TestRoutesRegisterWithoutConflict(t *testing.T) {
	newMux(t) // panics on a conflicting pattern
}

func TestPagesRender(t *testing.T) {
	mux := newMux(t)

	// Every page that does not need a database or a signed-in user.
	paths := []string{
		"/", "/om-itemize", "/historie", "/for-bedrifter",
		"/ressurser", "/utmelding", "/registrert", "/arrangementer", "/registrer",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200", rec.Code)
			}
			if body := rec.Body.String(); len(body) < 500 {
				t.Errorf("suspiciously short response (%d bytes)", len(body))
			}
		})
	}
}

// The addresses must not appear in the markup at all — that is the whole point
// of assembling them in the browser.
func TestNoPlaintextEmailInMarkup(t *testing.T) {
	mux := newMux(t)

	for _, path := range []string{"/", "/om-itemize", "/for-bedrifter", "/utmelding"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		if body := rec.Body.String(); containsAddress(body) {
			t.Errorf("%s leaks a plaintext address into the markup", path)
		}
	}
}

func containsAddress(body string) bool {
	// Deliberately not a full address parser: this is looking for the literal
	// "@itemize.no" that a scraper's regular expression would find.
	const needle = "@itemize.no"
	for i := 0; i+len(needle) <= len(body); i++ {
		if body[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// The flag fragment is meant to be in the source and not on the page. A
// refactor that drops the hidden attribute, or the template branch, would
// otherwise go unnoticed.
func TestResourcesKeepsHiddenFlag(t *testing.T) {
	mux := newMux(t)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ressurser", nil))

	const want = `<p id="flag-part" hidden>b9d6b360c532f30e9cf97a9e84e64479</p>`
	if !contains(rec.Body.String(), want) {
		t.Errorf("the CTF flag fragment is missing or has changed shape; expected %q", want)
	}
}

func TestRedirects(t *testing.T) {
	mux := newMux(t)

	cases := map[string]string{
		"/qr":                          "/arrangementer",
		"/register":                    "/arrangementer",
		"/registrering":                "/arrangementer",
		"/påmelding":                   "/arrangementer",
		"/den-ekte-registreringssiden": "/registrer",
	}
	for from, to := range cases {
		t.Run(from, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, from, nil))

			if rec.Code != http.StatusMovedPermanently {
				t.Fatalf("got %d, want 301", rec.Code)
			}
			if got := rec.Header().Get("Location"); got != to {
				t.Errorf("redirected to %q, want %q", got, to)
			}
		})
	}
}

func TestUnknownPathIs404(t *testing.T) {
	mux := newMux(t)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ingen-slik-side", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// The registration form's conditional fields work with no JavaScript at all,
// via `#type-x:checked ~ .typepick__fields > [data-when~="x"]` in the
// stylesheet. That selector only reaches the fields if the radios are sibling
// elements *preceding* the container — wrap them in a div for tidiness and
// every conditional field silently disappears, with nothing failing to say so.
//
// This pins the structure the stylesheet depends on.
func TestRegistrationConditionalFieldStructure(t *testing.T) {
	mux := newMux(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/registrer", nil))
	body := rec.Body.String()

	radios := indexOf(body, `id="type-student"`)
	container := indexOf(body, `class="typepick__fields"`)
	switch {
	case radios < 0:
		t.Fatal("the membership-type radios are missing")
	case container < 0:
		t.Fatal("the conditional-field container is missing")
	case radios > container:
		t.Fatal("the radios come after .typepick__fields; the sibling selector " +
			"cannot reach it and every conditional field will stay hidden")
	}

	// Each membership type needs at least one field that appears for it.
	for _, want := range []string{
		`data-when="student alumni"`, // study programme, shared
		`data-when="student"`,
		`data-when="alumni"`,
		`data-when="employee"`,
	} {
		if !contains(body, want) {
			t.Errorf("no field marked %s", want)
		}
	}

	// All three radios must share the name, or the browser will not treat them
	// as one group and more than one could be selected at once.
	if n := count(body, `name="type"`); n != 3 {
		t.Errorf(`found %d inputs named "type", want 3`, n)
	}
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func count(haystack, needle string) int {
	n := 0
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			n++
		}
	}
	return n
}
