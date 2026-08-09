package config

// No test in this file may call t.Parallel: every one of them uses t.Setenv
// or t.Chdir, both of which panic in a parallel test. Load reads process-wide
// state, so the whole package runs sequentially by design.

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// configEnv is every variable Load consults. Each test clears all of them
// before setting the ones it cares about, so a developer's own shell — which on
// this project very plausibly has BASE_URL or MONGO_DB_URL exported — can
// neither rescue a case that should fail nor break one that should pass.
var configEnv = []string{
	"NODE_ENV", "ENV",
	"PORT", "LISTEN",
	"BASE_URL",
	"FUSION_AUTH_HOST",
	"FUSION_AUTH_CLIENT_ID",
	"FUSION_AUTH_CLIENT_SECRET",
	"FUSION_AUTH_SECRET",
	"FUSION_AUTH_API_TOKEN",
	"FUSION_AUTH_ID_TOKEN_ALG",
	"FUSION_AUTH_ID_TOKEN_HMAC_SECRET",
	"MONGO_DB_URL",
	"MONGO_DB_NAME",
	"DISCORD_CLIENT_ID",
	"DISCORD_CLIENT_SECRET",
	"DISCORD_BOT_TOKEN",
	"DISCORD_SERVER_ID",
	"DISCORD_SERVER_MEMBER_ROLE_ID",
}

// testSecret is exactly minSecretLen bytes, the shortest session key Load
// accepts.
const testSecret = "0123456789abcdef0123456789abcdef"

// validEnv is the smallest environment that starts the server in development:
// everything Load insists on and nothing else.
func validEnv() map[string]string {
	return map[string]string{
		"BASE_URL":                  "https://itemize.no",
		"FUSION_AUTH_HOST":          "https://auth.itemize.no",
		"FUSION_AUTH_CLIENT_ID":     "5c1b8e2a-0000-4000-8000-000000000001",
		"FUSION_AUTH_CLIENT_SECRET": "client-secret",
		"FUSION_AUTH_SECRET":        testSecret,
		"MONGO_DB_URL":              "mongodb://localhost:27017/website",
	}
}

// withEnv installs env as the complete configuration environment and moves the
// test into an empty directory.
//
// The chdir is not incidental. Load seeds from ./.env outside production, so
// without it a stray .env in whatever directory the test binary happens to run
// from would leak into the result — and, worse, would leak differently on a
// contributor's machine than in CI.
func withEnv(t *testing.T, env map[string]string) {
	t.Helper()
	t.Chdir(t.TempDir())
	for _, key := range configEnv {
		// The empty string is what Load treats as unset everywhere it uses
		// os.Getenv, and it also stops loadDotenv from filling the key in.
		t.Setenv(key, "")
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
}

// wantErrors fails unless err mentions every listed problem. Load joins its
// findings so one restart reveals all of them; asserting on substrings is what
// proves an operator fixing a deployment is not sent round the loop twice.
func wantErrors(t *testing.T, err error, substrings ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Load accepted an invalid environment; the server would start misconfigured instead of refusing to boot (expected %v)", substrings)
	}
	for _, want := range substrings {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Load's error does not mention %q, so the operator never learns about that problem.\nfull error: %v", want, err)
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	withEnv(t, validEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("the documented minimum environment was rejected: %v", err)
	}

	checks := []struct {
		what string
		got  string
		want string
	}{
		{"Env", cfg.Env, "development"},
		{"Addr", cfg.Addr, ":3000"},
		{"BaseURL", cfg.BaseURL.String(), "https://itemize.no"},
		{"FusionAuth.Host", cfg.FusionAuth.Host.String(), "https://auth.itemize.no"},
		{"FusionAuth.IDTokenAlg", cfg.FusionAuth.IDTokenAlg, "HS256"},
		{"Mongo.Database", cfg.Mongo.Database, "website"},
		{"Mongo.URI", cfg.Mongo.URI, "mongodb://localhost:27017/website"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("cfg.%s = %q, want %q", c.what, c.got, c.want)
		}
	}
	if !cfg.Dev {
		// Dev governs TLS enforcement and whether .env is read at all; getting
		// it wrong turns a local checkout into a half-production server.
		t.Error("an unset ENV must mean development, but cfg.Dev is false")
	}
	if cfg.Discord.Enabled() {
		t.Error("Discord reports itself enabled with no Discord variables set; the site would try to announce events to a guild it has no token for")
	}
}

// The environment name comes from two variables because the Node app used
// NODE_ENV and docker-compose now sets ENV. Both still have to work, and ENV
// has to win — that is the one docker-compose writes.
func TestLoadEnvironmentName(t *testing.T) {
	tests := []struct {
		name    string
		nodeEnv string
		env     string
		wantEnv string
		wantDev bool
	}{
		{name: "neither set", wantEnv: "development", wantDev: true},
		{name: "NODE_ENV production", nodeEnv: "production", wantEnv: "production", wantDev: false},
		{name: "ENV production", env: "production", wantEnv: "production", wantDev: false},
		{name: "ENV overrides NODE_ENV", nodeEnv: "production", env: "development", wantEnv: "development", wantDev: true},
		{name: "ENV wins even when NODE_ENV is development", nodeEnv: "development", env: "production", wantEnv: "production", wantDev: false},
		// Anything that is not exactly "production" is development. A typo in
		// the orchestrator therefore fails open into the safer direction: TLS
		// checks relax, but the value is echoed in the startup log line.
		{name: "unrecognised value is development", env: "prod", wantEnv: "prod", wantDev: true},
		{name: "capitalised Production is not production", env: "Production", wantEnv: "Production", wantDev: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnv()
			env["NODE_ENV"] = tt.nodeEnv
			env["ENV"] = tt.env
			withEnv(t, env)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}
			if cfg.Env != tt.wantEnv {
				t.Errorf("cfg.Env = %q, want %q; the startup log would name the wrong environment", cfg.Env, tt.wantEnv)
			}
			if cfg.Dev != tt.wantDev {
				t.Errorf("cfg.Dev = %t, want %t; this decides TLS enforcement and the log format", cfg.Dev, tt.wantDev)
			}
		})
	}
}

// An empty environment is what a deployment looks like when the secrets never
// got injected. Every missing piece has to be named in one error: reporting
// them one per restart is what turns a five-minute fix into an afternoon.
func TestLoadReportsEveryProblemAtOnce(t *testing.T) {
	withEnv(t, nil)

	_, err := Load()
	wantErrors(t, err,
		"BASE_URL is required",
		"FUSION_AUTH_HOST is required",
		"FUSION_AUTH_CLIENT_ID is required",
		"FUSION_AUTH_CLIENT_SECRET is required",
		"FUSION_AUTH_SECRET is required",
		"MONGO_DB_URL is required",
	)
}

// In production a plaintext host means session cookies and OAuth codes cross
// the network in the clear, so it is a refusal to start rather than a warning.
// In development http://localhost is the only thing that works.
func TestLoadTLSRequirement(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		baseURL  string
		authHost string
		wantErrs []string
	}{
		{
			name: "http is fine in development", env: "development",
			baseURL: "http://localhost:3000", authHost: "http://localhost:9011",
		},
		{
			name: "http is rejected in production", env: "production",
			baseURL: "http://itemize.no", authHost: "http://auth.itemize.no",
			wantErrs: []string{
				`BASE_URL must use https in production (got "http")`,
				`FUSION_AUTH_HOST must use https in production (got "http")`,
			},
		},
		{
			name: "https is fine in production", env: "production",
			baseURL: "https://itemize.no", authHost: "https://auth.itemize.no",
		},
		{
			name: "only one of the two is plaintext", env: "production",
			baseURL: "https://itemize.no", authHost: "http://auth.itemize.no",
			wantErrs: []string{`FUSION_AUTH_HOST must use https in production`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnv()
			env["ENV"] = tt.env
			env["BASE_URL"] = tt.baseURL
			env["FUSION_AUTH_HOST"] = tt.authHost
			withEnv(t, env)

			_, err := Load()
			if len(tt.wantErrs) == 0 {
				if err != nil {
					t.Fatalf("Load rejected a valid %s environment: %v", tt.env, err)
				}
				return
			}
			wantErrors(t, err, tt.wantErrs...)
		})
	}
}

// The session key is stretched into an AES-256 key by hashing, which means a
// short input produces a full-length key that is nowhere near full strength.
// The length check is the only thing standing between "we set a secret" and a
// forgeable session cookie.
func TestLoadSessionSecretLength(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantErr string
	}{
		{name: "unset", secret: "", wantErr: "FUSION_AUTH_SECRET is required"},
		{name: "one byte short", secret: strings.Repeat("a", minSecretLen-1),
			wantErr: "FUSION_AUTH_SECRET must be at least 32 bytes, got 31"},
		{name: "exactly the minimum", secret: strings.Repeat("a", minSecretLen)},
		{name: "comfortably long", secret: strings.Repeat("a", 64)},
		// Length is counted in bytes, not runes: sixteen two-byte runes are
		// 32 bytes of key material and are accepted, which is the honest
		// measure of how much entropy the hash is given.
		{name: "multibyte runes count as bytes", secret: strings.Repeat("æ", 16)},
		{name: "multibyte runes below the byte minimum", secret: strings.Repeat("æ", 15),
			wantErr: "must be at least 32 bytes, got 30"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnv()
			env["FUSION_AUTH_SECRET"] = tt.secret
			withEnv(t, env)

			_, err := Load()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("a %d-byte session key was rejected: %v", len(tt.secret), err)
				}
				return
			}
			wantErrors(t, err, tt.wantErr)
		})
	}
}

// Picking the wrong signing algorithm fails every single login, and the
// FusionAuth error that results says nothing about this variable — so it is
// checked at startup instead.
func TestLoadIDTokenAlg(t *testing.T) {
	tests := []struct {
		name    string
		alg     string
		want    string
		wantErr string
	}{
		{name: "unset defaults to HS256", alg: "", want: "HS256"},
		{name: "HS256", alg: "HS256", want: "HS256"},
		{name: "RS256", alg: "RS256", want: "RS256"},
		{name: "lowercase is upper-cased", alg: "rs256", want: "RS256"},
		{name: "mixed case is upper-cased", alg: "Hs256", want: "HS256"},
		{name: "unsupported algorithm", alg: "ES256",
			wantErr: `FUSION_AUTH_ID_TOKEN_ALG must be HS256 or RS256, got "ES256"`},
		// "none" is the classic JWT downgrade attack; accepting it would mean
		// any unsigned token verifies.
		{name: "none is refused", alg: "none",
			wantErr: `FUSION_AUTH_ID_TOKEN_ALG must be HS256 or RS256, got "NONE"`},
		// Surrounding whitespace is not trimmed. That is worth pinning down:
		// the value is quoted back in the error message, which is the only
		// hint an operator gets that the problem is an invisible space.
		{name: "surrounding whitespace is not trimmed", alg: " HS256",
			wantErr: `FUSION_AUTH_ID_TOKEN_ALG must be HS256 or RS256, got " HS256"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnv()
			env["FUSION_AUTH_ID_TOKEN_ALG"] = tt.alg
			withEnv(t, env)

			cfg, err := Load()
			if tt.wantErr != "" {
				wantErrors(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("Load rejected FUSION_AUTH_ID_TOKEN_ALG=%q: %v", tt.alg, err)
			}
			if cfg.FusionAuth.IDTokenAlg != tt.want {
				t.Errorf("IDTokenAlg = %q, want %q", cfg.FusionAuth.IDTokenAlg, tt.want)
			}
		})
	}
}

// FusionAuth's own default HMAC key is not the client secret the OIDC spec
// names, and the two are indistinguishable by eye. Whichever is configured has
// to be the one used to verify, or every login fails with a signature error.
func TestIDTokenSecret(t *testing.T) {
	tests := []struct {
		name string
		fa   FusionAuth
		want string
	}{
		{
			name: "falls back to the client secret",
			fa:   FusionAuth{ClientSecret: "client"},
			want: "client",
		},
		{
			name: "a separate HMAC key wins",
			fa:   FusionAuth{ClientSecret: "client", IDTokenHMACSecret: "hmac"},
			want: "hmac",
		},
		{
			name: "an empty HMAC key is not a key",
			fa:   FusionAuth{ClientSecret: "client", IDTokenHMACSecret: ""},
			want: "client",
		},
		{
			name: "neither configured",
			fa:   FusionAuth{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fa.IDTokenSecret(); got != tt.want {
				t.Errorf("IDTokenSecret() = %q, want %q; ID token verification would use the wrong key and every login would fail", got, tt.want)
			}
		})
	}
}

// The FusionAuth API token is deliberately optional: without it the site still
// serves and members can still log in, only registration and profile writes
// degrade to 503. A contributor should not need one to run the site.
func TestLoadAPITokenIsOptional(t *testing.T) {
	withEnv(t, validEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load required an API token it documents as optional: %v", err)
	}
	if cfg.FusionAuth.APIToken != "" {
		t.Errorf("APIToken = %q with the variable unset, want empty", cfg.FusionAuth.APIToken)
	}
}

// Half-configured Discord is a mistake, not an opt-out: it starts cleanly and
// then fails at the first API call, hours later and nowhere near the variable
// that caused it.
func TestLoadDiscord(t *testing.T) {
	full := map[string]string{
		"DISCORD_CLIENT_ID":             "1234567890",
		"DISCORD_CLIENT_SECRET":         "discord-secret",
		"DISCORD_BOT_TOKEN":             "MTIzNDU2Nzg5.GaBcDe.FgHiJkLmNoPqRsTuVwXyZ",
		"DISCORD_SERVER_ID":             "9876543210",
		"DISCORD_SERVER_MEMBER_ROLE_ID": "5555555555",
	}

	t.Run("fully configured", func(t *testing.T) {
		env := validEnv()
		for k, v := range full {
			env[k] = v
		}
		withEnv(t, env)

		cfg, err := Load()
		if err != nil {
			t.Fatalf("a complete Discord configuration was rejected: %v", err)
		}
		if !cfg.Discord.Enabled() {
			t.Fatal("Discord is not enabled despite every variable being set; events would silently stop being announced")
		}
		// The bot token is normalised on the way in, so the "Bot " prefix and
		// stray whitespace never reach an Authorization header.
		if got := cfg.Discord.BotToken; got != full["DISCORD_BOT_TOKEN"] {
			t.Errorf("BotToken = %q, want %q", got, full["DISCORD_BOT_TOKEN"])
		}
	})

	t.Run("not configured at all", func(t *testing.T) {
		withEnv(t, validEnv())

		cfg, err := Load()
		if err != nil {
			t.Fatalf("running without Discord must be supported, but Load failed: %v", err)
		}
		if cfg.Discord.Enabled() {
			t.Error("Discord reports itself enabled with nothing configured")
		}
	})

	// Each subtest drops exactly one variable, so the check cannot pass by
	// accident on a single field.
	for missing := range full {
		t.Run("missing "+missing, func(t *testing.T) {
			env := validEnv()
			for k, v := range full {
				env[k] = v
			}
			env[missing] = ""
			withEnv(t, env)

			_, err := Load()
			wantErrors(t, err, "Discord is partially configured")
		})
	}

	// Whitespace-only values are what a copy-paste into a secret store leaves
	// behind. They have to count as unset, or the site refuses to start over a
	// variable that looks empty in every tool an operator has.
	t.Run("whitespace-only values count as unset", func(t *testing.T) {
		env := validEnv()
		for k := range full {
			env[k] = "   "
		}
		withEnv(t, env)

		cfg, err := Load()
		if err != nil {
			t.Fatalf("whitespace-only Discord variables were treated as a partial configuration: %v", err)
		}
		if cfg.Discord != (Discord{}) {
			t.Errorf("Discord = %+v after trimming, want the zero value", cfg.Discord)
		}
	})

	// A bot token pasted with the prefix from Discord's documentation is the
	// most common cause of "401 Unauthorized", and after normalisation it must
	// still count as configured rather than collapsing to empty.
	t.Run("a Bot-prefixed token still counts as configured", func(t *testing.T) {
		env := validEnv()
		for k, v := range full {
			env[k] = v
		}
		env["DISCORD_BOT_TOKEN"] = "Bot " + full["DISCORD_BOT_TOKEN"] + "\n"
		withEnv(t, env)

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if !cfg.Discord.Enabled() {
			t.Fatal("a Bot-prefixed token left Discord disabled")
		}
		if got := cfg.Discord.BotToken; got != full["DISCORD_BOT_TOKEN"] {
			t.Errorf("BotToken = %q, want the prefix and newline stripped (%q)", got, full["DISCORD_BOT_TOKEN"])
		}
	})
}

func TestDiscordEnabled(t *testing.T) {
	complete := Discord{
		ClientID:     "id",
		ClientSecret: "secret",
		BotToken:     "token",
		GuildID:      "guild",
		MemberRoleID: "role",
	}

	if !complete.Enabled() {
		t.Fatal("a fully populated Discord config reports itself disabled")
	}
	if (Discord{}).Enabled() {
		t.Error("the zero Discord config reports itself enabled")
	}

	// Blanking one field at a time proves Enabled is an AND over all five and
	// not, say, a check of the bot token alone.
	blank := []struct {
		name string
		blot func(*Discord)
	}{
		{"ClientID", func(d *Discord) { d.ClientID = "" }},
		{"ClientSecret", func(d *Discord) { d.ClientSecret = "" }},
		{"BotToken", func(d *Discord) { d.BotToken = "" }},
		{"GuildID", func(d *Discord) { d.GuildID = "" }},
		{"MemberRoleID", func(d *Discord) { d.MemberRoleID = "" }},
	}
	for _, b := range blank {
		t.Run("without "+b.name, func(t *testing.T) {
			d := complete
			b.blot(&d)
			if d.Enabled() {
				t.Errorf("Discord reports itself enabled without %s; the first API call would fail instead of startup", b.name)
			}
		})
	}
}

// A trailing slash on BASE_URL is invisible in a browser and fatal for OAuth:
// redirect URIs are built by concatenation, so it produces "https://itemize.no//callback",
// which will not match what FusionAuth has registered.
func TestParseHost(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		requireTLS bool
		want       string
		wantErr    string
	}{
		{name: "plain https host", raw: "https://itemize.no", want: "https://itemize.no"},
		{name: "trailing slash is stripped", raw: "https://itemize.no/", want: "https://itemize.no"},
		{name: "several trailing slashes are stripped", raw: "https://itemize.no///", want: "https://itemize.no"},
		{name: "a path is preserved", raw: "https://itemize.no/app/", want: "https://itemize.no/app"},
		{name: "a port is preserved", raw: "http://localhost:3000", want: "http://localhost:3000"},
		{name: "query and fragment survive", raw: "https://itemize.no/a?b=c", want: "https://itemize.no/a?b=c"},

		{name: "empty", raw: "", wantErr: "EXAMPLE is required"},
		// A bare slash trims to the empty string, which parses cleanly and
		// would otherwise sail through as a valid URL with no host.
		{name: "just a slash", raw: "/", wantErr: "EXAMPLE must be absolute"},
		{name: "relative path", raw: "/callback", wantErr: "EXAMPLE must be absolute"},
		{name: "host without a scheme", raw: "itemize.no", wantErr: "EXAMPLE must be absolute"},
		{name: "scheme-relative", raw: "//itemize.no", wantErr: "EXAMPLE must be absolute"},
		{name: "scheme with no host", raw: "https://", wantErr: "EXAMPLE must be absolute"},
		{name: "a space in the host is a parse error", raw: "https://itemize .no", wantErr: "EXAMPLE is not a valid URL"},

		{name: "https satisfies the TLS requirement", raw: "https://itemize.no", requireTLS: true, want: "https://itemize.no"},
		{name: "http fails the TLS requirement", raw: "http://itemize.no", requireTLS: true,
			wantErr: `EXAMPLE must use https in production (got "http")`},
		// The scheme comparison against "https" is exact, but url.Parse
		// lower-cases the scheme first, so an upper-cased URL copied out of a
		// document still satisfies the production check.
		{name: "uppercase HTTPS is normalised", raw: "HTTPS://itemize.no", requireTLS: true, want: "https://itemize.no"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHost("EXAMPLE", tt.raw, tt.requireTLS)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("parseHost(%q) was accepted; want an error mentioning %q", tt.raw, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseHost(%q) error = %v, want it to mention %q", tt.raw, err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("parseHost returned a URL alongside its error; the caller stores it and would dereference a half-parsed host")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseHost(%q) failed: %v", tt.raw, err)
			}
			if got.String() != tt.want {
				t.Errorf("parseHost(%q) = %q, want %q", tt.raw, got.String(), tt.want)
			}
		})
	}
}

// The database name is the path of the connection string, the way the Node app
// relied on mongodb://host/website. A connection string without one connects
// happily and then every query lands in a database called "".
func TestParseMongo(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		dbName  string // MONGO_DB_NAME
		wantDB  string
		wantErr string
	}{
		{name: "database from the path", raw: "mongodb://localhost:27017/website", wantDB: "website"},
		{name: "trailing slash on the database", raw: "mongodb://localhost:27017/website/", wantDB: "website"},
		{name: "query parameters are ignored", raw: "mongodb://localhost:27017/website?retryWrites=true", wantDB: "website"},
		{name: "credentials in the URI", raw: "mongodb://user:pass@localhost:27017/website", wantDB: "website"},
		{name: "replica set with several hosts", raw: "mongodb://a:27017,b:27017/website", wantDB: "website"},
		{name: "mongodb+srv", raw: "mongodb+srv://cluster.example.net/website", wantDB: "website"},

		{name: "MONGO_DB_NAME overrides the path", raw: "mongodb://localhost:27017/website", dbName: "itemize", wantDB: "itemize"},
		{name: "MONGO_DB_NAME supplies a missing path", raw: "mongodb://localhost:27017", dbName: "itemize", wantDB: "itemize"},
		{name: "MONGO_DB_NAME rescues a bare slash", raw: "mongodb://localhost:27017/", dbName: "itemize", wantDB: "itemize"},

		{name: "empty", raw: "", wantErr: "MONGO_DB_URL is required"},
		{name: "no database in the path", raw: "mongodb://localhost:27017", wantErr: "MONGO_DB_URL must name a database"},
		{name: "bare slash is no database", raw: "mongodb://localhost:27017/", wantErr: "MONGO_DB_URL must name a database"},
		{name: "unparseable URL", raw: "mongodb://local host:27017/website", wantErr: "MONGO_DB_URL is not a valid URL"},
		// A stray extra segment used to become the database name verbatim.
		// MongoDB rejects "a/b" at the first query, which is a long way from
		// the connection string that caused it.
		{name: "a multi-segment path is refused", raw: "mongodb://localhost:27017/a/b", wantErr: "MONGO_DB_URL must name a single database"},
		// Refused on the strength of the URL alone: the driver is handed the
		// connection string verbatim, so an override cannot rescue it.
		{name: "a multi-segment path is refused even with MONGO_DB_NAME set", raw: "mongodb://localhost:27017/a/b", dbName: "itemize", wantErr: "MONGO_DB_URL must name a single database"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withEnv(t, map[string]string{"MONGO_DB_NAME": tt.dbName})

			got, err := parseMongo(tt.raw)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("parseMongo(%q) was accepted; want an error mentioning %q", tt.raw, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseMongo(%q) error = %v, want it to mention %q", tt.raw, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMongo(%q) failed: %v", tt.raw, err)
			}
			if got.Database != tt.wantDB {
				t.Errorf("parseMongo(%q).Database = %q, want %q", tt.raw, got.Database, tt.wantDB)
			}
			// The URI is handed to the driver verbatim; rewriting it would
			// drop the options a deployment depends on.
			if got.URI != tt.raw {
				t.Errorf("parseMongo(%q).URI = %q; the connection string must reach the driver unchanged", tt.raw, got.URI)
			}
		})
	}
}

// The listen address has two spellings for historical reasons — the Express
// server read PORT, template.env and docker-compose document LISTEN — and both
// have to keep working, with PORT winning to match what is deployed today.
//
// A port that cannot be bound is refused here rather than passed on to
// ListenAndServe, so the deployment stops at startup next to the variable that
// caused it. The error also has to name the variable the operator actually
// set: being told about PORT when LISTEN is what is in the compose file sends
// them looking in the wrong place.
func TestResolveAddr(t *testing.T) {
	tests := []struct {
		name    string
		port    string
		listen  string
		want    string
		wantErr string
	}{
		{name: "neither set falls back to 3000", want: ":3000"},
		{name: "bare PORT", port: "8080", want: ":8080"},
		{name: "bare LISTEN", listen: "8080", want: ":8080"},
		{name: "PORT wins over LISTEN", port: "8080", listen: "9090", want: ":8080"},
		{name: "empty PORT falls through to LISTEN", port: "", listen: "9090", want: ":9090"},
		{name: "PORT already has a colon", port: ":8080", want: ":8080"},
		{name: "a full host:port binds one interface", port: "127.0.0.1:8080", want: "127.0.0.1:8080"},
		{name: "a wildcard host:port", port: "0.0.0.0:8080", want: "0.0.0.0:8080"},
		{name: "IPv6 host:port", port: "[::1]:8080", want: "[::1]:8080"},
		{name: "the lowest port", port: "1", want: ":1"},
		{name: "the highest port", port: "65535", want: ":65535"},

		// Port 0 asks the kernel for an ephemeral port. The server would come
		// up on an unpredictable one, and the container health check — which
		// probes the configured address — could never find it, so the container
		// would stay unhealthy while serving. There is no deployment in which
		// that is what was wanted, so it is refused outright rather than
		// allowed in development.
		{name: "port zero is refused", port: "0", wantErr: "PORT must name a port between 1 and 65535"},
		{name: "an out-of-range port is refused", port: "99999", wantErr: "PORT must name a port between 1 and 65535"},
		{name: "a negative port is refused", port: "-1", wantErr: "PORT must name a port between 1 and 65535"},
		{name: "an out-of-range port in LISTEN names LISTEN", listen: "99999", wantErr: "LISTEN must name a port between 1 and 65535"},
		{name: "a port out of range inside a host:port", port: "127.0.0.1:99999", wantErr: "PORT must name a port between 1 and 65535"},

		{name: "a non-numeric value is refused", port: "http", wantErr: "PORT must be a port, :port or host:port"},
		{name: "surrounding whitespace is refused", port: " 3000", wantErr: "PORT must be a port, :port or host:port"},
		{name: "a host with no port is refused", listen: "127.0.0.1", wantErr: "LISTEN must be a port, :port or host:port"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withEnv(t, map[string]string{"PORT": tt.port, "LISTEN": tt.listen})

			got, err := ResolveAddr()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ResolveAddr() accepted PORT=%q LISTEN=%q and returned %q; the server would fail at ListenAndServe instead of at startup",
						tt.port, tt.listen, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ResolveAddr() error = %v with PORT=%q LISTEN=%q, want it to mention %q so the operator knows which variable to fix",
						err, tt.port, tt.listen, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveAddr() rejected PORT=%q LISTEN=%q (%v); a bindable address must not stop the server",
					tt.port, tt.listen, err)
			}
			if got != tt.want {
				t.Errorf("ResolveAddr() = %q with PORT=%q LISTEN=%q, want %q; the server would bind the wrong address",
					got, tt.port, tt.listen, tt.want)
			}
		})
	}
}

// An unusable port has to come out of Load together with everything else that
// is wrong, not from a crash inside ListenAndServe after the rest of the
// startup has already succeeded.
func TestLoadRejectsAnUnusablePort(t *testing.T) {
	env := validEnv()
	env["PORT"] = "99999"
	env["BASE_URL"] = ""
	withEnv(t, env)

	_, err := Load()
	wantErrors(t, err,
		"PORT must name a port between 1 and 65535",
		// Still joined with the other findings: one restart, every problem.
		"BASE_URL is required")
}

// resolveAddr is only reached through Load in production, so the fallback
// chain is worth confirming end to end as well.
func TestLoadUsesListenAddress(t *testing.T) {
	env := validEnv()
	env["LISTEN"] = "8080"
	withEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("cfg.Addr = %q, want %q; LISTEN is what docker-compose sets", cfg.Addr, ":8080")
	}
}

func TestEnvOr(t *testing.T) {
	withEnv(t, map[string]string{"BASE_URL": "set"})

	if got := envOr("BASE_URL", "fallback"); got != "set" {
		t.Errorf("envOr returned %q for a variable that is set, want %q", got, "set")
	}
	// An exported-but-empty variable is indistinguishable from an unset one
	// here, which is what lets a docker-compose default of "" mean "use ours".
	if got := envOr("MONGO_DB_NAME", "fallback"); got != "fallback" {
		t.Errorf("envOr returned %q for an empty variable, want the fallback %q", got, "fallback")
	}
}

// The startup log line is the fastest way to diagnose a deployment that came up
// wrong, and it is also the easiest place to leak a secret into a log
// aggregator that keeps it forever.
func TestLogValueRedactsSecrets(t *testing.T) {
	env := validEnv()
	secrets := map[string]string{
		"FUSION_AUTH_CLIENT_SECRET":        "svaert-hemmelig-klientnokkel",
		"FUSION_AUTH_SECRET":               "svaert-hemmelig-sesjonsnokkel-0123",
		"FUSION_AUTH_API_TOKEN":            "svaert-hemmelig-api-token",
		"FUSION_AUTH_ID_TOKEN_HMAC_SECRET": "svaert-hemmelig-hmac-nokkel",
		"DISCORD_CLIENT_ID":                "1234567890",
		"DISCORD_CLIENT_SECRET":            "svaert-hemmelig-discord-secret",
		"DISCORD_BOT_TOKEN":                "svaert.hemmelig.bottoken",
		"DISCORD_SERVER_ID":                "9876543210",
		"DISCORD_SERVER_MEMBER_ROLE_ID":    "5555555555",
	}
	for k, v := range secrets {
		env[k] = v
	}
	withEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	log.Info("starting", "config", cfg)
	line := buf.String()

	// LogValue must be picked up by slog through the slog.LogValuer interface.
	// A signature returning `any` would compile, quietly fail to satisfy the
	// interface, and print the whole struct — secrets included.
	if !strings.Contains(line, "config.env=development") {
		t.Fatalf("slog did not use Config.LogValue; the whole struct was printed instead.\nline: %s", line)
	}

	for name, secret := range secrets {
		switch name {
		case "DISCORD_CLIENT_ID", "DISCORD_SERVER_ID", "DISCORD_SERVER_MEMBER_ROLE_ID":
			// Identifiers, not credentials — and they are not logged anyway.
			continue
		}
		if strings.Contains(line, secret) {
			t.Errorf("%s appears verbatim in the startup log line; it would be retained by every log sink the server writes to.\nline: %s", name, line)
		}
	}

	// The non-secret fields are the entire point of the line: without them
	// there is nothing to diagnose from.
	for _, want := range []string{
		"config.addr=:3000",
		"config.base_url=https://itemize.no",
		"config.fusionauth_host=https://auth.itemize.no",
		"config.id_token_alg=HS256",
		"config.mongo_database=website",
		"config.discord_enabled=true",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("the startup log line is missing %q, which is what an operator reads to spot a wrong environment.\nline: %s", want, line)
		}
	}
}

// The startup line is logged before anything else can fail, so it has to
// survive a Config whose URLs were never parsed — a nil *url.URL must print as
// "<unset>" rather than panicking and taking the process down.
func TestLogValueOnZeroConfig(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))

	cfg := &Config{}
	log.Info("starting", "config", cfg)

	line := buf.String()
	for _, want := range []string{
		"config.base_url=<unset>",
		"config.fusionauth_host=<unset>",
		"config.session_secret=<unset>",
		"config.fusionauth_api_token=<unset>",
		"config.discord_enabled=false",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("logging a zero Config is missing %q.\nline: %s", want, line)
		}
	}
}

func TestRedact(t *testing.T) {
	// "<unset>" and "****" have to be different, or a log reader cannot tell a
	// secret that is missing from one that is merely hidden — which is exactly
	// the question being asked when a deployment will not authenticate.
	if got := redact(""); got != "<unset>" {
		t.Errorf("redact(\"\") = %q, want %q", got, "<unset>")
	}
	if got := redact("hemmelig"); got != "****" {
		t.Errorf("redact returned %q, want the value masked as %q", got, "****")
	}
}

// Outside production Load seeds from ./.env, so a contributor can clone the
// repository, fill in one file and run the server.
func TestLoadReadsDotenvInDevelopment(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), strings.Join([]string{
		"BASE_URL=http://localhost:3000",
		"FUSION_AUTH_HOST=http://localhost:9011",
		"FUSION_AUTH_CLIENT_ID=fra-dotenv",
		"FUSION_AUTH_CLIENT_SECRET=client-secret",
		"FUSION_AUTH_SECRET=" + testSecret,
		"MONGO_DB_URL=mongodb://localhost:27017/website",
	}, "\n"))

	withEnv(t, nil)
	// withEnv leaves every variable exported-but-empty, which counts as set and
	// would make loadDotenv skip all of them. Genuinely removing them is the
	// only state in which the file is consulted.
	unsetenv(t, configEnv...)
	t.Chdir(dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("a complete .env did not produce a usable configuration: %v", err)
	}
	if cfg.FusionAuth.ClientID != "fra-dotenv" {
		t.Errorf("FusionAuth.ClientID = %q, want it to come from .env", cfg.FusionAuth.ClientID)
	}
}

// In production the environment is injected by the orchestrator, and a .env
// that survived into the image — a stale build context, a bad COPY — must not
// win over it or a rollback becomes impossible to reason about.
func TestLoadIgnoresDotenvInProduction(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"),
		"FUSION_AUTH_CLIENT_ID=fra-dotenv\nBASE_URL=http://stale.example\n")

	env := validEnv()
	env["ENV"] = "production"
	env["FUSION_AUTH_CLIENT_ID"] = "fra-orkestratoren"
	withEnv(t, env)
	t.Chdir(dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.FusionAuth.ClientID != "fra-orkestratoren" {
		t.Errorf("FusionAuth.ClientID = %q; a .env file overrode the injected production environment", cfg.FusionAuth.ClientID)
	}
}

// The environment name is resolved before .env is read, so ENV or NODE_ENV set
// in the file cannot change the decision that led to the file being read. It is
// a genuine chicken-and-egg, and pinning it down keeps anyone from assuming
// `ENV=production` in .env does anything.
func TestLoadIgnoresEnvironmentNameFromDotenv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "ENV=production\nNODE_ENV=production\n")

	withEnv(t, validEnv())
	unsetenv(t, "ENV", "NODE_ENV")
	t.Chdir(dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Dev != true || cfg.Env != "development" {
		t.Errorf("cfg.Env = %q, Dev = %t; ENV in .env was honoured, which it cannot be — the file is only read once development has already been decided",
			cfg.Env, cfg.Dev)
	}
	// The file was still read: its other variables would have been applied.
	if _, ok := os.LookupEnv("ENV"); !ok {
		t.Error("ENV was not set from .env at all, so this test is not exercising what it claims")
	}
}

// A .env that cannot be parsed is a hard failure rather than a silent partial
// load: starting with half a configuration is worse than not starting.
func TestLoadSurfacesDotenvErrors(t *testing.T) {
	dir := t.TempDir()
	// A single line longer than bufio.Scanner's 64 KiB buffer. Nothing after
	// it would be read, so the load has to fail rather than continue.
	writeFile(t, filepath.Join(dir, ".env"), "A_SECRET="+strings.Repeat("x", 70_000)+"\n")

	withEnv(t, validEnv())
	t.Chdir(dir)

	_, err := Load()
	wantErrors(t, err, "reading .env")
}
