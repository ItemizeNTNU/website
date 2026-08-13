package web_test

import (
	"net/http"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/events"
)

// eventsPageRepo holds one visible and one hidden (draft) event, both far in
// the future so Past() stays false.
func eventsPageRepo() (*memRepo, *events.Event, *events.Event) {
	repo := newMemRepo()
	start := time.Date(2098, 9, 1, 17, 15, 0, 0, time.UTC)

	visible := &events.Event{
		ID:       bson.NewObjectID(),
		Name:     "Pizza og CTF",
		Location: events.Place{Name: "Savannen"},
		Date:     start,
		Duration: 3,
		End:      start.Add(3 * time.Hour),
		Info:     "Ta med laptop.",
		CheckIn:  events.CheckIn{Code: "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"},
	}
	hidden := &events.Event{
		ID:       bson.NewObjectID(),
		Name:     "Skjult verksted",
		Location: events.Place{Name: "Savannen"},
		Date:     start.Add(24 * time.Hour),
		Duration: 2,
		End:      start.Add(26 * time.Hour),
		Info:     "Bare et utkast.",
		Hidden:   true,
		CheckIn:  events.CheckIn{Code: "bbbbbbbb-cccc-4ddd-8eee-ffffffffffff"},
	}
	repo.seed(visible)
	repo.seed(hidden)
	return repo, visible, hidden
}

// Drafts are for the board only. This is the hidden-leak regression test: it
// pins both the filter the repository is asked for and that a draft's name
// never reaches an anonymous visitor's markup.
func TestArrangementerHidesDraftsFromVisitors(t *testing.T) {
	tests := []struct {
		name       string
		user       *auth.User
		wantHidden bool
	}{
		{"anonymous never sees drafts", nil, false},
		{"the board sees drafts", styret, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _, hidden := eventsPageRepo()
			mux := newSite(t, siteConfig{repo: repo})

			rec := get(t, mux, "/arrangementer", tt.user)
			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200", rec.Code)
			}
			if repo.gotFilter.IncludeHidden != tt.wantHidden {
				t.Errorf("List was asked for IncludeHidden=%v, want %v — the visibility decision belongs to the query, not the template",
					repo.gotFilter.IncludeHidden, tt.wantHidden)
			}

			body := rec.Body.String()
			if got := contains(body, hidden.Name); got != tt.wantHidden {
				if tt.wantHidden {
					t.Error("the board cannot see its own draft on the page")
				} else {
					t.Error("a draft event's name leaked into an anonymous visitor's markup")
				}
			}
		})
	}
}

// "Show past events" is a query parameter so the view is bookmarkable; the
// shell line names which view this is.
func TestArrangementerOldToggle(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantOld     bool
		wantCommand string
	}{
		{"upcoming by default", "/arrangementer", false, `data-cmd="./events --upcoming"`},
		{"?old includes finished events", "/arrangementer?old=1", true, `data-cmd="./events --all"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _, _ := eventsPageRepo()
			mux := newSite(t, siteConfig{repo: repo})

			rec := get(t, mux, tt.path, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200", rec.Code)
			}
			if repo.gotFilter.IncludeOld != tt.wantOld {
				t.Errorf("List was asked for IncludeOld=%v, want %v", repo.gotFilter.IncludeOld, tt.wantOld)
			}
			if !contains(rec.Body.String(), tt.wantCommand) {
				t.Errorf("the page does not open with %s, so the shell line misstates which view this is", tt.wantCommand)
			}
		})
	}
}

// ?rediger=<id> pre-fills the admin form — for the board only, so a crafted
// link cannot reveal a draft's contents to anyone else.
func TestArrangementerEditPrefill(t *testing.T) {
	repo, visible, _ := eventsPageRepo()
	editURL := "/arrangementer?rediger=" + visible.HexID()

	t.Run("the board gets a pre-filled form", func(t *testing.T) {
		mux := newSite(t, siteConfig{repo: repo})
		rec := get(t, mux, editURL, styret)
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", rec.Code)
		}

		body := rec.Body.String()
		if !contains(body, `value="`+visible.Name+`"`) {
			t.Error("the name field is not pre-filled; editing means retyping the whole event")
		}
		if !contains(body, `name="id" value="`+visible.HexID()+`"`) {
			t.Error("the hidden id field is empty, so saving the edit would create a duplicate event instead of updating")
		}
	})

	t.Run("anyone else gets the plain listing", func(t *testing.T) {
		mux := newSite(t, siteConfig{repo: repo})
		rec := get(t, mux, editURL, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200 — the parameter is ignored, not an error", rec.Code)
		}
		if contains(rec.Body.String(), `<details class="admin"`) {
			t.Error("the admin form is rendered for an anonymous visitor")
		}
	})

	t.Run("a malformed id is ignored", func(t *testing.T) {
		mux := newSite(t, siteConfig{repo: repo})
		rec := get(t, mux, "/arrangementer?rediger=zzz", styret)
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200 — a bad edit link should degrade to the listing", rec.Code)
		}
		if contains(rec.Body.String(), `value="`+visible.Name+`"`) {
			t.Error("a malformed rediger value still pre-filled the form")
		}
	})
}

// The subscribe box's three links are the same feed in the three shapes the
// calendar clients expect: plain https:// for copying, webcal:// so Apple and
// Outlook subscribe rather than download, and Google's add-by-URL page with
// the webcal address percent-encoded into cid. The exact strings are pinned
// because each is a contract with software outside this repository — a feed
// address that drifts breaks every subscriber, and a mangled cid encoding
// sends Google Calendar a URL it cannot parse. The test harness serves the
// site as https://itemize.no, which is what the derivations start from.
func TestArrangementerSubscribeLinks(t *testing.T) {
	repo, _, _ := eventsPageRepo()
	mux := newSite(t, siteConfig{repo: repo})

	rec := get(t, mux, "/arrangementer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"https://itemize.no/api/events/ical",
		"webcal://itemize.no/api/events/ical",
		"https://calendar.google.com/calendar/r?cid=webcal%3A%2F%2Fitemize.no%2Fapi%2Fevents%2Fical",
		// The download attribute is what makes "Last ned .ics" save a file
		// instead of navigating to the feed.
		`download="itemize.ics"`,
	} {
		if !contains(body, want) {
			t.Errorf("the page is missing %q; a visitor cannot subscribe that way", want)
		}
	}
}

// The subscribe box is derived from configuration alone, so it must survive
// the storage being gone: a visitor who cannot see the listing is exactly the
// one best served by a self-updating copy of the calendar. newSite without a
// repository mirrors a deployment whose database is unreachable at startup.
func TestArrangementerSubscribeRendersWithoutStorage(t *testing.T) {
	mux := newSite(t, siteConfig{})

	rec := get(t, mux, "/arrangementer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	if !contains(body, "Vi får ikke hentet arrangementene akkurat nå") {
		t.Error("the page does not admit the calendar is unavailable")
	}
	if !contains(body, `id="abonner"`) || !contains(body, "webcal://itemize.no/api/events/ical") {
		t.Error("the subscribe box vanished with the database, though it needs nothing from it")
	}
}

// A database failure renders the page saying so, rather than a 500 or — worse
// — an empty calendar that reads as "nothing planned".
func TestArrangementerSaysSoWhenListingFails(t *testing.T) {
	repo, _, _ := eventsPageRepo()
	repo.listErr = errTest
	mux := newSite(t, siteConfig{repo: repo})

	rec := get(t, mux, "/arrangementer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 with an explanation rather than an error page", rec.Code)
	}
	if !contains(rec.Body.String(), "Vi får ikke hentet arrangementene akkurat nå") {
		t.Error("the page does not admit the calendar is unavailable, so it reads as if no events exist")
	}
}
