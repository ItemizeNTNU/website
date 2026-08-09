package discord

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ItemizeNTNU/website/internal/config"
)

// decodePayload reads the scheduled-event body the client sent.
func decodePayload(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("the request body was not JSON (%v): %s", err, raw)
	}
	return out
}

// Whether an event is created or updated is decided entirely by the stored
// identifier. Getting the branch wrong either duplicates every event on each
// save (POST when it should PATCH) or tries to update an event that does not
// exist yet (PATCH on an empty id, which is a 404 against /scheduled-events/).
func TestUpsertChoosesCreateOrUpdateByStoredID(t *testing.T) {
	tests := []struct {
		name       string
		existingID string
		wantMethod string
		wantPath   string
	}{
		{
			"no stored id creates",
			"",
			http.MethodPost,
			"/api/v10/guilds/guild-1/scheduled-events",
		},
		{
			"a stored id updates that event",
			"1234567890",
			http.MethodPatch,
			"/api/v10/guilds/guild-1/scheduled-events/1234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, fake := newFakeDiscord(t, jsonReply(200, `{"id":"9876543210"}`))

			id, err := c.UpsertScheduledEvent(context.Background(), tt.existingID, ScheduledEvent{
				Name: "Ukas CTF",
			})
			if err != nil {
				t.Fatalf("the event was not published: %v", err)
			}
			if id != "9876543210" {
				t.Errorf("id = %q, want the id Discord returned; losing it orphans the event", id)
			}

			got := fake.first()
			if got.Method != tt.wantMethod {
				t.Errorf("method = %q, want %q", got.Method, tt.wantMethod)
			}
			if got.Path != tt.wantPath {
				t.Errorf("path = %q, want %q", got.Path, tt.wantPath)
			}
		})
	}
}

// The payload Discord requires: an external event, visible to the guild only,
// with times in RFC 3339. A missing entity_type or privacy_level is a 400 that
// reaches the board as "Error upserting discord event".
func TestUpsertPayloadShape(t *testing.T) {
	c, fake := newFakeDiscord(t, jsonReply(200, `{"id":"1"}`))

	oslo := time.FixedZone("CET", 3600)
	start := time.Date(2026, 3, 14, 17, 0, 0, 0, oslo)
	end := time.Date(2026, 3, 14, 20, 30, 0, 0, oslo)

	_, err := c.UpsertScheduledEvent(context.Background(), "", ScheduledEvent{
		Name:        "Ukas CTF",
		Description: "Vi løser oppgaver",
		Location:    "A4-104, Gløshaugen",
		Start:       start,
		End:         end,
	})
	if err != nil {
		t.Fatal(err)
	}

	body := decodePayload(t, fake.first().Body)

	// Discord rejects an unknown entity type outright, and the wrong privacy
	// level would publish the event beyond the guild.
	if body["entity_type"] != float64(entityTypeExternal) {
		t.Errorf("entity_type = %v, want %d (external)", body["entity_type"], entityTypeExternal)
	}
	if body["privacy_level"] != float64(privacyGuildOnly) {
		t.Errorf("privacy_level = %v, want %d (guild only)", body["privacy_level"], privacyGuildOnly)
	}
	if body["name"] != "Ukas CTF" {
		t.Errorf("name = %v", body["name"])
	}
	if body["description"] != "Vi løser oppgaver" {
		t.Errorf("description = %v", body["description"])
	}

	meta, ok := body["entity_metadata"].(map[string]any)
	if !ok {
		t.Fatalf("entity_metadata is missing or the wrong shape: %v", body["entity_metadata"])
	}
	// An external event with no location is rejected, and the venue is the one
	// detail a member needs before turning up.
	if meta["location"] != "A4-104, Gløshaugen" {
		t.Errorf("location = %v, want the venue", meta["location"])
	}

	// The offset has to survive. Sending a wall-clock time without one would
	// move every event by an hour for half the year.
	if got, want := body["scheduled_start_time"], start.Format(time.RFC3339); got != want {
		t.Errorf("scheduled_start_time = %v, want %v", got, want)
	}
	if got, want := body["scheduled_end_time"], end.Format(time.RFC3339); got != want {
		t.Errorf("scheduled_end_time = %v, want %v", got, want)
	}
	if !strings.Contains(body["scheduled_start_time"].(string), "+01:00") {
		t.Errorf("the start time lost its zone offset: %v", body["scheduled_start_time"])
	}
}

// Discord's description limit is 1000 while an event's info field allows 2000,
// with the venue and CTF lines prepended on top. The previous version sent the
// lot and Discord rejected the whole request.
func TestUpsertTruncatesToDiscordsLimits(t *testing.T) {
	c, fake := newFakeDiscord(t, jsonReply(200, `{"id":"1"}`))

	_, err := c.UpsertScheduledEvent(context.Background(), "", ScheduledEvent{
		Name:        repeat('æ', eventNameMax+50),
		Description: repeat('ø', eventDescriptionMax+500),
	})
	if err != nil {
		t.Fatalf("an over-long event was not shortened but rejected: %v", err)
	}

	body := decodePayload(t, fake.first().Body)

	name := body["name"].(string)
	if n := len([]rune(name)); n != eventNameMax {
		t.Errorf("name is %d runes, want %d — Discord rejects the whole request past its limit", n, eventNameMax)
	}
	if !strings.HasSuffix(name, "…") {
		t.Errorf("the shortened name does not show that it was cut: %q", name[len(name)-10:])
	}
	// Counting bytes rather than runes would both overshoot the limit and cut
	// a multi-byte character in half, producing mojibake in the event title.
	if strings.ContainsRune(name, '�') {
		t.Error("the name was cut mid-character; it was truncated by bytes, not runes")
	}

	desc := body["description"].(string)
	if n := len([]rune(desc)); n != eventDescriptionMax {
		t.Errorf("description is %d runes, want %d", n, eventDescriptionMax)
	}
}

// truncate is what stands between a board member's prose and a rejected
// request, so its boundaries matter more than its middle.
func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"well inside the limit is untouched", "Ukas CTF", 100, "Ukas CTF"},
		{"empty stays empty", "", 100, ""},
		{"exactly at the limit is untouched", "abcde", 5, "abcde"},
		{"one over loses two and gains the ellipsis", "abcdef", 5, "abcd…"},
		{"the result is never longer than the limit", repeat('a', 500), 10, repeat('a', 9) + "…"},
		// Norwegian text and emoji are multi-byte. A byte-based cut would both
		// exceed the rune limit and split a character.
		{"multi-byte characters count as one", "æøåæøå", 6, "æøåæøå"},
		{"emoji are not split", "🎉🎉🎉🎉", 3, "🎉🎉…"},
		{"mixed scripts", "CTF på Gløshaugen 🚩", 10, "CTF på Gl…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.in, tt.max)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
			if n := len([]rune(got)); n > tt.max {
				t.Errorf("the result is %d runes, over the %d limit Discord enforces", n, tt.max)
			}
			if strings.ContainsRune(got, '�') && !strings.ContainsRune(tt.in, '�') {
				t.Errorf("truncate produced a replacement character: %q", got)
			}
		})
	}
}

// A failed publish has to reach the caller. Reporting success would leave the
// site showing an event that exists nowhere in Discord.
func TestUpsertPropagatesFailures(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"rejected payload", 400, `{"message":"Invalid Form Body","code":50035}`},
		{"bad bot token", 401, `{"message":"401: Unauthorized","code":0}`},
		{"bot cannot manage events", 403, `{"message":"Missing Permissions","code":50013}`},
		{"the event was deleted in Discord", 404, `{"message":"Unknown Guild Scheduled Event","code":10070}`},
		{"Discord is down", 500, ``},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newFakeDiscord(t, jsonReply(tt.status, tt.body))

			id, err := c.UpsertScheduledEvent(context.Background(), "1234", ScheduledEvent{Name: "x"})
			if err == nil {
				t.Fatal("a rejected event was reported as published")
			}
			if id != "" {
				t.Errorf("id = %q on failure, want empty; a bogus id would be stored", id)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Status != tt.status {
				t.Errorf("got %v, want an APIError carrying %d", err, tt.status)
			}
		})
	}
}

// The guild identifier comes from configuration, so it must be the configured
// one that ends up in the path rather than anything baked in.
func TestScheduledEventPathUsesTheConfiguredGuild(t *testing.T) {
	cfg := testConfig()
	cfg.GuildID = "987654321098765432"

	c, fake := newFakeDiscordWith(t, cfg, jsonReply(200, `{"id":"1"}`))

	if _, err := c.UpsertScheduledEvent(context.Background(), "", ScheduledEvent{}); err != nil {
		t.Fatal(err)
	}
	if want := "/api/v10/guilds/987654321098765432/scheduled-events"; fake.first().Path != want {
		t.Errorf("path = %q, want %q", fake.first().Path, want)
	}
}

// Someone deleting the event in Discord directly is ordinary, and the site
// deleting it again afterwards must not be an error — the desired state has
// been reached either way. Treating it as a failure would block the save that
// removes the event from the site.
func TestDeleteOfAMissingEventSucceeds(t *testing.T) {
	c, fake := newFakeDiscord(t, jsonReply(404, `{"message":"Unknown Guild Scheduled Event","code":10070}`))

	if err := c.DeleteScheduledEvent(context.Background(), "1234567890"); err != nil {
		t.Fatalf("deleting an already-deleted event failed: %v", err)
	}
	if n := fake.count(); n != 1 {
		t.Errorf("Discord saw %d requests, want 1", n)
	}
}

// An event that was never published to Discord has no id, and there is nothing
// to delete. Building a path with an empty segment would hit the collection
// endpoint instead of an event.
func TestDeleteWithoutAnIDMakesNoRequest(t *testing.T) {
	c, fake := newFakeDiscord(t, jsonReply(500, `{}`))

	if err := c.DeleteScheduledEvent(context.Background(), ""); err != nil {
		t.Fatalf("deleting an unpublished event failed: %v", err)
	}
	if n := fake.count(); n != 0 {
		t.Errorf("Discord saw %d requests for an event that was never published, want 0", n)
	}
}

func TestDeleteScheduledEventRequestShape(t *testing.T) {
	c, fake := newFakeDiscord(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.DeleteScheduledEvent(context.Background(), "1234567890"); err != nil {
		t.Fatal(err)
	}

	got := fake.first()
	if got.Method != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", got.Method)
	}
	if want := "/api/v10/guilds/guild-1/scheduled-events/1234567890"; got.Path != want {
		t.Errorf("path = %q, want %q", got.Path, want)
	}
	if got.Header.Get("Authorization") != "Bot bot-1" {
		t.Errorf("Authorization = %q, want the bot token", got.Header.Get("Authorization"))
	}
}

// Only 404 means "already gone". A 403 means the bot lost its permissions and
// the event is still there; swallowing that would leave a stale event in the
// guild with nothing in the logs.
func TestDeletePropagatesEverythingButNotFound(t *testing.T) {
	for _, status := range []int{400, 401, 403, 500, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, _ := newFakeDiscord(t, jsonReply(status, `{"message":"nope","code":50013}`))

			err := c.DeleteScheduledEvent(context.Background(), "1234567890")
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Status != status {
				t.Fatalf("got %v, want an APIError carrying %d; a stale event would be left behind", err, status)
			}
		})
	}
}

// The description Discord shows puts the practical details above the prose,
// in a fixed order. The previous version used the venue URL for the
// registration line, so anyone following "Registrering" landed on a map.
func TestBuildDescription(t *testing.T) {
	const sep = "--------------------------------------------------"

	tests := []struct {
		name        string
		info        string
		locationURL string
		registerURL string
		ctfName     string
		ctfURL      string
		want        string
	}{
		{
			name: "nothing but prose is passed through untouched",
			info: "Vi møtes som vanlig.",
			want: "Vi møtes som vanlig.",
		},
		{
			name: "no prose and no details is empty",
			want: "",
		},
		{
			name:        "registration comes before the venue",
			info:        "Bli med!",
			locationURL: "https://link.mazemap.com/abc",
			registerURL: "https://itemize.no/arrangement/ctf",
			want: "Registrering: https://itemize.no/arrangement/ctf\n" +
				"Hvor: https://link.mazemap.com/abc\n" +
				sep + "\n\nBli med!",
		},
		{
			name:    "a CTF with a link",
			info:    "Vi løser oppgaver.",
			ctfName: "HackTheBox Uni",
			ctfURL:  "https://ctftime.org/event/1",
			want:    "CTF: HackTheBox Uni (https://ctftime.org/event/1)\n" + sep + "\n\nVi løser oppgaver.",
		},
		{
			name:    "a CTF without a link drops the parentheses",
			info:    "Vi løser oppgaver.",
			ctfName: "Intern CTF",
			want:    "CTF: Intern CTF\n" + sep + "\n\nVi løser oppgaver.",
		},
		{
			// A URL with no name would render as "CTF: ()", which reads as a
			// bug to anybody looking at the event.
			name:   "a CTF link without a name is left out entirely",
			info:   "Prose.",
			ctfURL: "https://ctftime.org/event/1",
			want:   "Prose.",
		},
		{
			name:        "details with no prose still get the separator",
			locationURL: "https://link.mazemap.com/abc",
			want:        "Hvor: https://link.mazemap.com/abc\n" + sep + "\n\n",
		},
		{
			name:        "everything at once, in order",
			info:        "Møt opp!",
			locationURL: "https://link.mazemap.com/abc",
			registerURL: "https://itemize.no/p/x",
			ctfName:     "Ukas CTF æøå 🚩",
			ctfURL:      "https://ctftime.org/event/2",
			want: "Registrering: https://itemize.no/p/x\n" +
				"Hvor: https://link.mazemap.com/abc\n" +
				"CTF: Ukas CTF æøå 🚩 (https://ctftime.org/event/2)\n" +
				sep + "\n\nMøt opp!",
		},
		{
			name:        "prose containing newlines is preserved",
			info:        "Første linje\n\nAndre linje",
			registerURL: "https://itemize.no/p/x",
			want:        "Registrering: https://itemize.no/p/x\n" + sep + "\n\nFørste linje\n\nAndre linje",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildDescription(tt.info, tt.locationURL, tt.registerURL, tt.ctfName, tt.ctfURL)
			if got != tt.want {
				t.Errorf("the event description reads wrong.\ngot:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// The description is assembled before it is truncated, so a long info field
// plus the detail lines must still leave a payload Discord accepts.
func TestAssembledDescriptionIsTruncatedBeforeSending(t *testing.T) {
	c, fake := newFakeDiscord(t, jsonReply(200, `{"id":"1"}`))

	// 2000 runes is what an event's info field allows, before the detail lines
	// and separator are prepended.
	description := BuildDescription(
		repeat('å', 2000),
		"https://link.mazemap.com/abc",
		"https://itemize.no/p/x",
		"Ukas CTF",
		"https://ctftime.org/event/1",
	)
	if len([]rune(description)) <= eventDescriptionMax {
		t.Fatalf("the fixture is not long enough to exercise truncation (%d runes)", len([]rune(description)))
	}

	if _, err := c.UpsertScheduledEvent(context.Background(), "", ScheduledEvent{
		Description: description,
	}); err != nil {
		t.Fatal(err)
	}

	body := decodePayload(t, fake.first().Body)
	if n := len([]rune(body["description"].(string))); n > eventDescriptionMax {
		t.Errorf("sent a %d-rune description; Discord rejects anything past %d and "+
			"the board sees only 'Error upserting discord event'", n, eventDescriptionMax)
	}
}

// A zero time is what an event saved without an end date carries. It must
// still produce a parseable timestamp rather than something Discord chokes on
// in a way that hides the real problem.
func TestUpsertWithZeroTimes(t *testing.T) {
	c, fake := newFakeDiscord(t, jsonReply(200, `{"id":"1"}`))

	if _, err := c.UpsertScheduledEvent(context.Background(), "", ScheduledEvent{Name: "x"}); err != nil {
		t.Fatal(err)
	}

	body := decodePayload(t, fake.first().Body)
	for _, key := range []string{"scheduled_start_time", "scheduled_end_time"} {
		value, _ := body[key].(string)
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			t.Errorf("%s = %q is not RFC 3339: %v", key, value, err)
		}
	}
}

// Discord answering 200 with no id leaves the caller with nothing to update or
// delete by. This pins the current behaviour — the empty string is returned
// and the caller stores it — so a change to it is deliberate.
func TestUpsertWithoutAnIDInTheResponse(t *testing.T) {
	c, _ := newFakeDiscord(t, jsonReply(200, `{}`))

	id, err := c.UpsertScheduledEvent(context.Background(), "", ScheduledEvent{Name: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "" {
		t.Errorf("id = %q, want empty", id)
	}
}

// The event methods take the guild from configuration and nothing else, so a
// fresh client built by New has to carry it through untouched.
func TestNewCarriesTheConfigurationThrough(t *testing.T) {
	cfg := config.Discord{
		ClientID:     "111",
		ClientSecret: "shh",
		BotToken:     "tok",
		GuildID:      "222",
		MemberRoleID: "333",
	}
	c := New(cfg, discardLogger())
	if c == nil {
		t.Fatal("a complete configuration produced no client")
	}
	if c.cfg != cfg {
		t.Errorf("the client changed the configuration: %+v", c.cfg)
	}
	// http.DefaultClient has no timeout, so a hung Discord would pin
	// goroutines until the process restarted.
	if c.http == nil || c.http.Timeout == 0 {
		t.Error("the client has no request timeout; a hung Discord would pin goroutines forever")
	}
}
