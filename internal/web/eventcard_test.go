package web_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ItemizeNTNU/website/assets"
	"github.com/ItemizeNTNU/website/internal/events"
	"github.com/ItemizeNTNU/website/internal/fusionauth"
	"github.com/ItemizeNTNU/website/internal/httpx"
	"github.com/ItemizeNTNU/website/internal/web"
)

// stubRepo returns a fixed list, so the events page actually renders cards.
type stubRepo struct {
	events.Repository
	list []events.Event
}

func (s stubRepo) List(context.Context, events.Filter) ([]events.Event, error) {
	return s.list, nil
}

// The event card was never exercised: every existing test ran with a nil
// repository, which renders the "unavailable" branch and no cards at all. A
// template error inside the card — a method that cannot be called, a renamed
// field — therefore passed the whole suite and only appeared in production as
// a 500.
//
// This renders a real card. It would have caught the pointer-receiver
// regression, where boxing the value in a map made every accessor
// unreachable and the page failed with "can't evaluate field Past".
func TestEventCardRenders(t *testing.T) {
	start := time.Now().Add(48 * time.Hour)
	repo := stubRepo{list: []events.Event{{
		ID:          bson.NewObjectID(),
		Name:        "Pizza, øl og CTF",
		Location:    events.Place{Name: "Savannen", URL: "https://use.mazemap.com/x"},
		RegisterURL: "https://example.no/pamelding",
		Date:        start,
		Duration:    3,
		End:         start.Add(3 * time.Hour),
		CTF:         events.Place{Name: "picoCTF", URL: "https://play.picoctf.org"},
		Info:        "Ta med laptop.\nVi starter 17:15.",
		CheckIn:     events.CheckIn{Code: "3f8a1c2e-0000-4aaa-bbbb-ccccddddeeee"},
	}}}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	fsys := assets.FS(false)
	assetServer, err := httpx.NewAssets(fsys, false)
	if err != nil {
		t.Fatal(err)
	}
	site, err := web.NewServer(fsys, assetServer, repo, nil,
		fusionauth.New("https://auth.example", ""), nil, testSealer, "https://itemize.no", log, false)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	site.Routes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/arrangementer", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 — the card template probably failed to execute", rec.Code)
	}

	body := rec.Body.String()
	// Each of these comes from a different accessor, so a receiver or naming
	// mistake in any one of them fails here rather than in production.
	for _, want := range []string{
		"Pizza, øl og CTF",          // Name
		"Savannen",                  // Location
		"https://use.mazemap.com/x", // Location.URL
		"https://play.picoctf.org",  // CTF.URL
		"Ta med laptop.",            // Info, escaped
		`<time datetime="`,          // WhenISO
		`class="event__info"`,       // the non-colliding class
	} {
		if !contains(body, want) {
			t.Errorf("the rendered card is missing %q", want)
		}
	}

	// Board-only controls must not appear for an anonymous visitor.
	for _, forbidden := range []string{"/innsjekk/", "?rediger=", "3f8a1c2e-0000"} {
		if contains(body, forbidden) {
			t.Errorf("%q leaked to an anonymous visitor", forbidden)
		}
	}
}
