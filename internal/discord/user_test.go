package discord

import (
	"strconv"
	"strings"
	"testing"
)

// Discord retired discriminators in the "pomelo" migration. Migrated accounts
// report the literal "0", so the previous server's unconditional
// "username#discriminator" would now render every linked member as "name#0".
func TestDisplayName(t *testing.T) {
	tests := []struct {
		name string
		user User
		want string
	}{
		{
			"migrated account with a global name",
			User{Username: "kari", GlobalName: "Kari N", Discriminator: "0"},
			"Kari N",
		},
		{
			"migrated account without a global name",
			User{Username: "kari", Discriminator: "0"},
			"kari",
		},
		{
			"legacy account keeps its discriminator",
			User{Username: "kari", Discriminator: "1234"},
			"kari#1234",
		},
		{
			"no discriminator at all",
			User{Username: "kari"},
			"kari",
		},
		{
			"a global name wins over a legacy discriminator",
			User{Username: "kari", GlobalName: "Kari N", Discriminator: "1234"},
			"Kari N",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.DisplayName(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAvatarURLUsesTheUploadedAvatar(t *testing.T) {
	u := User{ID: "80351110224678912", Avatar: "abc123"}
	want := "https://cdn.discordapp.com/avatars/80351110224678912/abc123.png?size=512"
	if got := u.AvatarURL(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The default-avatar index changed with the migration: legacy accounts index
// by discriminator modulo 5, migrated ones by the snowflake shifted right 22
// bits modulo 6 — a different input and a different divisor.
func TestDefaultAvatarIndex(t *testing.T) {
	legacy := User{ID: "80351110224678912", Discriminator: "1234"}
	if got := legacy.defaultAvatarIndex(); got != 1234%5 {
		t.Errorf("legacy: got %d, want %d", got, 1234%5)
	}

	migrated := User{ID: "80351110224678912", Discriminator: "0"}
	want := uint64(80351110224678912) >> 22 % 6
	if got := migrated.defaultAvatarIndex(); got != want {
		t.Errorf("migrated: got %d, want %d", got, want)
	}
	if got := migrated.defaultAvatarIndex(); got > 5 {
		t.Errorf("index %d is outside the 0–5 range Discord defines", got)
	}
}

// A snowflake exceeds 2^53, so anything reading it as a float — which a direct
// port of the JavaScript would — loses precision and lands in the wrong
// bucket. This pins the value against float64 arithmetic.
func TestSnowflakeIsParsedAsUint64(t *testing.T) {
	// Chosen so that float64 rounding changes the result.
	const id = "1234567890123456789"

	exact, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	viaFloat := uint64(float64(exact))
	if exact == viaFloat {
		t.Skip("this identifier does not distinguish the two paths")
	}

	u := User{ID: id, Discriminator: "0"}
	if got, want := u.defaultAvatarIndex(), (exact>>22)%6; got != want {
		t.Errorf("got %d, want %d — the snowflake is losing precision", got, want)
	}
}

func TestAvatarURLFallsBackToDefault(t *testing.T) {
	u := User{ID: "80351110224678912", Discriminator: "0"}
	got := u.AvatarURL()
	if !strings.HasPrefix(got, "https://cdn.discordapp.com/embed/avatars/") {
		t.Errorf("got %q, want a default avatar URL", got)
	}
}

// A malformed identifier must not panic; the placeholder is cosmetic.
func TestDefaultAvatarIndexHandlesGarbage(t *testing.T) {
	u := User{ID: "not-a-snowflake", Discriminator: "0"}
	if got := u.defaultAvatarIndex(); got != 0 {
		t.Errorf("got %d, want the 0 fallback", got)
	}
}

func TestAuthorizeURLRequestsOnlyIdentify(t *testing.T) {
	c := &Client{cfg: testConfig()}
	got := c.AuthorizeURL("random-state", "https://itemize.no/api/discord/callback")

	for _, want := range []string{
		"scope=identify",
		"state=random-state",
		"response_type=code",
		"client_id=client-1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("authorize URL is missing %q: %s", want, got)
		}
	}
	// Anything broader than identify would be asking for access we do not use.
	if strings.Contains(got, "guilds.join") || strings.Contains(got, "email") {
		t.Errorf("authorize URL requests more than identify: %s", got)
	}
	// The authorization endpoint is not under the versioned API path.
	if strings.Contains(got, "/api/v10/oauth2/authorize") {
		t.Errorf("authorize URL uses the versioned API path: %s", got)
	}
}
