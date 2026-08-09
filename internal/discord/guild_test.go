package discord

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// A member record arrives with the account nested inside it and the role list
// alongside. The display name and avatar shown on the profile page are built
// from that nested account, so a decode that drops it renders a blank link.
func TestGuildMemberDecodesTheRecord(t *testing.T) {
	c, fake := newFakeDiscord(t, jsonReply(200, `{
		"user": {
			"id": "80351110224678912",
			"username": "kari",
			"global_name": "Kari Nordmann æøå 🚩",
			"discriminator": "0",
			"avatar": "a_1234567890abcdef"
		},
		"nick": "Kari (styret)",
		"roles": ["role-1", "role-2"]
	}`))

	m, err := c.GuildMember(context.Background(), testSnowflake)
	if err != nil {
		t.Fatalf("looking up a member failed: %v", err)
	}
	if m == nil {
		t.Fatal("a member who is in the guild was reported as absent")
	}
	if m.User == nil {
		t.Fatal("the nested account was dropped; the profile page would show a blank link")
	}
	if m.User.ID != "80351110224678912" {
		t.Errorf("id = %q", m.User.ID)
	}
	// Norwegian letters and emoji are ordinary in a Discord display name, and
	// a mangled one is stored verbatim against the account.
	if want := "Kari Nordmann æøå 🚩"; m.User.DisplayName() != want {
		t.Errorf("display name = %q, want %q", m.User.DisplayName(), want)
	}
	if m.Nick != "Kari (styret)" {
		t.Errorf("nick = %q", m.Nick)
	}
	if len(m.Roles) != 2 || m.Roles[0] != "role-1" {
		t.Errorf("roles = %v, want both roles in order", m.Roles)
	}

	got := fake.first()
	if got.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.Method)
	}
	if want := "/api/v10/guilds/guild-1/members/" + testSnowflake; got.Path != want {
		t.Errorf("path = %q, want %q", got.Path, want)
	}
}

// Discord omits fields it has nothing to say about: no nickname, no roles, no
// avatar. Those must decode as absent rather than as a failure, or every
// member without a nickname would look like an outage.
func TestGuildMemberWithMissingOptionalFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"an empty object", `{}`},
		{"no nickname and no roles", `{"user":{"id":"80351110224678912","username":"kari"}}`},
		{"an explicit null nickname", `{"user":{"id":"1"},"nick":null,"roles":null}`},
		{"roles present but empty", `{"user":{"id":"1"},"roles":[]}`},
		{"an unknown field Discord added later", `{"user":{"id":"1"},"flags":42,"banner":null}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newFakeDiscord(t, jsonReply(200, tt.body))

			m, err := c.GuildMember(context.Background(), testSnowflake)
			if err != nil {
				t.Fatalf("a sparse but valid member record was rejected: %v", err)
			}
			if m == nil {
				t.Fatal("a member who is in the guild was reported as absent")
			}
		})
	}
}

// A nil member with a nil error is the ordinary case: somebody linked their
// account and has not followed the invite yet. Reporting that as an error
// would break the linking flow for every new member.
func TestGuildMemberNotInTheGuild(t *testing.T) {
	c, _ := newFakeDiscord(t, jsonReply(404, `{"message":"Unknown Member","code":10007}`))

	m, err := c.GuildMember(context.Background(), testSnowflake)
	if err != nil {
		t.Fatalf("somebody who has not joined yet produced an error: %v", err)
	}
	if m != nil {
		t.Errorf("got a member record for somebody who is not in the guild: %+v", m)
	}
}

// Without the guild members intent Discord answers 403, not 404. Folding that
// into "not a member" would tell every linked member they had not joined, and
// the actual fix — a checkbox in the developer portal — would never surface.
func TestGuildMemberForbiddenIsNotAnAnswerAboutMembership(t *testing.T) {
	c, _ := newFakeDiscord(t, jsonReply(403, `{"message":"Missing Access","code":50001}`))

	m, err := c.GuildMember(context.Background(), testSnowflake)
	if err == nil {
		t.Fatal("a missing guild members intent was reported as 'not a member'; " +
			"every linked member would be told they had not joined")
	}
	if m != nil {
		t.Errorf("got a member record alongside the error: %+v", m)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusForbidden {
		t.Errorf("got %v, want an APIError carrying 403", err)
	}
}

// Anything else is a real failure and must not be reported as an answer about
// this person, or a rejected token would silently strip everybody's role.
func TestGuildMemberPropagatesOtherFailures(t *testing.T) {
	for _, status := range []int{400, 401, 429, 500, 502, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, _ := newFakeDiscord(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "0.001")
				w.WriteHeader(status)
			})

			m, err := c.GuildMember(context.Background(), testSnowflake)
			if err == nil {
				t.Fatalf("HTTP %d was reported as a successful lookup", status)
			}
			if m != nil {
				t.Errorf("got a member record alongside the %d: %+v", status, m)
			}
		})
	}
}

// Granting the role is a PUT against the member's role path, built from the
// configured guild and role. A wrong role id grants the wrong permissions;
// a wrong method is a 405.
func TestAddMemberRoleRequestShape(t *testing.T) {
	cfg := testConfig()
	cfg.GuildID = "111111111111111111"
	cfg.MemberRoleID = "222222222222222222"

	c, fake := newFakeDiscordWith(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.AddMemberRole(context.Background(), testSnowflake); err != nil {
		t.Fatalf("granting the member role failed: %v", err)
	}

	got := fake.first()
	if got.Method != http.MethodPut {
		t.Errorf("method = %q, want PUT", got.Method)
	}
	want := "/api/v10/guilds/111111111111111111/members/" + testSnowflake +
		"/roles/222222222222222222"
	if got.Path != want {
		t.Errorf("path = %q, want %q — the wrong path grants the wrong role", got.Path, want)
	}
	if got.Header.Get("Authorization") != "Bot bot-1" {
		t.Errorf("Authorization = %q, want the bot token", got.Header.Get("Authorization"))
	}
	if len(got.Body) != 0 {
		t.Errorf("the role grant carried a body: %q", got.Body)
	}
}

// Failing to grant the role has to be visible. Reporting success would leave
// somebody linked but without access to the channels the role opens, with
// nothing in the logs to explain it.
func TestAddMemberRolePropagatesFailures(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, _ := newFakeDiscord(t, jsonReply(status, `{"message":"nope","code":50013}`))

			err := c.AddMemberRole(context.Background(), testSnowflake)
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Status != status {
				t.Fatalf("got %v, want an APIError carrying %d; the member would be "+
					"left without the access the role grants", err, status)
			}
		})
	}
}

// Somebody who already left the guild, or already lost the role, is not an
// error: the desired state has been reached either way. Unlinking must not
// fail because Discord has already forgotten them.
func TestRemoveMemberRoleTreatsMissingAsDone(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{"the role was removed", 204, ``, false},
		{"the member already left the guild", 404, `{"message":"Unknown Member","code":10007}`, false},
		{"the role no longer exists", 404, `{"message":"Unknown Role","code":10011}`, false},
		{"the bot lost its permissions", 403, `{"message":"Missing Permissions","code":50013}`, true},
		{"the bot token was rejected", 401, `{"message":"401: Unauthorized","code":0}`, true},
		{"Discord is down", 500, ``, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newFakeDiscord(t, jsonReply(tt.status, tt.body))

			err := c.RemoveMemberRole(context.Background(), testSnowflake)
			if tt.wantErr && err == nil {
				t.Fatalf("HTTP %d was swallowed; the member keeps access they should have lost", tt.status)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("HTTP %d should read as 'already done', got %v", tt.status, err)
			}
		})
	}
}

func TestRemoveMemberRoleRequestShape(t *testing.T) {
	c, fake := newFakeDiscord(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.RemoveMemberRole(context.Background(), testSnowflake); err != nil {
		t.Fatal(err)
	}

	got := fake.first()
	if got.Method != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", got.Method)
	}
	want := "/api/v10/guilds/guild-1/members/" + testSnowflake + "/roles/role-1"
	if got.Path != want {
		t.Errorf("path = %q, want %q", got.Path, want)
	}
}

// Every guild call validates the identifier before building a path with it,
// because that identifier is concatenated straight into the URL. A rejected
// one must never reach the network, or a crafted value would aim an
// authenticated request at a different endpoint.
func TestGuildCallsRejectMalformedIdentifiers(t *testing.T) {
	malformed := []string{
		"", "abc", "../../users/@me", "..%2fguilds", "123?q=1", "12 34",
		"123/roles/999", "1234567890123456789012345678901234567890",
	}

	// A slice rather than a map: the order these run in should not change
	// between runs, and neither should the subtest names in a failure report.
	calls := []struct {
		name string
		call func(*Client, string) error
	}{
		{"GuildMember", func(c *Client, id string) error {
			_, err := c.GuildMember(context.Background(), id)
			return err
		}},
		{"AddMemberRole", func(c *Client, id string) error {
			return c.AddMemberRole(context.Background(), id)
		}},
		{"RemoveMemberRole", func(c *Client, id string) error {
			return c.RemoveMemberRole(context.Background(), id)
		}},
		{"GetUser", func(c *Client, id string) error {
			_, err := c.GetUser(context.Background(), id)
			return err
		}},
	}

	for _, tc := range calls {
		for _, id := range malformed {
			t.Run(tc.name+"/"+id, func(t *testing.T) {
				c, fake := newFakeDiscord(t, jsonReply(200, `{"id":"1"}`))

				if err := tc.call(c, id); !errors.Is(err, ErrInvalidID) {
					t.Errorf("%s(%q) = %v, want ErrInvalidID", tc.name, id, err)
				}
				if n := fake.count(); n != 0 {
					t.Errorf("%s(%q) sent %d authenticated requests for a malformed id", tc.name, id, n)
				}
			})
		}
	}
}
