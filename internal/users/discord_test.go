package users

// Edge cases in the Discord account link. The happy path is only half the
// story here: every step of the flow talks to a service we do not control, and
// what matters is that a failure in one of them leaves the member with a
// truthful profile rather than a lost link or a lie about their membership.

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/ItemizeNTNU/website/internal/discord"
	"github.com/ItemizeNTNU/website/internal/fusionauth"
)

// A service that is not fully wired must refuse every entry point with
// ErrUnavailable rather than panicking on a nil client. This is the ordinary
// state of a contributor's laptop, and of production for the minutes after a
// bot token is rotated — the site has to stay up through it.
func TestDiscordServiceUnavailable(t *testing.T) {
	configuredFusion := fusionauth.New("https://auth.invalid", "an-api-key")

	tests := []struct {
		name string
		svc  *DiscordService
	}{
		{"no service at all", nil},
		{"no Discord client", NewDiscordService(nil, configuredFusion, discardLogger())},
		{"FusionAuth has no API token", NewDiscordService(nil,
			fusionauth.New("https://auth.invalid", ""), discardLogger())},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.svc.Available() {
				t.Fatal("Available reported true for a half-configured service; " +
					"the handlers would start a flow that cannot finish")
			}

			if _, err := tt.svc.AuthorizeURL("state", "https://itemize.no/cb"); !errors.Is(err, ErrUnavailable) {
				t.Errorf("AuthorizeURL = %v, want ErrUnavailable", err)
			}
			if _, err := tt.svc.Complete(context.Background(), testUserID, "code", "https://itemize.no/cb"); !errors.Is(err, ErrUnavailable) {
				t.Errorf("Complete = %v, want ErrUnavailable", err)
			}
			if _, err := tt.svc.Refresh(context.Background(), testUserID); !errors.Is(err, ErrUnavailable) {
				t.Errorf("Refresh = %v, want ErrUnavailable", err)
			}
			if err := tt.svc.Unlink(context.Background(), testUserID); !errors.Is(err, ErrUnavailable) {
				t.Errorf("Unlink = %v, want ErrUnavailable", err)
			}
		})
	}
}

// A fully wired service reports itself available and hands out an authorize
// URL carrying the state it was given. The state is the only thing standing
// between the callback and a forged completion, so it must survive verbatim.
func TestAuthorizeURLCarriesTheState(t *testing.T) {
	log := &callLog{}
	svc := linkService(t, newDiscordAPI(log), newFusionAPI(log))

	if !svc.Available() {
		t.Fatal("a fully configured service reported itself unavailable")
	}

	got, err := svc.AuthorizeURL("a-random-state", "https://itemize.no/api/discord/callback")
	if err != nil {
		t.Fatalf("building the authorize URL failed: %v", err)
	}
	for _, want := range []string{
		"state=a-random-state",
		"redirect_uri=https%3A%2F%2Fitemize.no%2Fapi%2Fdiscord%2Fcallback",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the authorize URL is missing %q: %s", want, got)
		}
	}
	if len(log.all()) != 0 {
		t.Errorf("building a URL contacted an upstream service: %v", log.all())
	}
}

// The three answers Discord can give about guild membership are three
// different things to tell the member, and conflating any two of them either
// sends someone to join a server they are already in or hides an outage of
// ours behind a message blaming them.
func TestCompleteRecordsMembershipTruthfully(t *testing.T) {
	tests := []struct {
		name         string
		inGuild      bool
		memberStatus int

		wantMember  bool
		wantUnknown bool
		wantRole    bool
	}{
		{
			name:       "in the guild: the role is granted",
			inGuild:    true,
			wantMember: true, wantRole: true,
		},
		{
			name:    "not in the guild: nothing to grant",
			inGuild: false,
		},
		{
			// A 403 is the bot missing the guild members intent — our problem,
			// not an answer about this person.
			name:         "membership could not be checked",
			memberStatus: http.StatusForbidden,
			wantUnknown:  true,
		},
		{
			name:         "Discord is down while checking membership",
			memberStatus: http.StatusInternalServerError,
			wantUnknown:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := &callLog{}
			api := newDiscordAPI(log)
			api.inGuild = tt.inGuild
			api.memberStatus = tt.memberStatus
			fusion := newFusionAPI(log)
			svc := linkService(t, api, fusion)

			link, err := svc.Complete(context.Background(), testUserID, "a-code",
				"https://itemize.no/cb")
			if err != nil {
				t.Fatalf("completing the link failed: %v. A member who granted "+
					"access on Discord would be sent back with nothing to show "+
					"for it.", err)
			}

			if link.IsMember != tt.wantMember {
				t.Errorf("IsMember = %v, want %v", link.IsMember, tt.wantMember)
			}
			if link.MembershipUnknown != tt.wantUnknown {
				t.Errorf("MembershipUnknown = %v, want %v — an unknown answer shown "+
					"as \"has not joined\" tells a member to join a server they are "+
					"already in", link.MembershipUnknown, tt.wantUnknown)
			}
			if got := log.count(callAddRole) > 0; got != tt.wantRole {
				t.Errorf("role granted = %v, want %v (calls: %v)", got, tt.wantRole, log.all())
			}

			// Whatever happened with the guild, the link itself must be stored:
			// that is what the member came here to do.
			block, ok := discordBlock(t, fusion.lastPatch(t))
			if !ok {
				t.Fatal("data.discord was not written as an object")
			}
			if block["id"] != testDiscordID {
				t.Errorf("stored discord id = %v, want %q", block["id"], testDiscordID)
			}
			// Note what this pins in the two "unknown" cases: isMember is
			// written as false, and MembershipUnknown is not written at all.
			// The distinction the Link carries therefore survives exactly one
			// response — the next page load reads the record back through
			// CurrentLink and tells the member they have not joined.
			if block["isMember"] != tt.wantMember {
				t.Errorf("stored isMember = %v, want %v — the profile reads this "+
					"back on every page load", block["isMember"], tt.wantMember)
			}
			if _, stored := block["membershipUnknown"]; stored {
				t.Error("membershipUnknown reached storage; if that is now " +
					"deliberate, CurrentLink has to read it back")
			}
		})
	}
}

// The stored link is what the profile page renders without asking Discord, so
// it has to carry the display name and avatar in their final form.
func TestCompleteStoresTheRenderedAccount(t *testing.T) {
	log := &callLog{}
	api := newDiscordAPI(log)
	api.inGuild = true
	fusion := newFusionAPI(log)
	svc := linkService(t, api, fusion)

	link, err := svc.Complete(context.Background(), testUserID, "a-code", "https://itemize.no/cb")
	if err != nil {
		t.Fatalf("completing the link failed: %v", err)
	}

	const wantAvatar = "https://cdn.discordapp.com/avatars/" + testDiscordID + "/abc123.png?size=512"
	if link.Username != "Kari N" {
		t.Errorf("Username = %q, want the global name — a migrated account rendered "+
			"as \"kari#0\" is the exact bug the display rules exist to avoid", link.Username)
	}
	if link.Avatar != wantAvatar {
		t.Errorf("Avatar = %q, want %q", link.Avatar, wantAvatar)
	}

	patch := fusion.lastPatch(t)
	if patch["imageUrl"] != wantAvatar {
		t.Errorf("imageUrl = %v, want the Discord avatar so the profile picture "+
			"follows the linked account", patch["imageUrl"])
	}
	block, ok := discordBlock(t, patch)
	if !ok {
		t.Fatal("data.discord was not written as an object")
	}
	if block["username"] != "Kari N" || block["avatar"] != wantAvatar {
		t.Errorf("stored block = %v, want the rendered username and avatar", block)
	}
}

// The order is load-bearing: membership decides whether the role is granted,
// and the record is only written once both are known. A patch that landed
// first would advertise a role that was never granted.
func TestCompleteChecksMembershipBeforeGrantingAndStoring(t *testing.T) {
	log := &callLog{}
	api := newDiscordAPI(log)
	api.inGuild = true
	svc := linkService(t, api, newFusionAPI(log))

	if _, err := svc.Complete(context.Background(), testUserID, "a-code", "https://itemize.no/cb"); err != nil {
		t.Fatalf("completing the link failed: %v", err)
	}

	token, member, role, patch := log.index(callToken), log.index(callMember),
		log.index(callAddRole), log.index(callPatch)
	if token < 0 || member < 0 || role < 0 || patch < 0 {
		t.Fatalf("a step of the flow never ran: %v", log.all())
	}
	if !(token < member && member < role && role < patch) {
		t.Errorf("calls ran in the order %v; the token exchange must precede the "+
			"membership check, which must precede the role grant and the write", log.all())
	}
}

// Failures before the account is known must leave FusionAuth untouched. A
// half-finished OAuth round trip is not evidence of anything, and writing a
// link from it would attach whatever the last request happened to return.
func TestCompleteUpstreamFailuresStoreNothing(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		prepare func(*discordAPI)

		wantErr    error  // errors.Is, when set
		wantStatus int    // an *discord.APIError status, when set
		wantText   string // a substring of the message, when set
		wantCalls  []string
	}{
		{
			name:      "the member declined on Discord's consent screen",
			code:      "",
			wantErr:   discord.ErrDenied,
			wantCalls: nil, // an empty code never becomes a request
		},
		{
			name:       "the OAuth application credentials are wrong",
			code:       "a-code",
			prepare:    func(d *discordAPI) { d.tokenStatus = http.StatusUnauthorized },
			wantStatus: http.StatusUnauthorized,
			wantCalls:  []string{callToken},
		},
		{
			name:       "the authorization code was already used",
			code:       "a-code",
			prepare:    func(d *discordAPI) { d.tokenStatus = http.StatusBadRequest },
			wantStatus: http.StatusBadRequest,
			wantCalls:  []string{callToken},
		},
		{
			name:      "the token response carries no access token",
			code:      "a-code",
			prepare:   func(d *discordAPI) { d.omitAccessToken = true },
			wantText:  "no access token",
			wantCalls: []string{callToken},
		},
		{
			name:       "Discord will not say who the token belongs to",
			code:       "a-code",
			prepare:    func(d *discordAPI) { d.meStatus = http.StatusInternalServerError },
			wantStatus: http.StatusInternalServerError,
			wantCalls:  []string{callToken, callMe},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := &callLog{}
			api := newDiscordAPI(log)
			if tt.prepare != nil {
				tt.prepare(api)
			}
			fusion := newFusionAPI(log)
			svc := linkService(t, api, fusion)

			link, err := svc.Complete(context.Background(), testUserID, tt.code,
				"https://itemize.no/cb")
			if err == nil {
				t.Fatalf("a broken exchange was reported as success: %+v", link)
			}
			if link != nil {
				t.Errorf("a link was returned alongside an error: %+v", link)
			}

			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("got %v, want %v", err, tt.wantErr)
			}
			if tt.wantStatus != 0 {
				var apiErr *discord.APIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("got %T (%v), want a *discord.APIError so the handler "+
						"can tell an upstream refusal from a bug of ours", err, err)
				}
				if apiErr.Status != tt.wantStatus {
					t.Errorf("APIError.Status = %d, want %d", apiErr.Status, tt.wantStatus)
				}
			}
			if tt.wantText != "" && !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("error %q does not mention %q", err, tt.wantText)
			}

			if got := log.all(); len(got) != len(tt.wantCalls) {
				t.Errorf("calls = %v, want exactly %v", got, tt.wantCalls)
			}
			if fusion.patchCount() != 0 {
				t.Errorf("the member record was patched despite the flow failing "+
					"upstream: %v", fusion.patches)
			}
		})
	}
}

// Granting the role and storing the link are two writes to two systems, and
// either can fail after the other succeeded. Neither failure may take the
// other one down with it, but they are not symmetric: a missing role is
// recoverable with the refresh button, a missing record is not.
func TestCompletePartialFailures(t *testing.T) {
	t.Run("the role grant fails after membership was confirmed", func(t *testing.T) {
		log := &callLog{}
		api := newDiscordAPI(log)
		api.inGuild = true
		api.roleStatus = http.StatusForbidden
		fusion := newFusionAPI(log)
		svc := linkService(t, api, fusion)

		link, err := svc.Complete(context.Background(), testUserID, "a-code", "https://itemize.no/cb")
		if err != nil {
			t.Fatalf("a failed role grant lost the whole link: %v. The member "+
				"authorised us and would have nothing to show for it, with no "+
				"way to retry other than starting over.", err)
		}
		if !link.IsMember {
			t.Error("IsMember = false after Discord confirmed the membership; the " +
				"role grant failing says nothing about whether they joined")
		}
		if fusion.patchCount() != 1 {
			t.Errorf("the link was written %d times, want exactly once", fusion.patchCount())
		}
	})

	t.Run("storing the link fails after the role was granted", func(t *testing.T) {
		log := &callLog{}
		api := newDiscordAPI(log)
		api.inGuild = true
		fusion := newFusionAPI(log)
		fusion.patchStatus = http.StatusInternalServerError
		svc := linkService(t, api, fusion)

		link, err := svc.Complete(context.Background(), testUserID, "a-code", "https://itemize.no/cb")
		if err == nil {
			t.Fatal("a failed write was reported as a successful link; the profile " +
				"would show a link that does not exist")
		}
		if link != nil {
			t.Errorf("a link was returned alongside the write failure: %+v", link)
		}
		if !strings.Contains(err.Error(), "storing the Discord link") {
			t.Errorf("error %q does not say which step failed, which is the whole "+
				"point of wrapping it", err)
		}
		// The role was granted before the write failed, so Discord and
		// FusionAuth now disagree. Nothing rolls that back; the member has a
		// role and no stored link, and pressing link again repairs it.
		if log.count(callAddRole) != 1 {
			t.Errorf("the role grant did not run before the write: %v", log.all())
		}
	})
}

// A cancelled request — the member closed the tab, or the server is shutting
// down — must abort rather than run to completion against two upstreams.
func TestCompleteHonoursContextCancellation(t *testing.T) {
	log := &callLog{}
	fusion := newFusionAPI(log)
	svc := linkService(t, newDiscordAPI(log), fusion)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := svc.Complete(ctx, testUserID, "a-code", "https://itemize.no/cb"); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want a context.Canceled error", err)
	}
	if fusion.patchCount() != 0 {
		t.Error("a cancelled request still wrote to the member record")
	}
}

// Linking a second Discord account replaces the first without complaint, and
// nothing checks whether the incoming account is already linked to a different
// member. This pins the behaviour so the day someone wants uniqueness, the
// test that has to change says so plainly.
func TestCompleteReplacesAnExistingLink(t *testing.T) {
	log := &callLog{}
	api := newDiscordAPI(log)
	api.account.ID = otherDiscordID
	api.account.GlobalName = "Ola N"
	fusion := newFusionAPI(log).withLink(testDiscordID, "Kari N", "old-avatar", true)
	svc := linkService(t, api, fusion)

	link, err := svc.Complete(context.Background(), testUserID, "a-code", "https://itemize.no/cb")
	if err != nil {
		t.Fatalf("relinking failed: %v", err)
	}
	if link.ID != otherDiscordID {
		t.Errorf("link.ID = %q, want the newly authorised account %q", link.ID, otherDiscordID)
	}

	block, ok := discordBlock(t, fusion.lastPatch(t))
	if !ok {
		t.Fatal("data.discord was not written as an object")
	}
	if block["id"] != otherDiscordID {
		t.Errorf("stored id = %v, want the new account; the old one would keep "+
			"granting the profile page's Discord section", block["id"])
	}
	// The member's own record never had to be read to overwrite it.
	log.mustNotCall(t, callGetUser,
		"Complete overwrites the link without consulting what was stored")
}

// Refresh is the button a member presses after joining the server. It has to
// distinguish "you never linked" from "Discord is unhappy" — the first is
// something they can fix, the second is not.
func TestRefreshWithoutAStoredLink(t *testing.T) {
	tests := []struct {
		name string
		data map[string]any
	}{
		{"no data at all", nil},
		{"data without a discord block", map[string]any{"displayName": "kari"}},
		{"an empty discord block", map[string]any{"discord": map[string]any{}}},
		{"a discord block with a blank id", map[string]any{
			"discord": map[string]any{"id": "", "username": "kari"}}},
		// A hand-edited record, or one written by the previous site.
		{"discord stored as a string", map[string]any{"discord": "kari#1234"}},
		{"discord stored as a list", map[string]any{"discord": []any{"kari"}}},
		{"an id stored as a number", map[string]any{
			"discord": map[string]any{"id": 80351110224678912}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := &callLog{}
			fusion := newFusionAPI(log)
			fusion.user.Data = tt.data
			svc := linkService(t, newDiscordAPI(log), fusion)

			if _, err := svc.Refresh(context.Background(), testUserID); !errors.Is(err, ErrNotLinked) {
				t.Errorf("got %v, want ErrNotLinked so the page can say plainly "+
					"that nothing is connected", err)
			}
			log.mustNotCall(t, "discord ",
				"there is no account to ask Discord about")
			if fusion.patchCount() != 0 {
				t.Error("a refresh with nothing linked still rewrote the member record")
			}
		})
	}
}

// A refresh reconciles: the account may have been renamed, and the member may
// have joined the guild since they linked.
func TestRefreshReconcilesTheStoredLink(t *testing.T) {
	log := &callLog{}
	api := newDiscordAPI(log)
	api.inGuild = true
	api.account.GlobalName = "Kari Nordmann"
	api.account.Avatar = "def456"
	fusion := newFusionAPI(log).withLink(testDiscordID, "Kari N", "old-avatar", false)
	svc := linkService(t, api, fusion)

	link, err := svc.Refresh(context.Background(), testUserID)
	if err != nil {
		t.Fatalf("refreshing failed: %v", err)
	}
	if link.Username != "Kari Nordmann" {
		t.Errorf("Username = %q, want the renamed account — a refresh that keeps "+
			"the stale name makes the button look broken", link.Username)
	}
	if !link.IsMember {
		t.Error("IsMember = false after Discord confirmed the membership; this is " +
			"exactly what the member pressed the button to fix")
	}
	if log.count(callAddRole) != 1 {
		t.Errorf("the member role was not granted on refresh: %v", log.all())
	}
	if log.index(callGetUser) > log.index(callAccount) {
		t.Errorf("Discord was asked before the stored id was read: %v", log.all())
	}
}

// When the upstreams fail, a refresh must leave the stored link alone. Losing
// a link because Discord had a bad minute would mean re-authorising.
func TestRefreshUpstreamFailuresKeepTheLink(t *testing.T) {
	tests := []struct {
		name      string
		storedID  string
		prepare   func(*discordAPI, *fusionAPI)
		wantErr   error
		wantCalls int
	}{
		{
			name:     "FusionAuth cannot be read",
			storedID: testDiscordID,
			prepare:  func(_ *discordAPI, f *fusionAPI) { f.getStatus = http.StatusInternalServerError },
			// The read failed, so the id was never known.
			wantCalls: 1,
		},
		{
			name:     "the stored id is not a snowflake",
			storedID: "not-a-snowflake",
			wantErr:  discord.ErrInvalidID,
			// Refused before a request could be built from it.
			wantCalls: 1,
		},
		{
			name:      "Discord no longer knows the account",
			storedID:  testDiscordID,
			prepare:   func(d *discordAPI, _ *fusionAPI) { d.userStatus = http.StatusNotFound },
			wantCalls: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := &callLog{}
			api := newDiscordAPI(log)
			fusion := newFusionAPI(log).withLink(tt.storedID, "Kari N", "an-avatar", true)
			if tt.prepare != nil {
				tt.prepare(api, fusion)
			}
			svc := linkService(t, api, fusion)

			if _, err := svc.Refresh(context.Background(), testUserID); err == nil {
				t.Fatal("a failed refresh was reported as success")
			} else if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("got %v, want %v", err, tt.wantErr)
			}
			if got := len(log.all()); got != tt.wantCalls {
				t.Errorf("made %d upstream calls (%v), want %d", got, log.all(), tt.wantCalls)
			}
			if fusion.patchCount() != 0 {
				t.Error("a failed refresh rewrote the member record; a bad minute " +
					"upstream would cost the member their link")
			}
		})
	}
}

// A refresh for an identifier FusionAuth would never have issued is refused
// before it becomes a request — the id is concatenated into the upstream URL.
func TestRefreshAndUnlinkRejectMalformedUserIDs(t *testing.T) {
	log := &callLog{}
	svc := linkService(t, newDiscordAPI(log), newFusionAPI(log))

	for _, id := range []string{"", "not-a-uuid", "../key", testUserID + "?x=1"} {
		if _, err := svc.Refresh(context.Background(), id); !errors.Is(err, fusionauth.ErrInvalidID) {
			t.Errorf("Refresh(%q) = %v, want ErrInvalidID", id, err)
		}
		if err := svc.Unlink(context.Background(), id); !errors.Is(err, fusionauth.ErrInvalidID) {
			t.Errorf("Unlink(%q) = %v, want ErrInvalidID", id, err)
		}
	}
	if len(log.all()) != 0 {
		t.Errorf("a malformed user id still produced upstream calls: %v", log.all())
	}
}

// Unlinking is the one operation that must succeed even when Discord will not
// cooperate: the member asked us to forget the connection, and the role is
// Discord's business.
func TestUnlinkClearsTheRecordDespiteDiscord(t *testing.T) {
	tests := []struct {
		name       string
		storedID   string
		roleStatus int
	}{
		{"the role was withdrawn cleanly", testDiscordID, 0},
		{"they already left the guild", testDiscordID, http.StatusNotFound},
		{"the bot token was rejected", testDiscordID, http.StatusUnauthorized},
		{"Discord is down", testDiscordID, http.StatusInternalServerError},
		{"the stored id is malformed", "not-a-snowflake", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := &callLog{}
			api := newDiscordAPI(log)
			api.roleStatus = tt.roleStatus
			fusion := newFusionAPI(log).withLink(tt.storedID, "Kari N", "an-avatar", true)
			svc := linkService(t, api, fusion)

			if err := svc.Unlink(context.Background(), testUserID); err != nil {
				t.Fatalf("unlinking failed with %v; whatever Discord answers, the "+
					"member asked us to forget the link and that must happen", err)
			}

			block, isMap := discordBlock(t, fusion.lastPatch(t))
			if isMap {
				t.Errorf("data.discord was patched to %v; only an explicit null "+
					"deletes a key in a merge patch, and anything else leaves the "+
					"link in place", block)
			}
		})
	}
}

// The avatar came from Discord, so it goes with the link — but only when it is
// still the Discord one. Clearing a picture the member set themselves would be
// deleting something we never gave them.
func TestUnlinkClearsOnlyTheDiscordAvatar(t *testing.T) {
	tests := []struct {
		name      string
		imageURL  string
		wantClear bool
	}{
		{"the profile picture is the linked avatar", "an-avatar", true},
		{"the member has a picture of their own", "https://example.no/me.png", false},
		{"no picture at all", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := &callLog{}
			fusion := newFusionAPI(log).withLink(testDiscordID, "Kari N", "an-avatar", true)
			fusion.user.ImageURL = tt.imageURL
			svc := linkService(t, newDiscordAPI(log), fusion)

			if err := svc.Unlink(context.Background(), testUserID); err != nil {
				t.Fatalf("unlinking failed: %v", err)
			}

			patch := fusion.lastPatch(t)
			value, mentioned := patch["imageUrl"]
			switch {
			case tt.wantClear && (!mentioned || value != nil):
				t.Errorf("imageUrl = %v (mentioned=%v), want an explicit null — the "+
					"profile would keep showing a picture from an account that is "+
					"no longer connected", value, mentioned)
			case !tt.wantClear && mentioned:
				t.Errorf("imageUrl was set to %v, but the picture did not come from "+
					"the linked account and is not ours to delete", value)
			}
		})
	}
}

func TestUnlinkWithoutALink(t *testing.T) {
	log := &callLog{}
	fusion := newFusionAPI(log) // no discord block
	svc := linkService(t, newDiscordAPI(log), fusion)

	if err := svc.Unlink(context.Background(), testUserID); !errors.Is(err, ErrNotLinked) {
		t.Errorf("got %v, want ErrNotLinked", err)
	}
	log.mustNotCall(t, callDelRole, "there is no account whose role could be withdrawn")
	if fusion.patchCount() != 0 {
		t.Error("a no-op unlink still rewrote the member record")
	}
}

func TestUnlinkReportsAFailedWrite(t *testing.T) {
	log := &callLog{}
	fusion := newFusionAPI(log).withLink(testDiscordID, "Kari N", "an-avatar", true)
	fusion.patchStatus = http.StatusInternalServerError
	svc := linkService(t, newDiscordAPI(log), fusion)

	err := svc.Unlink(context.Background(), testUserID)
	if err == nil {
		t.Fatal("a failed write was reported as a successful unlink; the member " +
			"would be told they are disconnected while the link is still stored")
	}
	if !strings.Contains(err.Error(), "clearing the Discord link") {
		t.Errorf("error %q does not say which step failed", err)
	}
	// The role was withdrawn first and is not restored, which is the safe way
	// round: the member keeps no access they asked to give up.
	if log.count(callDelRole) != 1 {
		t.Errorf("the role was not withdrawn before the write: %v", log.all())
	}
}

func TestUnlinkPropagatesAFailedRead(t *testing.T) {
	log := &callLog{}
	fusion := newFusionAPI(log)
	fusion.getStatus = http.StatusInternalServerError
	svc := linkService(t, newDiscordAPI(log), fusion)

	if err := svc.Unlink(context.Background(), testUserID); err == nil {
		t.Fatal("an unreadable member record was treated as a successful unlink")
	} else if errors.Is(err, ErrNotLinked) {
		t.Error("an unreadable record was reported as \"nothing is linked\"; the " +
			"member would be told their link is gone when it is not")
	}
	log.mustNotCall(t, callDelRole, "the stored id was never read")
}

// CurrentLink reads whatever FusionAuth happens to hold, including records
// written by the previous site and by hand. Every shape has to yield either a
// usable link or nothing — never a panic on a page every signed-in member
// loads.
func TestCurrentLinkTolerantOfStoredShapes(t *testing.T) {
	tests := []struct {
		name string
		user *fusionauth.User
		want *Link
	}{
		{"no user", nil, nil},
		{"no data", &fusionauth.User{ID: testUserID}, nil},
		{"data without a discord block",
			&fusionauth.User{Data: map[string]any{"type": "student"}}, nil},
		{"discord stored as a string",
			&fusionauth.User{Data: map[string]any{"discord": "kari#1234"}}, nil},
		{"discord stored as null",
			&fusionauth.User{Data: map[string]any{"discord": nil}}, nil},
		{"an empty block",
			&fusionauth.User{Data: map[string]any{"discord": map[string]any{}}}, nil},
		{"an id that is not a string",
			&fusionauth.User{Data: map[string]any{
				"discord": map[string]any{"id": 42}}}, nil},
		{
			// JSON numbers decode as float64, and a link whose id survived but
			// whose other fields did not is still a link.
			name: "everything but the id is the wrong type",
			user: &fusionauth.User{Data: map[string]any{"discord": map[string]any{
				"id": testDiscordID, "username": 1, "avatar": nil, "isMember": "yes",
			}}},
			want: &Link{ID: testDiscordID},
		},
		{
			name: "a complete record",
			user: &fusionauth.User{Data: map[string]any{"discord": map[string]any{
				"id": testDiscordID, "username": "Kari N",
				"avatar": "https://example.no/a.png", "isMember": true,
			}}},
			want: &Link{ID: testDiscordID, Username: "Kari N",
				Avatar: "https://example.no/a.png", IsMember: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CurrentLink(tt.user)
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("got %+v, want no link — the profile would render a Discord "+
					"section for an account that is not linked", got)
			case tt.want != nil && got == nil:
				t.Fatalf("got no link, want %+v — a stored link would disappear from "+
					"the profile", tt.want)
			case tt.want != nil && *got != *tt.want:
				t.Errorf("got %+v, want %+v", *got, *tt.want)
			}

			// MembershipUnknown is not stored: it describes a single attempt to
			// reach Discord, not a fact about the member.
			if got != nil && got.MembershipUnknown {
				t.Error("MembershipUnknown was read back from storage; it only means " +
					"that one particular check failed")
			}
		})
	}
}

// linkedAvatar is what decides whether Unlink also clears the profile picture,
// so it has to agree with CurrentLink on every shape rather than reading the
// map a second way.
func TestLinkedIDAndAvatarFollowCurrentLink(t *testing.T) {
	full := &fusionauth.User{Data: map[string]any{"discord": map[string]any{
		"id": testDiscordID, "avatar": "https://example.no/a.png",
	}}}
	if got := linkedID(full); got != testDiscordID {
		t.Errorf("linkedID = %q, want %q", got, testDiscordID)
	}
	if got := linkedAvatar(full); got != "https://example.no/a.png" {
		t.Errorf("linkedAvatar = %q, want the stored avatar", got)
	}

	for _, u := range []*fusionauth.User{nil, {}, {Data: map[string]any{"discord": 1}}} {
		if got := linkedID(u); got != "" {
			t.Errorf("linkedID(%v) = %q, want the empty string", u, got)
		}
		if got := linkedAvatar(u); got != "" {
			t.Errorf("linkedAvatar(%v) = %q, want the empty string", u, got)
		}
	}
}
