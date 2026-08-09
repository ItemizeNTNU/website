package discord

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ItemizeNTNU/website/internal/config"
)

// New returns nil for anything short of a complete configuration, and that nil
// is the whole opt-out mechanism: it is what lets a contributor run the site
// without a bot token. If a partially configured Discord ever produced a live
// client instead, local development would start making authenticated calls
// with half-empty credentials and failing in ways that look like outages.
func TestNewReturnsNilUntilFullyConfigured(t *testing.T) {
	full := testConfig()

	tests := []struct {
		name string
		cfg  config.Discord
	}{
		{"nothing set at all", config.Discord{}},
		{"missing client id", config.Discord{ClientSecret: "s", BotToken: "b", GuildID: "g", MemberRoleID: "r"}},
		{"missing client secret", config.Discord{ClientID: "c", BotToken: "b", GuildID: "g", MemberRoleID: "r"}},
		{"missing bot token", config.Discord{ClientID: "c", ClientSecret: "s", GuildID: "g", MemberRoleID: "r"}},
		{"missing guild id", config.Discord{ClientID: "c", ClientSecret: "s", BotToken: "b", MemberRoleID: "r"}},
		{"missing member role id", config.Discord{ClientID: "c", ClientSecret: "s", BotToken: "b", GuildID: "g"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(tt.cfg, discardLogger())
			if c != nil {
				t.Fatal("a partially configured Discord produced a live client; " +
					"the site would try to talk to Discord with incomplete credentials")
			}
			if c.Enabled() {
				t.Error("Enabled() reported true for a client that does not exist")
			}
		})
	}

	if c := New(full, discardLogger()); c == nil || !c.Enabled() {
		t.Fatal("a fully configured Discord did not produce a usable client")
	}
}

// Enabled is the only method safe to call on the nil client New hands back;
// everything else dereferences the configuration. Callers therefore have to
// gate on Enabled, and this pins that contract so a refactor cannot quietly
// turn "disabled" into "silently succeeds" — which would leave event sync
// reporting success while publishing nothing.
func TestDisabledClientMustBeGatedOnEnabled(t *testing.T) {
	var c *Client

	if c.Enabled() {
		t.Fatal("a nil client reported itself enabled")
	}

	defer func() {
		if recover() == nil {
			t.Error("a call on the disabled client neither panicked nor was " +
				"gated; if this becomes a silent no-op, callers lose the " +
				"signal that Discord was never contacted")
		}
	}()
	_, _ = c.GetUser(context.Background(), testSnowflake)
}

// Every bot call carries the same three things: the bot token, a descriptive
// user agent, and the versioned path. Discord rate limits the default Go user
// agent harder, and a request built against /api/v8 reaches a deprecated API.
func TestBotRequestShape(t *testing.T) {
	c, fake := newFakeDiscord(t, jsonReply(200, `{"id":"`+testSnowflake+`"}`))

	if _, err := c.GetUser(context.Background(), testSnowflake); err != nil {
		t.Fatalf("a well-formed lookup failed: %v", err)
	}

	got := fake.first()
	if got.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.Method)
	}
	if want := "/api/v10/users/" + testSnowflake; got.Path != want {
		t.Errorf("path = %q, want %q — a wrong path reaches a different endpoint", got.Path, want)
	}
	if want := "Bot bot-1"; got.Header.Get("Authorization") != want {
		t.Errorf("Authorization = %q, want %q; anything else is a 401",
			got.Header.Get("Authorization"), want)
	}
	if got.Header.Get("User-Agent") != userAgent {
		t.Errorf("User-Agent = %q, want the descriptive bot agent %q — the default "+
			"Go agent is rate limited harder", got.Header.Get("User-Agent"), userAgent)
	}
	// No body means no Content-Type: claiming JSON on an empty GET is a lie
	// that some proxies act on.
	if ct := got.Header.Get("Content-Type"); ct != "" {
		t.Errorf("Content-Type = %q on a bodyless request, want none", ct)
	}
	if len(got.Body) != 0 {
		t.Errorf("a GET carried a body: %q", got.Body)
	}
}

// A request with a payload must announce JSON, or Discord rejects it before
// looking at the content.
func TestRequestWithBodyDeclaresJSON(t *testing.T) {
	c, fake := newFakeDiscord(t, jsonReply(200, `{"id":"999"}`))

	_, err := c.UpsertScheduledEvent(context.Background(), "", ScheduledEvent{Name: "Hack"})
	if err != nil {
		t.Fatalf("creating an event failed: %v", err)
	}

	got := fake.first()
	if ct := got.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json; Discord will not parse the payload otherwise", ct)
	}
	var decoded map[string]any
	if err := json.Unmarshal(got.Body, &decoded); err != nil {
		t.Fatalf("the request body was not JSON (%v): %q", err, got.Body)
	}
}

// Discord answers errors as JSON with its own numeric code, and losing either
// the HTTP status or that code turns a diagnosable failure ("missing
// permissions") into "something went wrong".
func TestNonSuccessStatusesBecomeAPIError(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantMsg string
		wantErr string
	}{
		{"bad request", 400, `{"message":"Invalid Form Body","code":50035}`, "Invalid Form Body", "HTTP 400, code 50035"},
		{"forbidden", 403, `{"message":"Missing Permissions","code":50013}`, "Missing Permissions", "HTTP 403, code 50013"},
		{"not found", 404, `{"message":"Unknown User","code":10013}`, "Unknown User", "HTTP 404, code 10013"},
		{"rate limited past the retry", 429, `{"message":"You are being rate limited.","code":0}`, "You are being rate limited.", "HTTP 429"},
		{"server error", 500, `{"message":"Internal Server Error","code":0}`, "Internal Server Error", "HTTP 500"},
		{"bad gateway with no body", 502, ``, "", "discord: HTTP 502"},
		{"html error page from a proxy", 503, `<html>maintenance</html>`, "", "discord: HTTP 503"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := jsonReply(tt.status, tt.body)
			c, _ := newFakeDiscord(t, func(w http.ResponseWriter, r *http.Request) {
				// The 429 case is answered twice, once for the retry. The
				// header keeps that retry immediate instead of adding the
				// one-second fallback wait to the suite.
				w.Header().Set("Retry-After", "0.001")
				reply(w, r)
			})

			_, err := c.GetUser(context.Background(), testSnowflake)

			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("got %T (%v), want *APIError — callers branch on the status", err, err)
			}
			if apiErr.Status != tt.status {
				t.Errorf("Status = %d, want %d", apiErr.Status, tt.status)
			}
			if apiErr.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q; a lost message leaves nothing to act on", apiErr.Message, tt.wantMsg)
			}
			if !strings.Contains(apiErr.Error(), tt.wantErr) {
				t.Errorf("Error() = %q, want it to mention %q", apiErr.Error(), tt.wantErr)
			}
		})
	}
}

// A 401 on a bot call is nearly always the token itself, and "HTTP 401" sends
// nobody anywhere. The message names the variable and the mistake people
// actually make, so the fix is one line rather than a support thread.
func TestUnauthorizedErrorNamesTheBotToken(t *testing.T) {
	err := (&APIError{Status: http.StatusUnauthorized, Message: "401: Unauthorized", Code: 0}).Error()

	for _, want := range []string{"DISCORD_BOT_TOKEN", "401", "client secret"} {
		if !strings.Contains(err, want) {
			t.Errorf("the 401 message does not mention %q: %s", want, err)
		}
	}
}

// A body that is not JSON at all must not mask the status. The status is the
// only thing GuildMember and RemoveMemberRole branch on, so losing it turns
// "not in the guild" into a hard failure.
func TestMalformedErrorBodyStillCarriesTheStatus(t *testing.T) {
	c, _ := newFakeDiscord(t, jsonReply(404, `{"message": `))

	_, err := c.GuildMember(context.Background(), testSnowflake)
	if err != nil {
		t.Fatalf("a 404 with a broken body should still read as 'not a member', got %v", err)
	}
}

// A truncated or non-JSON success body is a failure, not an empty result.
// Silently returning a zero value here would store an empty Discord link
// against a real account.
func TestMalformedSuccessBodyIsAnError(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"cut off mid-object", `{"id":"803511102246789`},
		{"cut off mid-array", `{"roles":["1","2"`},
		{"not JSON at all", `<!doctype html><title>Cloudflare</title>`},
		{"a bare fragment", `id=1234`},
		{"wrong shape entirely", `[1,2,3]`},
		{"NUL bytes", "\x00\x00\x00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newFakeDiscord(t, jsonReply(200, tt.body))
			if _, err := c.GetUser(context.Background(), testSnowflake); err == nil {
				t.Fatal("an unparseable success body was accepted; the caller would " +
					"store a blank account as though the lookup had worked")
			}
		})
	}
}

// A 2xx with nothing in it is the normal answer to PUT and DELETE. It must not
// be treated as a decode failure, or granting a role would report an error
// every single time it worked.
func TestEmptySuccessBodyIsNotAnError(t *testing.T) {
	for _, status := range []int{200, 201, 204} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, _ := newFakeDiscord(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			})
			if err := c.AddMemberRole(context.Background(), testSnowflake); err != nil {
				t.Fatalf("an empty %d was reported as a failure: %v", status, err)
			}
		})
	}
}

// The response is read through a limit, so a misbehaving or compromised
// endpoint cannot make the process allocate without bound. Past the cap the
// JSON is necessarily incomplete, which is an error rather than a silent
// partial decode.
func TestOversizedResponseIsCappedRatherThanRead(t *testing.T) {
	c, _ := newFakeDiscord(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"`)
		_, _ = io.WriteString(w, repeat('a', maxBody*2))
		_, _ = io.WriteString(w, `"}`)
	})

	done := make(chan error, 1)
	go func() { _, err := c.GetUser(context.Background(), testSnowflake); done <- err }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a response larger than the cap decoded successfully, which " +
				"means the cap is not being applied")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reading an oversized response never finished; the limit reader is gone")
	}
}

// A connection that dies partway through the response is different from a
// malformed one: the failure happens while reading, before there is anything
// to parse. It must surface as an error rather than as an account with the
// half of the fields that arrived.
func TestConnectionDroppedMidResponse(t *testing.T) {
	c, _ := newFakeDiscord(t, truncatedReply)

	u, err := c.GetUser(context.Background(), testSnowflake)
	if err == nil {
		t.Fatal("a response cut short by a dropped connection was accepted; the " +
			"member would be linked to whatever partial account arrived")
	}
	if u != nil {
		t.Errorf("got %+v alongside the error", u)
	}
}

// Event sync fires on every save, so a 429 is a live possibility rather than a
// theoretical one. The previous server ignored 429 entirely, which surfaced as
// an event that silently failed to publish.
func TestRateLimitIsRetriedOnce(t *testing.T) {
	c, fake := newFakeDiscord(t, sequence(
		rateLimited,
		jsonReply(200, `{"id":"`+testSnowflake+`"}`),
	))

	u, err := c.GetUser(context.Background(), testSnowflake)
	if err != nil {
		t.Fatalf("a rate-limited call was not retried: %v", err)
	}
	if u.ID != testSnowflake {
		t.Errorf("id = %q after the retry, want %q", u.ID, testSnowflake)
	}
	if n := fake.count(); n != 2 {
		t.Errorf("Discord saw %d requests, want 2 (the original and one retry)", n)
	}
}

// The retry rebuilds the request from scratch. If the body reader were created
// outside the loop it would already be drained, and the second attempt would
// send an empty payload — Discord would answer 400 and the event would never
// appear, with the 429 nowhere in the error.
func TestRetryResendsTheRequestBody(t *testing.T) {
	c, fake := newFakeDiscord(t, sequence(
		rateLimited,
		jsonReply(200, `{"id":"evt-1"}`),
	))

	_, err := c.UpsertScheduledEvent(context.Background(), "", ScheduledEvent{
		Name:     "Ukas CTF",
		Location: "A4-104",
	})
	if err != nil {
		t.Fatalf("the retried create failed: %v", err)
	}

	got := fake.requests()
	if len(got) != 2 {
		t.Fatalf("got %d requests, want 2", len(got))
	}
	if len(got[1].Body) == 0 {
		t.Fatal("the retry sent an empty body; Discord would reject it and the " +
			"event would never be published")
	}
	if string(got[0].Body) != string(got[1].Body) {
		t.Errorf("the retry sent a different payload:\nfirst:  %s\nsecond: %s", got[0].Body, got[1].Body)
	}
}

// One retry, not a loop. A Discord that answers 429 forever must surface as an
// error rather than pinning a request goroutine indefinitely.
func TestRateLimitGivesUpAfterOneRetry(t *testing.T) {
	c, fake := newFakeDiscord(t, rateLimited)

	err := c.AddMemberRole(context.Background(), testSnowflake)

	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusTooManyRequests {
		t.Fatalf("got %v, want an APIError carrying 429", err)
	}
	if n := fake.count(); n != 2 {
		t.Errorf("Discord saw %d requests, want exactly 2 — the retry must not loop", n)
	}
}

// A long Retry-After is real: Discord hands out multi-second waits under a
// global limit. The wait has to be abandonable, or a cancelled HTTP request
// would still hold its goroutine for the whole window.
func TestRateLimitWaitHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c, fake := newFakeDiscord(t, func(w http.ResponseWriter, r *http.Request) {
		// Cancelling before the reply means the client reaches the wait with
		// an already-dead context: no sleeping, no timing assumptions.
		cancel()
		w.Header().Set("Retry-After", "600")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	start := time.Now()
	err := c.AddMemberRole(ctx, testSnowflake)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want a cancellation; the caller has gone away", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("the cancelled call took %s — it waited out the Retry-After", elapsed)
	}
	if n := fake.count(); n != 1 {
		t.Errorf("Discord saw %d requests, want 1 — a cancelled call must not retry", n)
	}
}

// Discord reports the wait in two different headers depending on the endpoint
// and whether the limit is global. Reading neither means always waiting the
// one-second fallback, which either retries too early (and gets another 429)
// or too late.
func TestRetryAfterHeaderParsing(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   time.Duration
	}{
		{"Retry-After in whole seconds", http.Header{"Retry-After": {"2"}}, 2 * time.Second},
		{"Retry-After fractional", http.Header{"Retry-After": {"0.25"}}, 250 * time.Millisecond},
		{"the reset-after header when Retry-After is absent",
			http.Header{"X-Ratelimit-Reset-After": {"1.5"}}, 1500 * time.Millisecond},
		{"Retry-After wins over reset-after",
			http.Header{"Retry-After": {"3"}, "X-Ratelimit-Reset-After": {"9"}}, 3 * time.Second},
		{"no headers at all", http.Header{}, time.Second},
		{"an empty Retry-After falls through",
			http.Header{"Retry-After": {""}, "X-Ratelimit-Reset-After": {"4"}}, 4 * time.Second},
		// RFC 7231 allows an HTTP-date here. Discord sends seconds, so this is
		// a documented gap rather than a bug: an unparseable value falls back
		// to a second, which retries early but never hangs.
		{"an HTTP-date falls back rather than failing",
			http.Header{"Retry-After": {"Wed, 21 Oct 2015 07:28:00 GMT"}}, time.Second},
		{"garbage falls back", http.Header{"Retry-After": {"soon"}}, time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryAfter(&http.Response{Header: tt.header}); got != tt.want {
				t.Errorf("waiting %s, want %s — the retry lands at the wrong moment", got, tt.want)
			}
		})
	}
}

// A context that is already dead must stop the call before it leaves the
// process, and the error must still be recognisable as a cancellation so the
// caller does not log a request timeout as a Discord outage.
func TestAlreadyCancelledContextMakesNoRequest(t *testing.T) {
	c, fake := newFakeDiscord(t, jsonReply(200, `{"id":"1"}`))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.GetUser(ctx, testSnowflake)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if n := fake.count(); n != 0 {
		t.Errorf("Discord saw %d requests from a cancelled context, want 0", n)
	}
}

// A Discord that accepts the connection and then stops responding is exactly
// what the client timeout exists for. The deadline must win, and it must come
// back as a deadline so a caller can tell it apart from a rejected token.
func TestDeadlineExceededWhileWaiting(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	c, _ := newFakeDiscord(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := c.GetUser(ctx, testSnowflake)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want context.DeadlineExceeded from a Discord that never answers", err)
	}
}

// When Discord is unreachable the error has to say which call failed. A bare
// "connection refused" in the logs identifies neither the endpoint nor the
// operation that was lost.
func TestTransportFailureNamesTheCall(t *testing.T) {
	c, fake := newFakeDiscord(t, nil)
	// Taking the fake off the air gives a deterministic refusal on a port
	// nothing else is listening to.
	fake.stop()

	err := c.AddMemberRole(context.Background(), testSnowflake)
	if err == nil {
		t.Fatal("an unreachable Discord was reported as success")
	}
	for _, want := range []string{"discord:", "PUT", "/guilds/guild-1/members/"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %q, so the logs will not say what was lost: %v", want, err)
		}
	}
}

// Identifiers are concatenated into request paths and carry the bot token, so
// the shape check is a path-traversal guard as much as a validation. These are
// the boundaries the pattern draws.
func TestValidIDBoundaries(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"1", true},
		{testSnowflake, true},
		{"0000000000000000000", true},
		{repeat('9', 25), true},
		{repeat('9', 26), false},
		{"", false},
		{"12.34", false},
		{"12-34", false},
		{"+123", false},
		{"-123", false},
		{"0x1f", false},
		// Go's $ does not match before a trailing newline, but a reader used to
		// other regexp dialects would assume it does; a smuggled newline would
		// split the request line.
		{"123\n", false},
		{"\n123", false},
		{"123 ", false},
		{"12 34", false},
		// Unicode digits are digits to a human and not to Discord.
		{"１２３", false},
		{"١٢٣", false},
		{"../../users/@me", false},
		{"123/roles/999", false},
		{"123?limit=1", false},
		{"123#frag", false},
		{"12%2f34", false},
	}

	for _, tt := range tests {
		t.Run("id="+tt.id, func(t *testing.T) {
			if got := ValidID(tt.id); got != tt.want {
				t.Errorf("ValidID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}
