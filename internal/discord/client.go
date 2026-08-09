// Package discord talks to the Discord API.
//
// Version 10. The previous server used v8, which is deprecated — v6 and v7 are
// already decommissioned and v8 is on the same path.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/ItemizeNTNU/website/internal/config"
)

// APIBase is the versioned API root.
const APIBase = "https://discord.com/api/v10"

// userAgent identifies this bot. Discord requires a descriptive one and rate
// limits the default Go client string more aggressively.
const userAgent = "DiscordBot (https://itemize.no, 2.0)"

// maxBody caps how much of a response we will read, so a misbehaving endpoint
// cannot exhaust memory.
const maxBody = 1 << 20

// maxRetryWait caps how long a 429 may hold the caller before we give up.
//
// Discord's per-route buckets reset within a second or two, so five seconds
// covers every retry actually worth making. A global limit is different: it is
// handed out in tens of seconds or minutes, and sleeping one out parks whatever
// goroutine is here. For event sync that is a detached worker with its own
// deadline, but the account-linking handlers pass the request context, which
// carries no deadline at all — WriteTimeout does not cancel it. Past the cap we
// fail visibly rather than hold a request open for a window we did not choose.
const maxRetryWait = 5 * time.Second

// snowflakePattern matches a Discord identifier: decimal digits, nothing else.
//
// These identifiers are concatenated into request paths, and anything carrying
// a slash, a dot segment or a question mark would reach a different endpoint
// than intended. Checking the shape is cheaper and clearer than escaping every
// call site, and a Discord id that is not all digits is malformed anyway.
var snowflakePattern = regexp.MustCompile(`^[0-9]{1,25}$`)

// ErrInvalidID is returned for an identifier that is not a Discord snowflake.
var ErrInvalidID = errors.New("not a valid Discord id")

// ValidID reports whether id is a well-formed Discord snowflake.
func ValidID(id string) bool { return snowflakePattern.MatchString(id) }

// Client is a Discord API client.
type Client struct {
	cfg  config.Discord
	http *http.Client
	log  *slog.Logger
}

// New builds a client. It returns nil when Discord is not configured, which
// callers treat as "skip the integration" rather than an error — that is what
// lets the site run locally without a bot token.
func New(cfg config.Discord, log *slog.Logger) *Client {
	if !cfg.Enabled() {
		return nil
	}
	return &Client{
		cfg: cfg,
		// Never http.DefaultClient: it has no timeout, so a hung Discord would
		// pin goroutines until the process restarted.
		http: &http.Client{Timeout: 10 * time.Second},
		log:  log,
	}
}

// Enabled reports whether the integration is available. Safe on a nil client.
func (c *Client) Enabled() bool { return c != nil }

// credential names the secret a request authenticated with.
//
// A 401 means something different for each of them, and the status alone
// cannot tell them apart. The zero value is the bot token because that is what
// every call except the two OAuth legs carries.
type credential int

const (
	credentialBotToken     credential = iota // Authorization: Bot …
	credentialClientSecret                   // client_secret in the token form
	credentialAccessToken                    // Authorization: Bearer … (the member's)
)

// APIError is a non-2xx response from Discord.
type APIError struct {
	Status  int
	Code    int    `json:"code"`
	Message string `json:"message"`

	// credential is which of our secrets the failing request carried. It is
	// set where the request is built rather than inferred from the status,
	// because the status cannot know.
	credential credential
}

// newAPIError decodes a non-2xx response to a request that carried cred.
func newAPIError(status int, payload []byte, cred credential) *APIError {
	e := &APIError{Status: status, credential: cred}
	_ = json.Unmarshal(payload, e)
	return e
}

func (e *APIError) Error() string {
	// A 401 says only that something was rejected, and the bare status sends
	// nobody anywhere. Which secret to go and check depends entirely on which
	// one the request carried, and the OAuth legs never send the bot token —
	// blaming DISCORD_BOT_TOKEN there sends an operator after a problem that
	// does not exist, usually for a callback someone simply reloaded.
	if e.Status == http.StatusUnauthorized {
		switch e.credential {
		case credentialClientSecret:
			return "discord: the OAuth token exchange was rejected (HTTP 401) — the " +
				"bot token is not used on this call; either DISCORD_CLIENT_SECRET no " +
				"longer matches the application, or the authorization code had already " +
				"been redeemed or expired, which is what a reloaded callback looks like"
		case credentialAccessToken:
			return "discord: the account lookup was rejected (HTTP 401) — this call " +
				"uses the member's access token from the exchange, not " +
				"DISCORD_BOT_TOKEN; the token was expired or revoked, and the member " +
				"has to start the linking flow again"
		default:
			return "discord: the bot token was rejected (HTTP 401) — check " +
				"DISCORD_BOT_TOKEN; it must be the bot token from Bot → Reset Token, " +
				"not the client secret, and it is invalidated whenever it is reset"
		}
	}
	if e.Message != "" {
		return fmt.Sprintf("discord: %s (HTTP %d, code %d)", e.Message, e.Status, e.Code)
	}
	return fmt.Sprintf("discord: HTTP %d", e.Status)
}

// do performs a request, retrying once when rate limited.
//
// The previous implementation ignored 429 entirely. Event sync fires on every
// save, so hitting the limit is a live possibility rather than a theoretical
// one — and an ignored 429 surfaces as an event that silently failed to
// publish.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	const attempts = 2

	for attempt := 1; ; attempt++ {
		var reader io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				return err
			}
			reader = bytes.NewReader(encoded)
		}

		req, err := http.NewRequestWithContext(ctx, method, APIBase+path, reader)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bot "+c.cfg.BotToken)
		req.Header.Set("User-Agent", userAgent)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("discord: %s %s: %w", method, path, err)
		}

		payload, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		resp.Body.Close()
		if readErr != nil {
			return readErr
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < attempts {
			// A caller that has already gone away gets a cancellation whatever
			// we would have done next; there is nobody left to retry for.
			if err := ctx.Err(); err != nil {
				return err
			}
			wait := retryAfter(resp)
			if wait > maxRetryWait {
				// A global limit, not a per-route bucket. Waiting it out would
				// hold this goroutine — an HTTP request handler, on the linking
				// flow — for a window Discord chose and we cannot shorten.
				c.log.Warn("rate limited by Discord for longer than we will wait",
					"path", path, "wait", wait, "cap", maxRetryWait)
				return fmt.Errorf("discord: rate limited for %s, longer than the %s "+
					"this client will wait; giving up rather than holding the caller: %w",
					wait, maxRetryWait, newAPIError(resp.StatusCode, payload, credentialBotToken))
			}
			c.log.Warn("rate limited by Discord, retrying", "path", path, "wait", wait)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return newAPIError(resp.StatusCode, payload, credentialBotToken)
		}

		if out != nil && len(payload) > 0 {
			return json.Unmarshal(payload, out)
		}
		return nil
	}
}

func retryAfter(resp *http.Response) time.Duration {
	for _, header := range []string{"Retry-After", "X-RateLimit-Reset-After"} {
		v := resp.Header.Get(header)
		if v == "" {
			continue
		}
		if seconds, err := strconv.ParseFloat(v, 64); err == nil {
			return max(time.Duration(seconds*float64(time.Second)), 0)
		}
		// RFC 7231 also allows an HTTP-date. Discord sends seconds, so this is
		// the unusual path — but a date read as garbage falls back to a second
		// and retries into the same limit. A date already past means "now".
		if when, err := http.ParseTime(v); err == nil {
			return max(time.Until(when), 0)
		}
	}
	return time.Second
}
