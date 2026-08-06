package web_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/events"
)

// checkinCode is the canonical UUID form the service assigns and the QR codes
// in circulation carry.
const checkinCode = "11111111-2222-4333-8444-555555555555"

// checkinRepo returns a repository holding one event reachable by checkinCode.
// The date is far in the future so Past() stays false for these tests' lives.
func checkinRepo() *memRepo {
	repo := newMemRepo()
	start := time.Date(2098, 9, 1, 17, 15, 0, 0, time.UTC)
	repo.seed(&events.Event{
		ID:       bson.NewObjectID(),
		Name:     "Pizza og CTF",
		Location: events.Place{Name: "Savannen"},
		Date:     start,
		Duration: 3,
		End:      start.Add(3 * time.Hour),
		Info:     "Ta med laptop.",
		CheckIn:  events.CheckIn{Code: checkinCode},
	})
	return repo
}

// The QR page is the board's view of the door: it must be reachable by the
// board only, and fail usefully for everyone and everything else.
func TestInnsjekkAccess(t *testing.T) {
	tests := []struct {
		name       string
		repo       *memRepo // nil means the database is down
		user       *auth.User
		path       string
		wantStatus int
	}{
		{"anonymous is sent to log in", checkinRepo(), nil, "/innsjekk/" + checkinCode, http.StatusFound},
		{"a member without the board role is refused", checkinRepo(), member, "/innsjekk/" + checkinCode, http.StatusForbidden},
		{"no database yields 503, not an empty page", nil, styret, "/innsjekk/" + checkinCode, http.StatusServiceUnavailable},
		{"an unknown code is 404", checkinRepo(), styret, "/innsjekk/99999999-8888-4777-8666-555555555555", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := siteConfig{}
			if tt.repo != nil {
				cfg.repo = tt.repo
			}
			mux := newSite(t, cfg)

			rec := get(t, mux, tt.path, tt.user)
			if rec.Code != tt.wantStatus {
				t.Fatalf("got %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusFound {
				if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/login?return_to=") {
					t.Errorf("redirected to %q; an anonymous board member would not come back here after logging in", loc)
				}
				return
			}
			// Refusals render the styled error page, not a blank response that
			// looks like the site broke.
			if len(rec.Body.String()) == 0 {
				t.Error("the error response has no body, so the visitor sees a blank page")
			}
		})
	}
}

func TestInnsjekkShowsScannableCodeToBoard(t *testing.T) {
	mux := newSite(t, siteConfig{repo: checkinRepo()})

	rec := get(t, mux, "/innsjekk/"+checkinCode, styret)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	// A code on screen is a code someone can register attendance with; a
	// shared cache or history store must never keep a copy.
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store — the check-in code could be replayed from a cache", got)
	}

	body := rec.Body.String()
	// The URL in the QR code must be absolute: a phone camera scanning it has
	// no origin to resolve a relative path against.
	if want := "https://itemize.no/innsjekk-qr/" + checkinCode; !contains(body, want) {
		t.Errorf("the page does not carry the scan URL %q, so the printed QR points nowhere", want)
	}
	if !contains(body, "<svg") {
		t.Error("no inline SVG on the page — there is nothing to hold up at the door")
	}
}

func TestInnsjekkQRRequiresLogin(t *testing.T) {
	mux := newSite(t, siteConfig{repo: checkinRepo()})

	rec := get(t, mux, "/innsjekk-qr/"+checkinCode, nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("got %d, want a redirect to login", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/login?return_to=") {
		t.Errorf("redirected to %q; the scan would be lost instead of resumed after login", loc)
	}
}

func TestInnsjekkQRChecksTheMemberIn(t *testing.T) {
	repo := checkinRepo()
	mux := newSite(t, siteConfig{repo: repo})

	rec := get(t, mux, "/innsjekk-qr/"+checkinCode, member)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}

	if len(repo.attendances) != 1 {
		t.Fatalf("recorded %d attendances, want exactly 1", len(repo.attendances))
	}
	// The attendance list is read by people afterwards, so it must carry the
	// real name, not a display name or a bare identifier.
	if got := repo.attendances[0].Name; got != "Kari Nordmann" {
		t.Errorf("attendance name = %q, want the full name", got)
	}
	if got := repo.attendances[0].UserID; got != member.ID {
		t.Errorf("attendance user id = %q, want %q", got, member.ID)
	}

	if !contains(rec.Body.String(), "Du er sjekket inn.") {
		t.Error("the page does not confirm the check-in; the member cannot tell whether the scan worked")
	}
	// The response embeds an attendance write behind a GET; it must never be
	// cached or leak the code onward via the referrer.
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy = %q, want no-referrer — the code would leak to any linked site", got)
	}
}

// The privacy headers are set before the code lookup, so even a 404 for a
// guessed code carries them.
func TestInnsjekkQRUnknownCodeKeepsPrivacyHeaders(t *testing.T) {
	mux := newSite(t, siteConfig{repo: checkinRepo()})

	rec := get(t, mux, "/innsjekk-qr/99999999-8888-4777-8666-555555555555", member)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store even on the 404 branch", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy = %q, want no-referrer even on the 404 branch", got)
	}
}

func TestInnsjekkQRWriteOutcomes(t *testing.T) {
	tests := []struct {
		name         string
		addErr       error
		wantStatus   int
		wantContains string
		notContains  string
	}{
		{
			// A double-scan at the door must not read as a failure — the
			// member did nothing wrong and is, in fact, checked in.
			name:         "scanning twice reads as success",
			addErr:       events.ErrAlreadyCheckedIn,
			wantStatus:   http.StatusOK,
			wantContains: "allerede sjekket inn",
		},
		{
			name:       "an event deleted mid-scan is 404",
			addErr:     events.ErrNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			// Deliberate UX: a database failure at the door still renders a
			// calm 200 page telling them who to talk to, rather than an error
			// page that looks like they broke something.
			name:         "a storage failure stays a calm page",
			addErr:       errors.New("mongo is down"),
			wantStatus:   http.StatusOK,
			wantContains: "Si fra til noen i styret",
			notContains:  "Du er sjekket inn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := checkinRepo()
			repo.addErr = tt.addErr
			mux := newSite(t, siteConfig{repo: repo})

			rec := get(t, mux, "/innsjekk-qr/"+checkinCode, member)
			if rec.Code != tt.wantStatus {
				t.Fatalf("got %d, want %d", rec.Code, tt.wantStatus)
			}
			body := rec.Body.String()
			if tt.wantContains != "" && !contains(body, tt.wantContains) {
				t.Errorf("the page does not say %q", tt.wantContains)
			}
			if tt.notContains != "" && contains(body, tt.notContains) {
				t.Errorf("the page claims %q despite the write failing — the attendance was never stored", tt.notContains)
			}
		})
	}
}
