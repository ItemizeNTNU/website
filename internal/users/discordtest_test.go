package users

// Fakes for the Discord linking tests. Helpers only — the tests themselves
// live in discord_test.go.
//
// Both sides of the flow are served by local httptest servers. FusionAuth
// takes its host from the constructor, so pointing it at a fake is ordinary
// wiring. Discord does not: discord.APIBase is a constant, and the client
// builds its own http.Client with a nil Transport. That nil is the seam —
// net/http falls back to http.DefaultTransport — so swapping DefaultTransport
// for one that rewrites discord.com to the test server is what makes the
// linking flow testable at all without editing the client. The swap is undone
// by t.Cleanup, and these tests must therefore never call t.Parallel.

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/ItemizeNTNU/website/internal/config"
	"github.com/ItemizeNTNU/website/internal/discord"
	"github.com/ItemizeNTNU/website/internal/fusionauth"
)

const (
	// A canonical UUID: fusionauth.ValidID refuses anything else before a
	// request is made, so a placeholder like "user-1" would turn every test
	// into an ErrInvalidID test.
	testUserID = "22222222-3333-4444-8555-666666666666"

	// Snowflakes: decimal digits and nothing else, or discord.ValidID refuses
	// them before a request goes out.
	testDiscordID  = "80351110224678912"
	otherDiscordID = "80351110224678913"
	testGuildID    = "111111111111111111"
	testRoleID     = "222222222222222222"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// callLog records every upstream call both fakes see, in order, so a test can
// assert not only what happened but in which order — and, just as often, that
// something did not happen at all.
type callLog struct {
	mu      sync.Mutex
	entries []string
}

func (l *callLog) add(entry string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

func (l *callLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.entries...)
}

// index returns the position of the first call containing substr, or -1.
func (l *callLog) index(substr string) int {
	for i, entry := range l.all() {
		if strings.Contains(entry, substr) {
			return i
		}
	}
	return -1
}

func (l *callLog) count(substr string) int {
	n := 0
	for _, entry := range l.all() {
		if strings.Contains(entry, substr) {
			n++
		}
	}
	return n
}

// mustNotCall fails the test when anything matching substr was called.
func (l *callLog) mustNotCall(t *testing.T, substr, why string) {
	t.Helper()
	if l.count(substr) != 0 {
		t.Errorf("%q was called %d time(s): %s. Calls: %v",
			substr, l.count(substr), why, l.all())
	}
}

// Path fragments the tests match on.
const (
	callToken   = "POST /api/v10/oauth2/token"
	callMe      = "GET /api/v10/users/@me"
	callAccount = "GET /api/v10/users/" + testDiscordID
	callMember  = "GET /api/v10/guilds/" + testGuildID + "/members/"
	callAddRole = "PUT /api/v10/guilds/"
	callDelRole = "DELETE /api/v10/guilds/"
	callGetUser = "fusion GET"
	callPatch   = "fusion PATCH"
)

// discordAPI stands in for Discord. The zero value answers every call
// successfully for an account that has not joined the guild; the status fields
// turn individual endpoints into failures.
type discordAPI struct {
	log *callLog

	account discord.User
	inGuild bool

	// A non-zero status makes that endpoint answer with a Discord-shaped error
	// body instead of succeeding.
	tokenStatus  int
	meStatus     int
	userStatus   int
	memberStatus int
	roleStatus   int

	// omitAccessToken answers the token endpoint with a 200 that carries no
	// access token — what a misconfigured OAuth application produces.
	omitAccessToken bool
}

func newDiscordAPI(log *callLog) *discordAPI {
	return &discordAPI{
		log: log,
		account: discord.User{
			ID:            testDiscordID,
			Username:      "kari",
			GlobalName:    "Kari N",
			Discriminator: "0",
			Avatar:        "abc123",
		},
	}
}

func (d *discordAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.log.add("discord " + r.Method + " " + r.URL.Path)

	path := r.URL.Path
	switch {
	case path == "/api/v10/oauth2/token":
		if d.wrote(w, d.tokenStatus) {
			return
		}
		if d.omitAccessToken {
			writeJSON(w, map[string]any{"token_type": "Bearer"})
			return
		}
		writeJSON(w, map[string]any{
			"access_token": "an-access-token", "token_type": "Bearer",
		})

	case path == "/api/v10/users/@me":
		if d.wrote(w, d.meStatus) {
			return
		}
		writeJSON(w, d.account)

	case strings.HasPrefix(path, "/api/v10/users/"):
		if d.wrote(w, d.userStatus) {
			return
		}
		writeJSON(w, d.account)

	// Role writes have to be recognised before the membership lookup: their
	// paths differ only by the /roles/ suffix.
	case strings.Contains(path, "/roles/"):
		if d.wrote(w, d.roleStatus) {
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case strings.Contains(path, "/members/"):
		if d.wrote(w, d.memberStatus) {
			return
		}
		if !d.inGuild {
			// Discord answers 404 for someone who has not joined.
			d.wrote(w, http.StatusNotFound)
			return
		}
		writeJSON(w, discord.GuildMember{User: &d.account, Roles: []string{}})

	default:
		d.log.add("discord UNEXPECTED " + path)
		http.Error(w, "unexpected Discord endpoint", http.StatusTeapot)
	}
}

// wrote answers with a Discord-shaped error when status is set, reporting
// whether it did.
func (d *discordAPI) wrote(w http.ResponseWriter, status int) bool {
	if status == 0 {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"code":50001,"message":"Missing Access"}`)
	return true
}

// fusionAPI stands in for the FusionAuth user API, recording every patch so a
// test can inspect exactly what would have been written to the member record.
type fusionAPI struct {
	log *callLog

	mu   sync.Mutex
	user fusionauth.User

	getStatus   int
	patchStatus int
	patches     []map[string]any
}

func newFusionAPI(log *callLog) *fusionAPI {
	return &fusionAPI{log: log, user: fusionauth.User{ID: testUserID}}
}

// withLink seeds the stored record with a Discord link, as if the member had
// already been through the flow once.
func (f *fusionAPI) withLink(id, username, avatar string, isMember bool) *fusionAPI {
	f.user.Data = map[string]any{"discord": map[string]any{
		"id": id, "username": username, "avatar": avatar, "isMember": isMember,
	}}
	f.user.ImageURL = avatar
	return f
}

func (f *fusionAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.log.add("fusion " + r.Method + " " + r.URL.Path)

	f.mu.Lock()
	defer f.mu.Unlock()

	switch r.Method {
	case http.MethodGet:
		if f.getStatus != 0 {
			w.WriteHeader(f.getStatus)
			_, _ = io.WriteString(w, `{"generalErrors":[{"message":"nope"}]}`)
			return
		}
		writeJSON(w, map[string]any{"user": f.user})

	case http.MethodPatch:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var envelope struct {
			User map[string]any `json:"user"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.patches = append(f.patches, envelope.User)

		if f.patchStatus != 0 {
			w.WriteHeader(f.patchStatus)
			_, _ = io.WriteString(w, `{"generalErrors":[{"message":"nope"}]}`)
			return
		}
		writeJSON(w, map[string]any{"user": f.user})

	default:
		http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
	}
}

// lastPatch returns the most recent patch body, failing when nothing was
// written — a link that never reached FusionAuth is lost the moment the
// request ends.
func (f *fusionAPI) lastPatch(t *testing.T) map[string]any {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.patches) == 0 {
		t.Fatal("no patch reached FusionAuth, so nothing about the Discord link was persisted")
	}
	return f.patches[len(f.patches)-1]
}

func (f *fusionAPI) patchCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.patches)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// discordRedirect points the Discord client at a local server.
//
// Any request for a host that is neither discord.com nor loopback fails the
// test instead of leaving the machine: these tests must pass offline, and a
// silent call to the real API would be both flaky and rude.
type discordRedirect struct {
	t      *testing.T
	base   http.RoundTripper
	target *url.URL
}

func (rt *discordRedirect) RoundTrip(req *http.Request) (*http.Response, error) {
	switch host := req.URL.Hostname(); host {
	case "discord.com":
		clone := req.Clone(req.Context())
		clone.URL.Scheme = rt.target.Scheme
		clone.URL.Host = rt.target.Host
		clone.Host = rt.target.Host
		return rt.base.RoundTrip(clone)

	case "127.0.0.1", "::1", "localhost":
		return rt.base.RoundTrip(req)

	default:
		rt.t.Errorf("a test tried to reach %s, which is neither the Discord fake "+
			"nor the FusionAuth fake; the suite must pass with no network", req.URL)
		return nil, fmt.Errorf("blocked request to %s", req.URL)
	}
}

// linkService wires a DiscordService onto the two fakes.
func linkService(t *testing.T, d *discordAPI, f *fusionAPI) *DiscordService {
	t.Helper()

	discordSrv := httptest.NewServer(d)
	t.Cleanup(discordSrv.Close)
	fusionSrv := httptest.NewServer(f)
	t.Cleanup(fusionSrv.Close)

	target, err := url.Parse(discordSrv.URL)
	if err != nil {
		t.Fatalf("the Discord fake's own URL does not parse: %v", err)
	}

	previous := http.DefaultTransport
	http.DefaultTransport = &discordRedirect{t: t, base: previous, target: target}
	t.Cleanup(func() { http.DefaultTransport = previous })

	client := discord.New(config.Discord{
		ClientID:     "client-1",
		ClientSecret: "secret-1",
		BotToken:     "bot-1",
		GuildID:      testGuildID,
		MemberRoleID: testRoleID,
	}, discardLogger())
	if client == nil {
		t.Fatal("discord.New returned nil despite a fully populated config")
	}

	return NewDiscordService(client, fusionauth.New(fusionSrv.URL, "test-api-key"),
		discardLogger())
}

// discordBlock digs the data.discord object out of a recorded patch body.
func discordBlock(t *testing.T, patch map[string]any) (map[string]any, bool) {
	t.Helper()
	data, ok := patch["data"].(map[string]any)
	if !ok {
		t.Fatalf("the patch carries no data object: %v", patch)
	}
	block, present := data["discord"]
	if !present {
		t.Fatalf("the patch does not mention data.discord at all, so the stored "+
			"link is left exactly as it was: %v", patch)
	}
	asMap, isMap := block.(map[string]any)
	return asMap, isMap
}
