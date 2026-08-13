package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/events"
	"github.com/ItemizeNTNU/website/internal/ical"
)

// The check-in code is the credential that registers attendance. Serving it to
// anyone who lists the events — which the previous version did — let a passer-by
// collect the codes and check themselves in without turning up. The block must
// therefore be absent for everyone but the board, and absent means the key is
// not in the JSON at all: a client that reads `check_in.code` and finds an
// empty string would look no different from one that got a real code.
func TestListEventsWithholdsCheckInFromNonBoard(t *testing.T) {
	repo := &stubRepo{list: []events.Event{pizzakveld(t)}}
	mux := newAPI(t, apiConfig{repo: repo})

	cases := []struct {
		name string
		user *auth.User
		want string
	}{
		{"anonymous", nil, pizzakveldJSON},
		{"signed in without the board role", member, pizzakveldJSON},
		{"board", styret, pizzakveldStyretJSON},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := getAs(t, mux, "/api/events", tc.user)

			wantStatus(t, rec, http.StatusOK)
			wantJSON(t, rec)
			// The encoder appends a newline; everything before it is the value.
			if got := strings.TrimSuffix(rec.Body.String(), "\n"); got != "["+tc.want+"]" {
				t.Errorf("the listing does not match the published wire shape.\n got: %s\nwant: [%s]", got, tc.want)
			}
			if tc.user.IsStyret() {
				return
			}
			if strings.Contains(rec.Body.String(), testCode) {
				t.Error("the check-in code reached a caller who is not on the board; " +
					"anyone who can list the events could register attendance without attending")
			}
		})
	}
}

// The filter is what actually keeps drafts off the page, so it is asserted on
// the repository call rather than inferred from the response — an event that
// happens not to be hidden would make a broken filter look fine.
func TestListEventsFilterFollowsTheRole(t *testing.T) {
	for _, tc := range []struct {
		name string
		user *auth.User
		want bool
	}{
		{"anonymous callers never see drafts", nil, false},
		{"an ordinary member never sees drafts", member, false},
		{"the board sees drafts", styret, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubRepo{}
			mux := newAPI(t, apiConfig{repo: repo})

			getAs(t, mux, "/api/events", tc.user)

			if repo.gotFilter.IncludeHidden != tc.want {
				t.Errorf("asked storage for IncludeHidden=%v, want %v",
					repo.gotFilter.IncludeHidden, tc.want)
			}
		})
	}
}

// ?old, ?old=true and ?old=1 all meant the same thing in the previous API, and
// subscribers still send whichever they picked then. The exact set matters in
// both directions: a value that stops meaning "yes" silently truncates
// somebody's archive view, and a value that starts meaning "yes" makes the
// default listing unbounded.
func TestListEventsOldParameter(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  bool
	}{
		{"", false},
		{"?old", true},
		{"?old=", true},
		{"?old=1", true},
		{"?old=true", true},
		{"?old=yes", true},
		{"?old=false", false},
		{"?old=0", false},
		{"?old=no", false},
		// Case-sensitive, deliberately: the previous API compared strings too.
		{"?old=TRUE", false},
		{"?old=Yes", false},
		{"?new=true", false},
	} {
		t.Run("GET /api/events"+tc.query, func(t *testing.T) {
			repo := &stubRepo{}
			mux := newAPI(t, apiConfig{repo: repo})

			getAs(t, mux, "/api/events"+tc.query, nil)

			if repo.gotFilter.IncludeOld != tc.want {
				t.Errorf("%q gave IncludeOld=%v, want %v",
					tc.query, repo.gotFilter.IncludeOld, tc.want)
			}
		})
	}
}

// truthy is the whole of that argument handling, so it gets a table of its own
// — reaching every case through a request would need a server per row.
func TestTruthy(t *testing.T) {
	for _, tc := range []struct {
		value   string
		present bool
		want    bool
	}{
		{"", false, false},
		{"true", false, false}, // absent beats any value
		{"", true, true},
		{"1", true, true},
		{"true", true, true},
		{"yes", true, true},
		{"0", true, false},
		{"false", true, false},
		{"no", true, false},
		{" true", true, false},
		{"ja", true, false},
	} {
		if got := truthy(tc.value, tc.present); got != tc.want {
			t.Errorf("truthy(%q, %v) = %v, want %v", tc.value, tc.present, got, tc.want)
		}
	}
}

// The query parameter is one-based and the repository's page is zero-based.
// Getting that conversion wrong shifts every page by one, which reads as
// "events are missing" rather than as a paging bug.
//
// Anything that is not a page number a caller could mean — absent, empty,
// "abc", "1.5", zero, negative — is the first page rather than an error, which
// is what the previous API did. The handler clamps rather than leaning on the
// repository doing it, so the page it asks for is never negative: a negative
// page becomes a negative skip, and a negative skip is an error from the driver
// rather than a listing.
func TestListEventsPaging(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"", 0},
		{"?page=1", 0},
		{"?page=2", 1},
		{"?page=3", 2},
		{"?page=0", 0},
		{"?page=", 0},
		{"?page=abc", 0},
		{"?page=-5", 0},
		{"?page=1.5", 0},
		{"?page=1000000", maxPage - 1},
		{"?page=1000001", maxPage - 1}, // clamped, and far past the end
	} {
		t.Run("GET /api/events"+tc.query, func(t *testing.T) {
			repo := &stubRepo{}
			mux := newAPI(t, apiConfig{repo: repo})

			getAs(t, mux, "/api/events"+tc.query, nil)

			if repo.gotFilter.Page != tc.want {
				t.Errorf("%q asked for page %d, want %d",
					tc.query, repo.gotFilter.Page, tc.want)
			}
		})
	}
}

// A page number too large for an int is worth its own case because strconv.Atoi
// does not simply fail on one: it returns the clamped maximum *and* an error.
// With the error discarded the filter carried roughly 9.2e18, the repository
// multiplied that by the page size, the product wrapped negative, and the
// driver refused the query — so an anonymous caller could turn a query string
// they made up into a 500 on the public listing.
//
// Both ends are checked: the page asked for must stay inside the clamp, so the
// multiplication that follows it cannot overflow whatever the caller sent.
func TestListEventsSurvivesAnAbsurdPageNumber(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
		want  int
	}{
		{"larger than an int", "?page=99999999999999999999", maxPage - 1},
		{"more digits than a machine word", "?page=" + strings.Repeat("9", 400), maxPage - 1},
		// Atoi clamps a huge negative to the minimum int, which used to reach
		// the filter as an even more negative page.
		{"smaller than an int", "?page=-99999999999999999999", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubRepo{}
			mux := newAPI(t, apiConfig{repo: repo})

			rec := getAs(t, mux, "/api/events"+tc.query, nil)

			wantStatus(t, rec, http.StatusOK)
			wantJSON(t, rec)
			if repo.gotFilter.Page != tc.want {
				t.Errorf("%q asked storage for page %d, want %d; a skip that has "+
					"overflowed is an error from the driver — a 500 on the public "+
					"listing for anyone who sends this", tc.query, repo.gotFilter.Page, tc.want)
			}
			if got := repo.gotFilter.Page * events.PageSize; got < 0 {
				t.Errorf("page %d times the page size is %d; the skip has wrapped",
					repo.gotFilter.Page, got)
			}
		})
	}
}

// An empty calendar must encode as [] and never as null. A client that does
// `for (const e of await res.json())` throws on null, so the difference between
// the two is a blank page with a console error rather than a blank page.
func TestListEventsEncodesEmptyAsArray(t *testing.T) {
	for _, tc := range []struct {
		name string
		list []events.Event
	}{
		{"nil slice from storage", nil},
		{"empty slice from storage", []events.Event{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := newAPI(t, apiConfig{repo: &stubRepo{list: tc.list}})

			rec := getAs(t, mux, "/api/events", nil)

			wantStatus(t, rec, http.StatusOK)
			if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
				t.Errorf("an empty calendar encoded as %s; a client iterating the "+
					"response would throw rather than render an empty list", got)
			}
		})
	}
}

// A database that will not answer must not look like an empty calendar, or the
// page renders "no upcoming events" during an outage and nobody investigates.
func TestListEventsStorageFailure(t *testing.T) {
	mux := newAPI(t, apiConfig{repo: &stubRepo{listErr: errRepo}})

	rec := getAs(t, mux, "/api/events", nil)

	wantStatus(t, rec, http.StatusInternalServerError)
	wantJSON(t, rec)
	if got := messageOf(t, rec); got != "Something broke :/" {
		t.Errorf("the failure message is %q, want %q", got, "Something broke :/")
	}
}

// The optional fields are optional in the encoding too. An event with no edit
// stamp, no Discord link and no identifier must not emit empty strings for
// them: a client showing "edited" whenever the key is present would claim every
// event had been edited.
func TestEventDTOOmitsAbsentFields(t *testing.T) {
	// A wholly empty event: the zero ObjectID, no timestamps, nothing set.
	got, err := json.Marshal(toDTO(events.Event{}, false))
	if err != nil {
		t.Fatalf("encoding a zero event failed: %v", err)
	}

	const want = `{"_id":"","name":"","location":{"name":"","url":""},` +
		`"register_url":"","date":"0001-01-01T00:00:00Z","duration":0,` +
		`"end":"0001-01-01T00:00:00Z","ctf":{"name":"","url":""},"info":"",` +
		`"hidden":false,"discord":false,"created":"0001-01-01T00:00:00Z"}`
	if string(got) != want {
		t.Errorf("a zero event encodes as\n %s\nwant\n %s", got, want)
	}
}

// The board's view of an event with no attendees yet must not carry an empty
// attendances array — but it must still carry the code, which is the whole
// reason the page asks for it.
func TestEventDTOCheckInWithoutAttendees(t *testing.T) {
	e := events.Event{CheckIn: events.CheckIn{Code: testCode}}

	got, err := json.Marshal(toDTO(e, true))
	if err != nil {
		t.Fatalf("encoding failed: %v", err)
	}
	if !strings.Contains(string(got), `"check_in":{"code":"`+testCode+`"}`) {
		t.Errorf("an event nobody has checked into encodes its check-in block as %s", got)
	}
}

// Durations are hours as a floating-point number because Mongoose wrote them
// that way, and half-hour events exist. Encoding one as an integer would round
// a 90-minute event to an hour on every client that trusts the field.
func TestEventDTOKeepsFractionalDuration(t *testing.T) {
	for _, tc := range []struct {
		duration float64
		want     string
	}{
		{0, `"duration":0`},
		{1.5, `"duration":1.5`},
		{3, `"duration":3`},
		{0.25, `"duration":0.25`},
	} {
		got, err := json.Marshal(toDTO(events.Event{Duration: tc.duration}, false))
		if err != nil {
			t.Fatalf("encoding failed: %v", err)
		}
		if !strings.Contains(string(got), tc.want) {
			t.Errorf("duration %v encoded without %s: %s", tc.duration, tc.want, got)
		}
	}
}

// Norwegian goes in the event name, the venue and the prose, and it has to come
// back out unchanged. Go's encoder escapes HTML-significant runes but leaves
// the rest alone, so æøå must survive as literal UTF-8 rather than as \u
// escapes — some of the things reading this feed do not unescape.
func TestListEventsPreservesNorwegianText(t *testing.T) {
	e := events.Event{
		Name:     "Årsmøte på Blåkläder",
		Location: events.Place{Name: "Realfagbygget, rom Ø-172"},
		Info:     "Vi møtes klokka seks — ta med deg både øl og godt humør. 🎉",
	}
	mux := newAPI(t, apiConfig{repo: &stubRepo{list: []events.Event{e}}})

	rec := getAs(t, mux, "/api/events", nil)

	wantStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	for _, want := range []string{"Årsmøte på Blåkläder", "rom Ø-172", "øl og godt humør. 🎉"} {
		if !strings.Contains(body, want) {
			t.Errorf("%q did not survive the round trip; body was %s", want, body)
		}
	}

	// And it must decode back to exactly what went in, not merely look right.
	var out []eventDTO
	decodeBody(t, rec, &out)
	if len(out) != 1 || out[0].Name != e.Name || out[0].Info != e.Info {
		t.Errorf("the decoded event is %+v, want the fixture back verbatim", out)
	}
}

// The calendar feed is subscribed in people's calendar applications, so the
// headers are as much of the contract as the body: served as anything but
// text/calendar, a subscriber's client downloads a file instead of adding a
// subscription.
func TestICalFeedHeaders(t *testing.T) {
	mux := newAPI(t, apiConfig{repo: &stubRepo{public: []events.Event{pizzakveld(t)}}})

	rec := getAs(t, mux, "/api/events/ical", nil)

	wantStatus(t, rec, http.StatusOK)
	for header, want := range map[string]string{
		"Content-Type":        ical.ContentType,
		"Content-Disposition": `inline; filename="itemize.ics"`,
		"Cache-Control":       "public, max-age=3600",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s is %q, want %q", header, got, want)
		}
	}
}

// The structural lines and the event's own details. A feed missing BEGIN or
// END is rejected outright by most clients, and one missing the UID silently
// duplicates the event on every poll.
func TestICalFeedBody(t *testing.T) {
	mux := newAPI(t, apiConfig{
		repo:    &stubRepo{public: []events.Event{pizzakveld(t)}},
		baseURL: "https://itemize.no",
	})

	rec := getAs(t, mux, "/api/events/ical", nil)
	body := rec.Body.String()

	for _, want := range []string{
		"BEGIN:VCALENDAR",
		"X-WR-CALNAME:Itemize Arrangementer",
		"BEGIN:VEVENT",
		"UID:" + ical.UID(testHexID),
		"DTSTART:20980901T171500Z",
		"DTEND:20980901T201500Z",
		// The edit stamp wins over the creation stamp, so a subscriber's client
		// notices that the event changed.
		"DTSTAMP:20980302T103000Z",
		"SUMMARY:Pizza og CTF",
		"LOCATION:Savannen",
		"URL:https://itemize.no/arrangementer",
		"END:VEVENT",
		"END:VCALENDAR",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the feed is missing %q; body was:\n%s", want, body)
		}
	}

	// RFC 5545 lines end CRLF. A feed with bare newlines is one some clients
	// accept and others reject, which is the worst kind of bug to be told about.
	if strings.Contains(strings.ReplaceAll(body, "\r\n", ""), "\n") {
		t.Error("the feed contains a bare LF; RFC 5545 requires CRLF line endings")
	}
}

// An event that has never been edited still needs a DTSTAMP, so the creation
// time stands in. Without one the entry is invalid and clients drop it.
func TestICalFeedFallsBackToCreatedStamp(t *testing.T) {
	e := pizzakveld(t)
	e.Edited = nil
	mux := newAPI(t, apiConfig{repo: &stubRepo{public: []events.Event{e}}})

	rec := getAs(t, mux, "/api/events/ical", nil)

	if !strings.Contains(rec.Body.String(), "DTSTAMP:20980301T090000Z") {
		t.Errorf("an unedited event did not fall back to its creation stamp:\n%s", rec.Body.String())
	}
}

// Each event carries its own stamp. Sharing one pointer across the loop — which
// is what taking the address of a loop variable used to do — would give every
// event the last one's timestamp, and clients would stop noticing edits.
func TestICalFeedStampsEachEventSeparately(t *testing.T) {
	first := pizzakveld(t)
	first.Edited = nil
	first.Created = time.Date(2098, 1, 1, 0, 0, 0, 0, time.UTC)

	second := pizzakveld(t)
	second.Edited = nil
	second.Created = time.Date(2098, 2, 2, 0, 0, 0, 0, time.UTC)

	mux := newAPI(t, apiConfig{repo: &stubRepo{public: []events.Event{first, second}}})

	body := getAs(t, mux, "/api/events/ical", nil).Body.String()
	for _, want := range []string{"DTSTAMP:20980101T000000Z", "DTSTAMP:20980202T000000Z"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q; the two events share a stamp:\n%s", want, body)
		}
	}
}

// Events stored before `end` existed have a zero End, and older ones let the
// duration default too. A VEVENT without DTEND is open-ended — clients render
// it swallowing the rest of the day, or refuse it — so the feed derives the
// end with ComputeEnd instead of copying the stored field. With no duration
// that is midnight in Oslo after the start: the fixture starts 17:15 UTC on
// 1 September, which is 19:15 CEST, so the following Oslo midnight is 22:00
// UTC the same calendar day.
func TestICalFeedDerivesEndForLegacyEvents(t *testing.T) {
	e := pizzakveld(t)
	e.Duration = 0
	e.End = time.Time{}
	mux := newAPI(t, apiConfig{repo: &stubRepo{public: []events.Event{e}}})

	body := getAs(t, mux, "/api/events/ical", nil).Body.String()

	if !strings.Contains(body, "DTEND:20980901T220000Z") {
		t.Errorf("a legacy event without a stored end did not get the derived DTEND:\n%s", body)
	}
}

// An empty calendar is still a calendar. Returning nothing at all would make
// every subscribed client report a broken feed the first time the board had no
// events queued.
func TestICalFeedWithNoEvents(t *testing.T) {
	mux := newAPI(t, apiConfig{repo: &stubRepo{}})

	rec := getAs(t, mux, "/api/events/ical", nil)
	body := rec.Body.String()

	wantStatus(t, rec, http.StatusOK)
	if !strings.HasPrefix(body, "BEGIN:VCALENDAR") || !strings.Contains(body, "END:VCALENDAR") {
		t.Errorf("an empty feed is not a well-formed calendar:\n%s", body)
	}
	if strings.Contains(body, "BEGIN:VEVENT") {
		t.Error("an empty calendar contains an event")
	}
}

// The feed's failure path is the one place in this package that answers with
// plain text rather than JSON, because its callers are calendar clients rather
// than the site's own scripts. Handing them a JSON envelope would be a body
// they cannot read at all.
func TestICalFeedStorageFailure(t *testing.T) {
	mux := newAPI(t, apiConfig{repo: &stubRepo{publicErr: errRepo}})

	rec := getAs(t, mux, "/api/events/ical", nil)

	wantStatus(t, rec, http.StatusInternalServerError)
	if got := strings.TrimSpace(rec.Body.String()); got != "Kunne ikke bygge kalenderen" {
		t.Errorf("the failure body is %q, want the Norwegian message", got)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type is %q, want text/plain", ct)
	}
	// A failed build must not be cached for an hour, or every subscriber keeps
	// the outage until their client next ignores its own cache.
	if got := rec.Header().Get("Cache-Control"); strings.Contains(got, "max-age=3600") {
		t.Errorf("a failed feed was served with %q; subscribers would cache the outage", got)
	}
}

// A long Norwegian title is folded across lines, and the fold counts octets.
// Splitting mid-rune produces a feed that renders as mojibake in the clients
// that accept it at all — and Norwegian titles are why that is a live concern
// rather than a theoretical one.
func TestICalFeedFoldsWithoutBreakingRunes(t *testing.T) {
	e := pizzakveld(t)
	e.Name = strings.Repeat("Årsmøte på Øya æ ", 6)
	mux := newAPI(t, apiConfig{repo: &stubRepo{public: []events.Event{e}}})

	body := getAs(t, mux, "/api/events/ical", nil).Body.String()

	if !utf8.ValidString(body) {
		t.Fatal("the folded feed is not valid UTF-8; a multi-byte character was cut in half")
	}
	// Unfolding is removing every CRLF-plus-space, which is what a parser does.
	unfolded := strings.ReplaceAll(body, "\r\n ", "")
	if !strings.Contains(unfolded, "SUMMARY:"+e.Name) {
		t.Errorf("the title did not survive folding and unfolding:\n%s", unfolded)
	}
}
