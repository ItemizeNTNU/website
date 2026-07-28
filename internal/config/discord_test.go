package config

import "testing"

// A rejected bot token is the single most common Discord misconfiguration, and
// the two causes below are invisible: the value looks right in an editor and
// in `printenv`, and the failure appears at the first API call, far from the
// configuration that caused it.
func TestBotTokenNormalisation(t *testing.T) {
	const real = "MTIzNDU2Nzg5.GaBcDe.FgHiJkLmNoPqRsTuVwXyZ"

	tests := map[string]string{
		"plain token":                real,
		"trailing newline":           real + "\n",
		"leading and trailing space": "  " + real + "  ",
		// Copied along with the prefix from Discord's documentation. We add
		// "Bot " ourselves, so this would send "Bot Bot xxx" and 401.
		"with the Bot prefix":           "Bot " + real,
		"with the Bot prefix and space": "  Bot " + real + "\n",
		"empty":                         "",
	}

	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			got := botToken(raw)
			want := real
			if raw == "" {
				want = ""
			}
			if got != want {
				t.Errorf("botToken(%q) = %q, want %q", raw, got, want)
			}
		})
	}
}

// "Bot" appearing inside a token must not be stripped — only the prefix
// followed by a space.
func TestBotTokenLeavesRealTokensAlone(t *testing.T) {
	for _, token := range []string{"Bottle123.abc.def", "BotToken.abc.def"} {
		if got := botToken(token); got != token {
			t.Errorf("botToken(%q) = %q; the value was mangled", token, got)
		}
	}
}
