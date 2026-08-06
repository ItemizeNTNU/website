// Package config loads and validates the server's environment.
//
// Every problem is reported at once from [Load] rather than one per restart,
// and anything that would make the server misbehave silently — a trailing
// slash on BASE_URL, a short session key, a plaintext host in production — is
// a startup failure rather than a runtime surprise.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config is the fully resolved, validated runtime configuration.
type Config struct {
	Env     string   // "development" or "production"
	Dev     bool     // Env != "production"
	Addr    string   // ":3000"
	BaseURL *url.URL // no trailing slash

	FusionAuth FusionAuth
	Mongo      Mongo
	Discord    Discord
}

// FusionAuth holds the identity provider settings. Login is impossible
// without these, so they are all required.
type FusionAuth struct {
	Host         *url.URL // no trailing slash
	ClientID     string
	ClientSecret string
	// Secret is key material for sealing the session cookie, not an OAuth
	// credential. Named for the environment variable it comes from.
	Secret string
	// APIToken authenticates server-to-server calls. Optional: when empty,
	// registration and profile writes degrade to 503 but login still works.
	APIToken string

	// IDTokenAlg is the algorithm FusionAuth signs ID tokens with: "HS256" or
	// "RS256".
	//
	// TODO(auth): default this to RS256 and delete the HS256 path once the
	// FusionAuth application has been switched over. HS256 means the shared
	// secret can mint tokens as well as verify them; RS256 means only
	// FusionAuth can issue them. The change is a per-application setting, so
	// it can be made without affecting the wiki or anything else on the same
	// tenant. See docs/auth.md.
	IDTokenAlg string

	// IDTokenHMACSecret is the shared secret for HS256, when it is not the
	// client secret. FusionAuth's own default is a separately generated HMAC
	// key rather than the client secret the OIDC specification names, and the
	// two are different values — using the wrong one fails every login.
	IDTokenHMACSecret string
}

// IDTokenSecret returns the key that verifies an HS256 ID token, defaulting to
// the client secret as the specification prescribes.
func (f FusionAuth) IDTokenSecret() string {
	if f.IDTokenHMACSecret != "" {
		return f.IDTokenHMACSecret
	}
	return f.ClientSecret
}

// Mongo holds the database connection settings.
type Mongo struct {
	URI      string
	Database string
}

// Discord holds the bot and OAuth settings. Every field is optional; see
// [Discord.Enabled].
type Discord struct {
	ClientID     string
	ClientSecret string
	BotToken     string
	GuildID      string
	MemberRoleID string
}

// Enabled reports whether Discord integration is fully configured. When it is
// not, event sync is skipped and the account-linking routes return 503 — which
// lets a contributor run the site locally without a bot token.
func (d Discord) Enabled() bool {
	return d.ClientID != "" && d.ClientSecret != "" && d.BotToken != "" &&
		d.GuildID != "" && d.MemberRoleID != ""
}

// minSecretLen is the shortest session key we accept. The key is stretched to
// an AES-256 key by hashing, so a short input would be silently weak.
const minSecretLen = 32

// Load reads the environment — optionally seeded from a .env file — and
// returns a validated Config. All validation failures are joined into the
// returned error so a misconfigured deployment surfaces every problem at once.
func Load() (*Config, error) {
	env := envOr("NODE_ENV", "development")
	if e := os.Getenv("ENV"); e != "" {
		env = e
	}
	dev := env != "production"

	// Only seed from .env outside production. In production docker-compose
	// injects via env_file, and a stray .env in the image should not win.
	if dev {
		if err := loadDotenv(".env"); err != nil {
			return nil, fmt.Errorf("reading .env: %w", err)
		}
	}

	cfg := &Config{
		Env:  env,
		Dev:  dev,
		Addr: resolveAddr(),
		FusionAuth: FusionAuth{
			ClientID:     os.Getenv("FUSION_AUTH_CLIENT_ID"),
			ClientSecret: os.Getenv("FUSION_AUTH_CLIENT_SECRET"),
			Secret:       os.Getenv("FUSION_AUTH_SECRET"),
			APIToken:     os.Getenv("FUSION_AUTH_API_TOKEN"),
			// HS256 is today's reality, not a preference. See the TODO above.
			IDTokenAlg:        strings.ToUpper(envOr("FUSION_AUTH_ID_TOKEN_ALG", "HS256")),
			IDTokenHMACSecret: os.Getenv("FUSION_AUTH_ID_TOKEN_HMAC_SECRET"),
		},
		Discord: Discord{
			ClientID:     strings.TrimSpace(os.Getenv("DISCORD_CLIENT_ID")),
			ClientSecret: strings.TrimSpace(os.Getenv("DISCORD_CLIENT_SECRET")),
			BotToken:     botToken(os.Getenv("DISCORD_BOT_TOKEN")),
			GuildID:      strings.TrimSpace(os.Getenv("DISCORD_SERVER_ID")),
			MemberRoleID: strings.TrimSpace(os.Getenv("DISCORD_SERVER_MEMBER_ROLE_ID")),
		},
	}

	var errs []error

	base, err := parseHost("BASE_URL", os.Getenv("BASE_URL"), !dev)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.BaseURL = base

	host, err := parseHost("FUSION_AUTH_HOST", os.Getenv("FUSION_AUTH_HOST"), !dev)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.FusionAuth.Host = host

	for name, v := range map[string]string{
		"FUSION_AUTH_CLIENT_ID":     cfg.FusionAuth.ClientID,
		"FUSION_AUTH_CLIENT_SECRET": cfg.FusionAuth.ClientSecret,
	} {
		if v == "" {
			errs = append(errs, fmt.Errorf("%s is required", name))
		}
	}

	switch n := len(cfg.FusionAuth.Secret); {
	case n == 0:
		errs = append(errs, errors.New("FUSION_AUTH_SECRET is required"))
	case n < minSecretLen:
		errs = append(errs, fmt.Errorf(
			"FUSION_AUTH_SECRET must be at least %d bytes, got %d", minSecretLen, n))
	}

	switch cfg.FusionAuth.IDTokenAlg {
	case "HS256", "RS256":
	default:
		errs = append(errs, fmt.Errorf(
			"FUSION_AUTH_ID_TOKEN_ALG must be HS256 or RS256, got %q", cfg.FusionAuth.IDTokenAlg))
	}

	mongo, err := parseMongo(os.Getenv("MONGO_DB_URL"))
	if err != nil {
		errs = append(errs, err)
	}
	cfg.Mongo = mongo

	// Partially configured Discord is a mistake, not a deliberate opt-out:
	// it fails at the first API call rather than at startup.
	if !cfg.Discord.Enabled() && cfg.Discord != (Discord{}) {
		errs = append(errs, errors.New(
			"Discord is partially configured: set all of DISCORD_CLIENT_ID, "+
				"DISCORD_CLIENT_SECRET, DISCORD_BOT_TOKEN, DISCORD_SERVER_ID and "+
				"DISCORD_SERVER_MEMBER_ROLE_ID, or none of them"))
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return cfg, nil
}

// botToken normalises the Discord bot token.
//
// Two mistakes account for most "401 Unauthorized" reports, and neither is
// visible by looking at the value:
//
//   - The token is copied together with the "Bot " prefix that appears in
//     Discord's documentation and in example headers. We add that prefix
//     ourselves, so the header becomes "Bot Bot xxx" and every call fails.
//   - A trailing newline survives being pasted into a .env file or a secret
//     store, and goes out as part of the header value.
//
// Both are silently corrected here rather than left to fail at the first API
// call, which happens well away from the configuration that caused it.
func botToken(raw string) string {
	token := strings.TrimSpace(raw)
	if after, found := strings.CutPrefix(token, "Bot "); found {
		token = strings.TrimSpace(after)
	}
	return token
}

// resolveAddr picks the listen address. The old Express server read PORT
// while template.env and docker-compose documented LISTEN, so both are
// accepted; PORT wins to match the deployed behaviour.
func resolveAddr() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("LISTEN")
	}
	if port == "" {
		port = "3000"
	}
	// Accept a bare port, a :port, or a full host:port.
	if _, err := strconv.Atoi(port); err == nil {
		return ":" + port
	}
	return port
}

// parseHost validates an absolute URL used as a base for concatenation, and
// strips any trailing slash. A trailing slash here produces double-slash
// OAuth redirect URIs that will not match what the provider has registered.
func parseHost(name, raw string, requireTLS bool) (*url.URL, error) {
	if raw == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil {
		return nil, fmt.Errorf("%s is not a valid URL: %w", name, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("%s must be absolute, e.g. https://itemize.no (got %q)", name, raw)
	}
	if requireTLS && u.Scheme != "https" {
		return nil, fmt.Errorf("%s must use https in production (got %q)", name, u.Scheme)
	}
	return u, nil
}

// parseMongo splits the connection string into a URI and a database name.
// The database is the URI path, matching how the Node app relied on
// mongodb://host/website.
func parseMongo(raw string) (Mongo, error) {
	if raw == "" {
		return Mongo{}, errors.New("MONGO_DB_URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Mongo{}, fmt.Errorf("MONGO_DB_URL is not a valid URL: %w", err)
	}
	db := strings.Trim(u.Path, "/")
	if name := os.Getenv("MONGO_DB_NAME"); name != "" {
		db = name
	}
	if db == "" {
		return Mongo{}, errors.New(
			"MONGO_DB_URL must name a database, e.g. mongodb://localhost:27017/website " +
				"(or set MONGO_DB_NAME)")
	}
	return Mongo{URI: raw, Database: db}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// LogValue renders the configuration for the startup log with every secret
// replaced. This one line is the fastest way to diagnose a deployment that
// came up with the wrong environment.
//
// The signature has to be exactly this for slog to use it — returning `any`
// would compile, quietly fail to satisfy slog.LogValuer, and print the whole
// struct including the secrets.
func (c *Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("env", c.Env),
		slog.String("addr", c.Addr),
		slog.String("base_url", urlString(c.BaseURL)),
		slog.String("fusionauth_host", urlString(c.FusionAuth.Host)),
		slog.String("fusionauth_client_id", c.FusionAuth.ClientID),
		slog.String("fusionauth_client_secret", redact(c.FusionAuth.ClientSecret)),
		slog.String("session_secret", redact(c.FusionAuth.Secret)),
		slog.String("fusionauth_api_token", redact(c.FusionAuth.APIToken)),
		slog.String("id_token_alg", c.FusionAuth.IDTokenAlg),
		slog.String("mongo_database", c.Mongo.Database),
		slog.Bool("discord_enabled", c.Discord.Enabled()),
		slog.String("discord_client_secret", redact(c.Discord.ClientSecret)),
		slog.String("discord_bot_token", redact(c.Discord.BotToken)),
	)
}

func redact(s string) string {
	if s == "" {
		return "<unset>"
	}
	return "****"
}

func urlString(u *url.URL) string {
	if u == nil {
		return "<unset>"
	}
	return u.String()
}
