package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/events"
)

// adminSite wires a mux with the real event service over the given repo.
func adminSite(t *testing.T, repo *memRepo, sync *fakeSyncer) *http.ServeMux {
	t.Helper()
	return newSite(t, siteConfig{repo: repo, svc: newSvc(repo, sync)})
}

// seededEvent stores an existing event and returns its id, for the update and
// delete paths.
func seededEvent(repo *memRepo) *events.Event {
	start := time.Date(2098, 9, 1, 17, 15, 0, 0, time.UTC)
	e := &events.Event{
		ID:       bson.NewObjectID(),
		Name:     "Eksisterende arrangement",
		Location: events.Place{Name: "Savannen"},
		Date:     start,
		Duration: 3,
		End:      start.Add(3 * time.Hour),
		Info:     "Allerede lagret.",
		CheckIn:  events.CheckIn{Code: "cccccccc-dddd-4eee-8fff-000000000000"},
	}
	repo.seed(e)
	return e
}

// Every way a save must be refused before the handler ever runs: role gates
// and the CSRF middleware. Each of these failing open is a way for someone
// else to write the board's calendar.
func TestSaveEventGates(t *testing.T) {
	tests := []struct {
		name       string
		request    func() *http.Request
		wantStatus int
		wantBody   string
	}{
		{
			name: "anonymous is sent to log in",
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/arrangementer", nil)
			},
			wantStatus: http.StatusFound,
		},
		{
			name: "a member without the board role is refused",
			request: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/arrangementer", nil)
				return asUser(r, member)
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "no CSRF cookie",
			request: func() *http.Request {
				form := validEventForm()
				form.Set(auth.CSRFField, "test-token")
				r := httptest.NewRequest(http.MethodPost, "/arrangementer",
					strings.NewReader(form.Encode()))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return asUser(r, styret)
			},
			wantStatus: http.StatusForbidden,
			wantBody:   "Skjemaet er utløpt",
		},
		{
			name: "cookie and form token disagree",
			request: func() *http.Request {
				form := validEventForm()
				form.Set(auth.CSRFField, "attacker-guess")
				r := httptest.NewRequest(http.MethodPost, "/arrangementer",
					strings.NewReader(form.Encode()))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				r.AddCookie(&http.Cookie{Name: "itemize_csrf", Value: "test-token"})
				return asUser(r, styret)
			},
			wantStatus: http.StatusForbidden,
			wantBody:   "Skjemaet er utløpt",
		},
		{
			name: "the browser says the submission is cross-site",
			request: func() *http.Request {
				form := validEventForm()
				form.Set(auth.CSRFField, "test-token")
				r := httptest.NewRequest(http.MethodPost, "/arrangementer",
					strings.NewReader(form.Encode()))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				r.Header.Set("Sec-Fetch-Site", "cross-site")
				r.AddCookie(&http.Cookie{Name: "itemize_csrf", Value: "test-token"})
				return asUser(r, styret)
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemRepo()
			mux := adminSite(t, repo, &fakeSyncer{})

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, tt.request())

			if rec.Code != tt.wantStatus {
				t.Fatalf("got %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && !contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("the refusal does not say %q, so the board member gets no hint the fix is a reload", tt.wantBody)
			}
			if len(repo.stored) != 0 {
				t.Error("a refused submission still wrote an event")
			}
		})
	}
}

func TestSaveEventWithoutService(t *testing.T) {
	// Repository present, service nil — the write path is what is down.
	mux := newSite(t, siteConfig{repo: newMemRepo()})

	rec := postForm(t, mux, "/arrangementer", validEventForm(), styret)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 — pretending the save worked would silently drop it", rec.Code)
	}
}

// A malformed id is refused before validation runs, and as a 404 rather than
// a 400: answering differently would confirm the identifier format to anyone
// probing.
func TestSaveEventMalformedIDIs404(t *testing.T) {
	form := validEventForm()
	form.Set("id", "zzz")
	// The rest of the form being invalid too proves the id check comes first.
	form.Set("name", "x")

	mux := adminSite(t, newMemRepo(), &fakeSyncer{})
	rec := postForm(t, mux, "/arrangementer", form, styret)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestSaveEventValidationEchoesTheForm(t *testing.T) {
	form := validEventForm()
	form.Set("name", "ab") // too short
	form.Set("info", "Husk å ta med laptop og godt humør.")

	repo := newMemRepo()
	mux := adminSite(t, repo, &fakeSyncer{})
	rec := postForm(t, mux, "/arrangementer", form, styret)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", rec.Code)
	}
	body := rec.Body.String()
	// What was typed must come back, or the board starts the form over.
	if !contains(body, `value="ab"`) {
		t.Error("the rejected name is not echoed back into the form")
	}
	if !contains(body, "Husk å ta med laptop og godt humør.") {
		t.Error("the typed info text is lost on a validation failure")
	}
	if !contains(body, "Navn må være minst 3 tegn.") {
		t.Error("no Norwegian error explains what was wrong with the name")
	}
	if len(repo.stored) != 0 {
		t.Error("an invalid submission was stored anyway")
	}
}

// A well-formed id for an event that does not exist fails in the service, not
// the parser — and still reads as 404.
func TestSaveEventUnknownIDIs404(t *testing.T) {
	form := validEventForm()
	form.Set("id", bson.NewObjectID().Hex())

	mux := adminSite(t, newMemRepo(), &fakeSyncer{})
	rec := postForm(t, mux, "/arrangementer", form, styret)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

// Discord being down must not lose the board's work: the event is stored and
// the warning names precisely what failed.
func TestSaveEventDiscordFailureStoresAndWarns(t *testing.T) {
	repo := newMemRepo()
	mux := adminSite(t, repo, &fakeSyncer{enabled: true, err: errTest})

	form := validEventForm()
	form.Set("discord", "1")
	rec := postForm(t, mux, "/arrangementer", form, styret)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want 303 — the save succeeded and should redirect", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/arrangementer" {
		t.Errorf("redirected to %q, want /arrangementer", got)
	}
	kind, text := flashOf(t, rec)
	if kind != "warning" {
		t.Errorf("flash kind = %q, want warning — an error would imply the event was lost", kind)
	}
	if !contains(text, "Discord ble ikke oppdatert") {
		t.Errorf("flash %q does not name Discord as the part that failed", text)
	}
	if len(repo.stored) != 1 {
		t.Fatalf("stored %d events, want 1 — the event itself must survive a Discord outage", len(repo.stored))
	}
}

func TestSaveEventStorageFailure(t *testing.T) {
	repo := newMemRepo()
	repo.upsertErr = errTest
	mux := adminSite(t, repo, &fakeSyncer{})

	rec := postForm(t, mux, "/arrangementer", validEventForm(), styret)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if !contains(body, "kunne ikke lagres") {
		t.Error("the page does not say the event could not be saved")
	}
	// The form comes back populated, so the board can retry without retyping.
	if !contains(body, `value="Pizza og CTF"`) {
		t.Error("the submission is lost on a storage failure")
	}
}

func TestSaveEventSuccessFlashes(t *testing.T) {
	tests := []struct {
		name      string
		update    bool
		wantFlash string
	}{
		{"creating", false, "Arrangementet er lagt til."},
		{"updating", true, "Arrangementet er oppdatert."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemRepo()
			mux := adminSite(t, repo, &fakeSyncer{})

			form := validEventForm()
			if tt.update {
				form.Set("id", seededEvent(repo).HexID())
			}
			rec := postForm(t, mux, "/arrangementer", form, styret)

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("got %d, want 303 — rendering directly would resubmit on refresh", rec.Code)
			}
			if got := rec.Header().Get("Location"); got != "/arrangementer" {
				t.Errorf("redirected to %q, want /arrangementer", got)
			}
			kind, text := flashOf(t, rec)
			if kind != "success" || text != tt.wantFlash {
				t.Errorf("flash = %q %q, want success %q", kind, text, tt.wantFlash)
			}
			if len(repo.stored) != 1 {
				t.Errorf("stored %d events, want 1", len(repo.stored))
			}
		})
	}
}

func TestConfirmDelete(t *testing.T) {
	t.Run("shows the event behind a real form", func(t *testing.T) {
		repo := newMemRepo()
		e := seededEvent(repo)
		mux := adminSite(t, repo, &fakeSyncer{})

		rec := get(t, mux, "/arrangementer/"+e.HexID()+"/slett", styret)
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", rec.Code)
		}
		body := rec.Body.String()
		// Naming the event is what stops someone deleting the wrong one from a
		// stale tab.
		if !contains(body, e.Name) {
			t.Error("the confirmation page does not name the event being deleted")
		}
		if !contains(body, `name="_csrf"`) {
			t.Error("the delete form carries no CSRF field, so submitting it would be refused")
		}
	})

	tests := []struct {
		name       string
		svc        bool
		path       string
		user       *auth.User
		wantStatus int
	}{
		{"no service is 503", false, "/slett", styret, http.StatusServiceUnavailable},
		{"a malformed id is 404", true, "/slett", styret, http.StatusNotFound},
		{"anonymous is sent to log in", true, "/slett", nil, http.StatusFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemRepo()
			cfg := siteConfig{repo: repo}
			if tt.svc {
				cfg.svc = newSvc(repo, &fakeSyncer{})
			}
			mux := newSite(t, cfg)

			rec := get(t, mux, "/arrangementer/zzz"+tt.path, tt.user)
			if rec.Code != tt.wantStatus {
				t.Fatalf("got %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}

	t.Run("an unknown id is 404", func(t *testing.T) {
		repo := newMemRepo()
		mux := adminSite(t, repo, &fakeSyncer{})
		rec := get(t, mux, "/arrangementer/"+bson.NewObjectID().Hex()+"/slett", styret)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404", rec.Code)
		}
	})
}

func TestDeleteEvent(t *testing.T) {
	t.Run("deletes and confirms", func(t *testing.T) {
		repo := newMemRepo()
		e := seededEvent(repo)
		mux := adminSite(t, repo, &fakeSyncer{})

		rec := postForm(t, mux, "/arrangementer/"+e.HexID()+"/slett", nil, styret)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("got %d, want 303", rec.Code)
		}
		kind, text := flashOf(t, rec)
		if kind != "success" || text != "Arrangementet er slettet." {
			t.Errorf("flash = %q %q, want the deletion confirmed", kind, text)
		}
		if _, ok := repo.stored[e.ID]; ok {
			t.Error("the event is still stored after a confirmed delete")
		}
	})

	t.Run("an unknown id is 404", func(t *testing.T) {
		repo := newMemRepo()
		mux := adminSite(t, repo, &fakeSyncer{})
		rec := postForm(t, mux, "/arrangementer/"+bson.NewObjectID().Hex()+"/slett", nil, styret)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404", rec.Code)
		}
	})

	// A failed delete still redirects — the confirmation page has already been
	// left, so the outcome travels as a flash rather than an error page.
	t.Run("a storage failure redirects with an error flash", func(t *testing.T) {
		repo := newMemRepo()
		e := seededEvent(repo)
		repo.deleteErr = errTest
		mux := adminSite(t, repo, &fakeSyncer{})

		rec := postForm(t, mux, "/arrangementer/"+e.HexID()+"/slett", nil, styret)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("got %d, want 303 even on failure", rec.Code)
		}
		kind, text := flashOf(t, rec)
		if kind != "error" || !contains(text, "kunne ikke slettes") {
			t.Errorf("flash = %q %q, want an error saying the delete failed — a success message here would hide a still-existing event", kind, text)
		}
	})
}
