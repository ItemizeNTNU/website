package web_test

// Shared fixtures and fakes for the handler-flow tests. Helpers only — the
// tests themselves live in the *_flow_test.go files.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ItemizeNTNU/website/assets"
	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/discord"
	"github.com/ItemizeNTNU/website/internal/events"
	"github.com/ItemizeNTNU/website/internal/fusionauth"
	"github.com/ItemizeNTNU/website/internal/httpx"
	"github.com/ItemizeNTNU/website/internal/users"
	"github.com/ItemizeNTNU/website/internal/web"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// errTest is a generic infrastructure failure — anything that is not one of
// the sentinel errors the handlers branch on.
var errTest = errors.New("the database is on fire")

// siteConfig is what a test can vary about the server under test. Zero values
// mean "like production, but unconfigured": no database, no FusionAuth token,
// no Discord integration.
type siteConfig struct {
	repo       events.Repository
	svc        *events.Service
	fusion     *fusionauth.Client
	discordSvc *users.DiscordService
	baseURL    string
}

// newSite mirrors the wiring in newMux (routes_test.go) but with injectable
// dependencies, so a flow test can seed a repository or point FusionAuth at a
// fake server.
//
// Every call builds a fresh Server, and with it a fresh signupLimit rate
// limiter. The 429 test in register_flow_test.go relies on that: it burns the
// whole allowance on its own mux. Every other test must stay at five or fewer
// POSTs to /registrer per mux, or it starts seeing 429s that have nothing to
// do with what it is testing.
func newSite(t *testing.T, cfg siteConfig) *http.ServeMux {
	t.Helper()

	fusion := cfg.fusion
	if fusion == nil {
		// Unconfigured: calls fail with ErrNotConfigured, like a deployment
		// without an API key.
		fusion = fusionauth.New("https://auth.example", "")
	}
	baseURL := cfg.baseURL
	if baseURL == "" {
		baseURL = "https://itemize.no"
	}

	// Embedded rather than on-disk: the test binary's working directory is the
	// package, not the repository root.
	fsys := assets.FS(false)
	assetServer, err := httpx.NewAssets(fsys, false)
	if err != nil {
		t.Fatalf("building assets: %v", err)
	}
	site, err := web.NewServer(fsys, assetServer, cfg.repo, cfg.svc, fusion,
		cfg.discordSvc, baseURL, discardLogger(), false)
	if err != nil {
		t.Fatalf("building server: %v", err)
	}

	mux := http.NewServeMux()
	assetServer.Register(mux)
	site.Routes(mux)
	return mux
}

// The user fixtures. The IDs are canonical UUIDs because fusionauth.ValidID
// refuses anything else before a request is even made — a placeholder like
// "user-1" would turn every FusionAuth-backed test into an ErrInvalidID test.
var styret = &auth.User{
	ID:    "11111111-2222-4333-8444-999999999999",
	Name:  "Styremedlem",
	Roles: []string{auth.RoleStyret},
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

// get serves a GET through the mux as u.
func get(t *testing.T, mux *http.ServeMux, path string, u *auth.User) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, asUser(httptest.NewRequest(http.MethodGet, path, nil), u))
	return rec
}

// postForm serves a form POST through the mux as u, carrying a matching CSRF
// cookie and form field so the request clears auth.CSRF.
//
// All POSTs must go through the mux rather than at a handler directly:
// auth.CSRF is what calls ParseForm, and a handler reached without it sees an
// empty form.
func postForm(t *testing.T, mux *http.ServeMux, path string, form url.Values, u *auth.User) *httptest.ResponseRecorder {
	t.Helper()

	if form == nil {
		form = url.Values{}
	}
	form.Set(auth.CSRFField, "test-token")

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// The cookie name is hardcoded rather than exported: it is a
	// browser-visible contract (csrf.go:12), and a rename should fail loudly
	// here rather than being silently followed.
	req.AddCookie(&http.Cookie{Name: "itemize_csrf", Value: "test-token"})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, asUser(req, u))
	return rec
}

// flashOf returns the kind and text of the flash message queued on the
// response, failing the test when there is none — the visitor would land on
// the next page with no explanation of what just happened.
func flashOf(t *testing.T, rec *httptest.ResponseRecorder) (kind, text string) {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name != "flash" {
			continue
		}
		raw, err := url.QueryUnescape(c.Value)
		if err != nil {
			t.Fatalf("the flash cookie is not URL-encoded: %v", err)
		}
		kind, text, ok := strings.Cut(raw, ":")
		if !ok {
			t.Fatalf("flash cookie %q has no kind:text separator; takeFlash would drop it", raw)
		}
		return kind, text
	}
	t.Fatal("no flash cookie was set, so the visitor would get no feedback after the redirect")
	return "", ""
}

// memRepo is an in-memory events.Repository. The embedded interface supplies
// panics for the methods no handler under test should ever reach.
type memRepo struct {
	events.Repository
	stored map[bson.ObjectID]*events.Event
	byCode map[string]*events.Event

	// gotFilter records what List was asked for, which is how the hidden-leak
	// regression tests see the filter rather than inferring it from markup.
	gotFilter   events.Filter
	attendances []events.Attendance

	listErr   error
	upsertErr error
	deleteErr error
	addErr    error
}

func newMemRepo() *memRepo {
	return &memRepo{
		stored: map[bson.ObjectID]*events.Event{},
		byCode: map[string]*events.Event{},
	}
}

// seed stores the event and, when it has a check-in code, indexes it by code.
func (m *memRepo) seed(e *events.Event) {
	m.stored[e.ID] = e
	if e.CheckIn.Code != "" {
		m.byCode[e.CheckIn.Code] = e
	}
}

func (m *memRepo) List(_ context.Context, f events.Filter) ([]events.Event, error) {
	m.gotFilter = f
	if m.listErr != nil {
		return nil, m.listErr
	}
	var out []events.Event
	for _, e := range m.stored {
		if e.Hidden && !f.IncludeHidden {
			continue
		}
		out = append(out, *e)
	}
	return out, nil
}

func (m *memRepo) ByID(_ context.Context, id bson.ObjectID) (*events.Event, error) {
	e, ok := m.stored[id]
	if !ok {
		return nil, events.ErrNotFound
	}
	copied := *e
	return &copied, nil
}

func (m *memRepo) ByCheckInCode(_ context.Context, code string) (*events.Event, error) {
	e, ok := m.byCode[code]
	if !ok {
		return nil, events.ErrNotFound
	}
	copied := *e
	return &copied, nil
}

func (m *memRepo) Upsert(_ context.Context, e *events.Event) (bson.ObjectID, error) {
	if m.upsertErr != nil {
		return bson.ObjectID{}, m.upsertErr
	}
	id := e.ID
	if id.IsZero() {
		id = bson.NewObjectID()
		e.ID = id
	}
	copied := *e
	m.stored[id] = &copied
	return id, nil
}

func (m *memRepo) Delete(_ context.Context, id bson.ObjectID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, ok := m.stored[id]; !ok {
		return events.ErrNotFound
	}
	delete(m.stored, id)
	return nil
}

func (m *memRepo) AddAttendance(_ context.Context, _ string, a events.Attendance) error {
	if m.addErr != nil {
		return m.addErr
	}
	m.attendances = append(m.attendances, a)
	return nil
}

// fakeSyncer stands in for the Discord client on the event write path.
type fakeSyncer struct {
	enabled  bool
	err      error
	upserted []discord.ScheduledEvent
	deleted  []string
}

func (f *fakeSyncer) Enabled() bool { return f.enabled }

func (f *fakeSyncer) UpsertScheduledEvent(_ context.Context, existingID string, e discord.ScheduledEvent) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.upserted = append(f.upserted, e)
	return existingID, nil
}

func (f *fakeSyncer) DeleteScheduledEvent(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, id)
	return nil
}

// newSvc builds the real event service over the given fakes.
func newSvc(repo events.Repository, sync events.Syncer) *events.Service {
	return events.NewService(repo, sync, discardLogger())
}

// fakeFusion points a configured FusionAuth client at handler.
func fakeFusion(t *testing.T, handler http.HandlerFunc) *fusionauth.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return fusionauth.New(srv.URL, "test-api-key")
}

// validEventForm mirrors validForm in internal/events/service_test.go. The
// date is far-future so the event never counts as past while the tests exist.
func validEventForm() url.Values {
	return url.Values{
		"name":          {"Pizza og CTF"},
		"location.name": {"Savannen"},
		"location.url":  {""},
		"register_url":  {""},
		"date":          {"2098-09-01T17:15"},
		"duration":      {"3"},
		"ctf.name":      {""},
		"ctf.url":       {""},
		"info":          {"Ta med laptop."},
	}
}
