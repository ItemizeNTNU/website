package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ItemizeNTNU/website/internal/events"
)

// Registering the API twice on one mux, or alongside a pattern it conflicts
// with, panics at registration — which is a crash at container start, in
// production, on a Friday. Building the table here turns that into a test
// failure instead.
func TestRoutesRegisterWithoutConflict(t *testing.T) {
	newAPI(t, apiConfig{repo: &stubRepo{}})
}

// Everything under /api answers as JSON rather than falling through to the
// site's HTML error page. A script that fetches a mistyped path and gets a page
// of markup fails with a parse error that says nothing about what went wrong.
func TestUnknownAPIPathAnswersJSON(t *testing.T) {
	mux := newAPI(t, apiConfig{repo: &stubRepo{}})

	for _, path := range []string{
		"/api/",
		"/api/nope",
		"/api/events/nope",
		"/api/checkin",
		"/api/user",
		"/api/arrangementer",
	} {
		t.Run(path, func(t *testing.T) {
			rec := getAs(t, mux, path, styret)

			wantStatus(t, rec, http.StatusNotFound)
			wantJSON(t, rec)
			if got := messageOf(t, rec); got != "API endpoint not found" {
				t.Errorf("the message is %q, want %q", got, "API endpoint not found")
			}
			if strings.Contains(rec.Body.String(), "<!DOCTYPE") {
				t.Error("the site's HTML error page leaked into an API response")
			}
		})
	}
}

// A request to a real path with the wrong method lands on the same catch-all
// rather than producing a 405. That is worth pinning because it is not what
// ServeMux does on its own: without the catch-all Go answers 405 with an Allow
// header and an empty body, and a client parsing every response as JSON would
// fail on it. Here it gets a JSON 404 instead.
func TestWrongMethodAnswersJSON(t *testing.T) {
	repo := &stubRepo{byCode: map[string]events.Event{testCode: pizzakveld(t)}}
	mux := newAPI(t, apiConfig{repo: repo})

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/events"},
		{http.MethodDelete, "/api/events"},
		{http.MethodPut, "/api/events/ical"},
		{http.MethodDelete, "/api/checkin/" + testCode},
		{http.MethodPatch, "/api/checkin/" + testCode},
		{http.MethodPost, "/api/user"},
		{http.MethodGet, "/api/user"},
		{http.MethodDelete, "/api/user/" + member.ID},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := do(t, mux, tc.method, tc.path, nil, styret)

			wantStatus(t, rec, http.StatusNotFound)
			wantJSON(t, rec)
			if got := messageOf(t, rec); got != "API endpoint not found" {
				t.Errorf("the message is %q, want %q", got, "API endpoint not found")
			}
			if repo.adds != 0 {
				t.Error("a request with the wrong method still wrote to the register")
			}
		})
	}
}

// HEAD is what a monitor or a link checker sends. ServeMux routes it to the GET
// handler, so the feed and the listing must answer rather than 404 — a probe
// reporting the calendar as missing would be a false alarm nobody could
// reproduce in a browser.
func TestHeadIsServedByTheGetHandlers(t *testing.T) {
	repo := &stubRepo{
		list:   []events.Event{pizzakveld(t)},
		public: []events.Event{pizzakveld(t)},
	}
	mux := newAPI(t, apiConfig{repo: repo})

	for _, path := range []string{"/api/events", "/api/events/ical"} {
		t.Run(path, func(t *testing.T) {
			rec := do(t, mux, http.MethodHead, path, nil, nil)

			wantStatus(t, rec, http.StatusOK)
		})
	}
}

// The listing and the feed are the two endpoints an unauthenticated visitor is
// meant to reach — the events page and the calendar subscription both depend on
// it. A gate added to either would break them without anything failing here
// unless this says so.
func TestPublicEndpointsStayPublic(t *testing.T) {
	repo := &stubRepo{public: []events.Event{pizzakveld(t)}}
	mux := newAPI(t, apiConfig{repo: repo})

	for _, path := range []string{"/api/events", "/api/events/ical"} {
		t.Run(path, func(t *testing.T) {
			rec := getAs(t, mux, path, nil)

			wantStatus(t, rec, http.StatusOK)
		})
	}
}

// The check-in code is a path segment, so a trailing slash or an extra segment
// must not be read as a code of its own — it lands on the catch-all instead.
func TestCheckInCodeIsASingleSegment(t *testing.T) {
	repo := &stubRepo{byCode: map[string]events.Event{testCode: pizzakveld(t)}}
	mux := newAPI(t, apiConfig{repo: repo})

	for _, path := range []string{
		"/api/checkin/",
		"/api/checkin/" + testCode + "/attendances",
	} {
		t.Run(path, func(t *testing.T) {
			rec := getAs(t, mux, path, styret)

			wantStatus(t, rec, http.StatusNotFound)
			if got := messageOf(t, rec); got != "API endpoint not found" {
				t.Errorf("the message is %q, want the catch-all's", got)
			}
			if repo.gotCode != "" {
				t.Errorf("storage was queried with %q for a path that does not name a code",
					repo.gotCode)
			}
		})
	}
}
