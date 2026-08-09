package api

// Shared fixtures and fakes for the API handler tests. Helpers only — the
// tests themselves live in events_test.go, checkin_test.go, users_test.go,
// respond_test.go and routes_test.go.
//
// These are in-package rather than in an api_test package because the wire
// contract lives as much in the unexported helpers (toDTO, truthy, num,
// yearOf) as in the handlers, and those are worth testing directly instead of
// only through whatever combination of requests happens to reach them.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/events"
	"github.com/ItemizeNTNU/website/internal/fusionauth"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// errRepo is a generic infrastructure failure — anything that is not one of
// the sentinel errors the handlers branch on.
var errRepo = errors.New("the database is on fire")

// stubRepo is an in-memory events.Repository. The embedded interface supplies
// the methods no API handler should ever reach: a call to one of them is a nil
// dereference, which is a louder failure than quietly returning a zero value
// and is exactly what a handler reaching for storage it has no business
// touching deserves.
type stubRepo struct {
	events.Repository

	// list is returned verbatim by List. The handler's job is deriving the
	// filter and mapping the result, not filtering — so the fake deliberately
	// does no filtering of its own, and the filter it was handed is recorded
	// instead.
	list   []events.Event
	public []events.Event
	byCode map[string]events.Event

	listErr   error
	publicErr error
	byCodeErr error
	addErr    error

	gotFilter     events.Filter
	gotCode       string
	gotAttendance events.Attendance
	adds          int
}

func (s *stubRepo) List(_ context.Context, f events.Filter) ([]events.Event, error) {
	s.gotFilter = f
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.list, nil
}

func (s *stubRepo) Public(context.Context) ([]events.Event, error) {
	if s.publicErr != nil {
		return nil, s.publicErr
	}
	return s.public, nil
}

func (s *stubRepo) ByCheckInCode(_ context.Context, code string) (*events.Event, error) {
	s.gotCode = code
	if s.byCodeErr != nil {
		return nil, s.byCodeErr
	}
	e, ok := s.byCode[code]
	if !ok {
		return nil, events.ErrNotFound
	}
	return &e, nil
}

func (s *stubRepo) AddAttendance(_ context.Context, code string, a events.Attendance) error {
	s.gotCode = code
	s.gotAttendance = a
	s.adds++
	return s.addErr
}

// apiConfig is what a test can vary about the API under test. The zero value
// is "like a deployment with no FusionAuth key": every user call answers 503.
type apiConfig struct {
	repo    events.Repository
	fusion  *fusionauth.Client
	baseURL string

	// nilFusion passes a literal nil client, which is what a caller that
	// forgot to wire FusionAuth would produce. Configured() has a nil check
	// for exactly this, and the handlers rely on it.
	nilFusion bool
}

// newAPI builds the real routing table over injectable dependencies.
//
// Every call builds a fresh Server, and with it a fresh signupLimit rate
// limiter holding five tokens. The 429 test in users_test.go burns the whole
// allowance on its own mux; every other test must stay at five or fewer PUTs
// to /api/user per mux, or it starts seeing 429s that have nothing to do with
// what it is testing.
func newAPI(t *testing.T, cfg apiConfig) *http.ServeMux {
	t.Helper()

	fusion := cfg.fusion
	if fusion == nil && !cfg.nilFusion {
		fusion = fusionauth.New("https://auth.example", "")
	}
	baseURL := cfg.baseURL
	if baseURL == "" {
		baseURL = "https://itemize.no"
	}

	mux := http.NewServeMux()
	NewServer(cfg.repo, fusion, baseURL, discardLogger()).Routes(mux)
	return mux
}

// fusionSpy records what reached the fake FusionAuth. Everything is behind a
// mutex because the handler runs on the server's goroutine while the test reads
// it from its own.
type fusionSpy struct {
	mu     sync.Mutex
	calls  int
	path   string
	method string
	auth   string
	body   string
}

func (f *fusionSpy) snapshot() fusionSpy {
	f.mu.Lock()
	defer f.mu.Unlock()
	return fusionSpy{calls: f.calls, path: f.path, method: f.method, auth: f.auth, body: f.body}
}

// fakeFusion points a configured FusionAuth client at handler and records what
// was asked of it — which is how the identifier-validation tests tell "refused
// here" from "refused by FusionAuth".
func fakeFusion(t *testing.T, handler http.HandlerFunc) (*fusionauth.Client, *fusionSpy) {
	t.Helper()
	spy := &fusionSpy{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		spy.mu.Lock()
		spy.calls++
		spy.path = r.URL.Path
		spy.method = r.Method
		spy.auth = r.Header.Get("Authorization")
		spy.body = string(body)
		spy.mu.Unlock()

		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return fusionauth.New(srv.URL, "test-api-key"), spy
}

// deadFusion is a client pointed at a server that is already gone: the call
// fails at the transport rather than with a status code, which is the branch a
// FusionAuth outage or a DNS failure takes.
func deadFusion(t *testing.T) *fusionauth.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	return fusionauth.New(url, "test-api-key")
}

// The user fixtures. The identifiers are canonical UUIDs because
// fusionauth.ValidID refuses anything else before a request is made — a
// placeholder like "user-1" would turn every FusionAuth-backed test into an
// ErrInvalidID test by accident.
var styret = &auth.User{
	ID:       "11111111-2222-4333-8444-999999999999",
	Name:     "Styremedlem",
	FullName: "Åse Øverland",
	Roles:    []string{auth.RoleStyret},
}

var member = &auth.User{
	ID:       "22222222-3333-4444-8555-666666666666",
	Name:     "Kari",
	FullName: "Kari Nordmann",
	Email:    "medlem@example.no",
}

// asUser attaches u to the request context the way the authn middleware would.
// The test mux carries no Inject middleware, so this is the only way a request
// is ever signed in. A nil user is an anonymous visitor.
func asUser(r *http.Request, u *auth.User) *http.Request {
	if u == nil {
		return r
	}
	return r.WithContext(auth.WithUser(r.Context(), u))
}

// do serves a request through the mux as u.
//
// Requests go through the mux rather than at a handler directly because the
// authorization middleware and the path wildcards are part of what is being
// tested: a handler reached without them sees an empty PathValue and an
// unchecked role.
func do(t *testing.T, mux *http.ServeMux, method, path string, body io.Reader, u *auth.User) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, asUser(httptest.NewRequest(method, path, body), u))
	return rec
}

// getAs is do for the read endpoints, which never carry a body.
func getAs(t *testing.T, mux *http.ServeMux, path string, u *auth.User) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, mux, http.MethodGet, path, nil, u)
}

// putJSON is do for the registration endpoint, whose body is JSON.
func putJSON(t *testing.T, mux *http.ServeMux, path, body string, u *auth.User) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, asUser(req, u))
	return rec
}

// wantStatus fails when the response carries a status other than want. The
// body is included because an unexpected status is nearly always explained by
// the message the handler put in it.
func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("got %d, want %d; body was %s", rec.Code, want, strings.TrimSpace(rec.Body.String()))
	}
}

// wantJSON checks the Content-Type. Clients parse the body without sniffing
// it, so a response served as anything else is one they will not read at all
// — which is a different and much more confusing failure than an error status.
func wantJSON(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	const want = "application/json; charset=utf-8"
	if got := rec.Header().Get("Content-Type"); got != want {
		t.Errorf("Content-Type is %q, want %q; a JSON client will not parse this", got, want)
	}
}

// messageOf decodes the {"message": ...} envelope every status and error
// response uses. Clients key off that field, so a response that has lost the
// shape is a break even when the status code is right.
func messageOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body message
	decodeBody(t, rec, &body)
	return body.Message
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("the response is not JSON the client can decode: %v; body was %s",
			err, strings.TrimSpace(rec.Body.String()))
	}
}

// The event fixtures. Every timestamp is fixed and in UTC so the encoded shape
// can be compared literally rather than field by field.
var (
	fixedCreated = time.Date(2098, 3, 1, 9, 0, 0, 0, time.UTC)
	fixedEdited  = time.Date(2098, 3, 2, 10, 30, 0, 0, time.UTC)
	fixedStart   = time.Date(2098, 9, 1, 17, 15, 0, 0, time.UTC)
	fixedEnd     = time.Date(2098, 9, 1, 20, 15, 0, 0, time.UTC)
	fixedCheckIn = time.Date(2098, 9, 1, 17, 22, 0, 0, time.UTC)
)

// testCode is the check-in credential. It is UUID-shaped because the real ones
// are, and the tests that put it in a URL want a realistic path segment.
const testCode = "3fa85f64-5717-4562-b3fc-2c963f66afa6"

const testHexID = "507f1f77bcf86cd799439011"

func mustObjectID(t *testing.T, hex string) bson.ObjectID {
	t.Helper()
	id, err := bson.ObjectIDFromHex(hex)
	if err != nil {
		t.Fatalf("the fixture identifier %q is not a valid ObjectID: %v", hex, err)
	}
	return id
}

// pizzakveld is a fully populated event: every optional field set, so a test
// that pins the encoded shape sees all of them, and Norwegian text in the
// places a board member would actually type it.
func pizzakveld(t *testing.T) events.Event {
	t.Helper()
	edited := fixedEdited
	return events.Event{
		ID:             mustObjectID(t, testHexID),
		Name:           "Pizza og CTF",
		Location:       events.Place{Name: "Savannen", URL: "https://itemize.no/savannen"},
		RegisterURL:    "https://itemize.no/pamelding",
		Date:           fixedStart,
		Duration:       3,
		End:            fixedEnd,
		CTF:            events.Place{Name: "ItemizeCTF", URL: "https://ctf.itemize.no"},
		Info:           "Ta med laptop.",
		Hidden:         false,
		Discord:        true,
		DiscordEventID: "1234567890",
		Created:        fixedCreated,
		Edited:         &edited,
		CheckIn: events.CheckIn{
			Code: testCode,
			Attendances: []events.Attendance{{
				ID:         mustObjectID(t, "507f191e810c19729de860ea"),
				Name:       "Kari Nordmann",
				UserID:     member.ID,
				Registered: fixedCheckIn,
			}},
		},
	}
}

// pizzakveldFields is the encoded form of the fixture, closing brace omitted
// so the check-in block can be appended.
//
// It is written out in full rather than asserted field by field because it is
// a published contract: this JSON and the iCal feed are what things outside
// this repository depend on, and a renamed, reordered or newly-omitempty field
// breaks a consumer nobody here can see. A diff on this string is the warning.
const pizzakveldFields = `{"_id":"507f1f77bcf86cd799439011",` +
	`"name":"Pizza og CTF",` +
	`"location":{"name":"Savannen","url":"https://itemize.no/savannen"},` +
	`"register_url":"https://itemize.no/pamelding",` +
	`"date":"2098-09-01T17:15:00Z",` +
	`"duration":3,` +
	`"end":"2098-09-01T20:15:00Z",` +
	`"ctf":{"name":"ItemizeCTF","url":"https://ctf.itemize.no"},` +
	`"info":"Ta med laptop.",` +
	`"hidden":false,` +
	`"discord":true,` +
	`"discordEventId":"1234567890",` +
	`"created":"2098-03-01T09:00:00Z",` +
	`"edited":"2098-03-02T10:30:00Z"`

// pizzakveldJSON is what a caller who is not on the board gets: no check-in
// block at all.
const pizzakveldJSON = pizzakveldFields + `}`

// pizzakveldStyretJSON is the same event for the board, carrying the check-in
// code and the attendance register.
const pizzakveldStyretJSON = pizzakveldFields +
	`,"check_in":{"code":"` + testCode + `",` +
	`"attendances":[{"name":"Kari Nordmann",` +
	`"user_id":"22222222-3333-4444-8555-666666666666",` +
	`"registered":"2098-09-01T17:22:00Z"}]}}`
