package discord

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Discord identifiers are concatenated into request paths the same way, and
// carry the bot token. A snowflake is decimal digits and nothing else.
func TestDiscordIDsAreValidated(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	c := &Client{cfg: testConfig(), http: srv.Client(), log: discardLogger()}

	for _, id := range []string{
		"../../users/@me", "..%2fguilds", "123?q=1", "", "abc", "12 34",
	} {
		called = false
		if _, err := c.GetUser(context.Background(), id); err != ErrInvalidID {
			t.Errorf("GetUser(%q) = %v, want ErrInvalidID", id, err)
		}
		if err := c.AddMemberRole(context.Background(), id); err != ErrInvalidID {
			t.Errorf("AddMemberRole(%q) = %v, want ErrInvalidID", id, err)
		}
		if called {
			t.Errorf("a request went out for the malformed id %q", id)
		}
	}
}

func TestSnowflakesAreAccepted(t *testing.T) {
	for _, id := range []string{"80351110224678912", "1", "1234567890123456789"} {
		if !ValidID(id) {
			t.Errorf("ValidID(%q) = false, want true", id)
		}
	}
}
