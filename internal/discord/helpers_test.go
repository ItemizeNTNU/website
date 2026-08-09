package discord

// Shared fixtures and fakes for the Discord client tests. Helpers only — the
// tests themselves live in the other *_test.go files in this package.

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ItemizeNTNU/website/internal/config"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testConfig() config.Discord {
	return config.Discord{
		ClientID:     "client-1",
		ClientSecret: "secret-1",
		BotToken:     "bot-1",
		GuildID:      "guild-1",
		MemberRoleID: "role-1",
	}
}

// testSnowflake is a realistic Discord identifier. The client validates the
// shape before building a path, so a placeholder like "user-1" would be
// refused before any request went out and the test would prove nothing.
const testSnowflake = "80351110224678912"

// discordRewriter points the client at a test server without changing the
// production code.
//
// APIBase is a constant, so unlike internal/fusionauth there is no base-URL
// argument to pass a httptest address to. The seam that does exist is the
// client's *http.Client: a RoundTripper that swaps only the scheme and host
// leaves the method, path, headers and body exactly as the production code
// built them, so everything asserted below is the real request.
//
// It also fails the test outright if anything is aimed at a host other than
// discord.com. That is what makes "these tests never touch the network" a
// checked property rather than a hope: a future call built against a different
// host would be caught here instead of silently reaching the internet.
type discordRewriter struct {
	t      *testing.T
	target *url.URL
}

func (rw *discordRewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "discord.com" {
		rw.t.Errorf("a request was aimed at %s; every Discord call must go to "+
			"discord.com, and this one would have escaped the test server", req.URL)
	}
	out := req.Clone(req.Context())
	out.URL.Scheme = rw.target.Scheme
	out.URL.Host = rw.target.Host
	out.Host = ""
	return http.DefaultTransport.RoundTrip(out)
}

// recordedRequest is what the fake Discord saw. Body is captured eagerly so a
// test can assert on it after the handler has returned.
type recordedRequest struct {
	Method   string
	Path     string
	RawQuery string
	Header   http.Header
	Body     []byte
}

// fakeDiscord is a stand-in for the Discord API that remembers every request.
type fakeDiscord struct {
	t   *testing.T
	srv *httptest.Server
	mu  sync.Mutex
	got []recordedRequest
}

// stop takes the fake off the air, so a test can see what the client does when
// Discord is unreachable.
func (f *fakeDiscord) stop() { f.srv.Close() }

func (f *fakeDiscord) record(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, recordedRequest{
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Header:   r.Header.Clone(),
		Body:     body,
	})
}

// count is how many requests reached the fake. Several tests care only that a
// malformed identifier produced none at all.
func (f *fakeDiscord) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.got)
}

func (f *fakeDiscord) requests() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedRequest(nil), f.got...)
}

// first returns the request the client sent first, failing the test when it
// never sent one — a nil-deref inside an assertion tells nobody anything.
func (f *fakeDiscord) first() recordedRequest {
	f.t.Helper()
	got := f.requests()
	if len(got) == 0 {
		f.t.Fatal("the client never contacted Discord")
	}
	return got[0]
}

// newFakeDiscord builds a configured client wired to handler.
func newFakeDiscord(t *testing.T, handler http.HandlerFunc) (*Client, *fakeDiscord) {
	t.Helper()
	return newFakeDiscordWith(t, testConfig(), handler)
}

// newFakeDiscordWith is newFakeDiscord for tests that need to vary the config,
// such as a guild or role identifier appearing in a path.
func newFakeDiscordWith(t *testing.T, cfg config.Discord, handler http.HandlerFunc) (*Client, *fakeDiscord) {
	t.Helper()

	fake := &fakeDiscord{t: t}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.record(r)
		if handler != nil {
			handler(w, r)
		}
	}))
	fake.srv = srv
	t.Cleanup(srv.Close)

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("the test server handed back an unusable address %q: %v", srv.URL, err)
	}

	c := &Client{
		cfg: cfg,
		// A timeout well short of `go test`'s own, so a wedged expectation
		// fails as a test failure rather than a ten-minute hang.
		http: &http.Client{
			Transport: &discordRewriter{t: t, target: target},
			Timeout:   5 * time.Second,
		},
		log: discardLogger(),
	}
	return c, fake
}

// jsonReply writes a canned response. Discord always answers JSON, including
// for errors, so this is the shape almost every test needs.
func jsonReply(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

// sequence serves each handler once, in order, and repeats the last one
// afterwards. Retry behaviour needs a server whose second answer differs from
// its first, and "repeat the last" keeps a test from depending on exactly how
// many times the client gives up.
func sequence(handlers ...http.HandlerFunc) http.HandlerFunc {
	var n atomic.Int64
	return func(w http.ResponseWriter, r *http.Request) {
		i := int(n.Add(1)) - 1
		if i >= len(handlers) {
			i = len(handlers) - 1
		}
		handlers[i](w, r)
	}
}

// rateLimited is a 429 that asks to be retried almost immediately. Discord
// reports the wait in seconds, so anything larger would put real sleeping into
// the test suite.
func rateLimited(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Retry-After", "0.001")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = io.WriteString(w, `{"message":"You are being rate limited.","code":0}`)
}

// truncatedReply promises more body than it delivers and then drops the
// connection, which is what a proxy timing out mid-response looks like from
// the client side. Hijacking is the only way to produce it reliably: the
// net/http server otherwise pads or corrects a short write for you.
func truncatedReply(w http.ResponseWriter, r *http.Request) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "not hijackable", http.StatusInternalServerError)
		return
	}
	conn, buf, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = buf.WriteString("HTTP/1.1 200 OK\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: 4096\r\n\r\n" +
		`{"id":"803511102`)
	_ = buf.Flush()
}

// repeat builds a long string of r. Used to push a name, a description or a
// response body past a limit.
func repeat(r rune, n int) string { return strings.Repeat(string(r), n) }
