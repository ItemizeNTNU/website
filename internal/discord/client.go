// Package discord talks to the Discord API.
//
// Version 10. The previous server used v8, which is deprecated — v6 and v7 are
// already decommissioned and v8 is on the same path.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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

// APIError is a non-2xx response from Discord.
type APIError struct {
	Status  int
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	// A 401 on a bot-token call is almost always the token itself, and the
	// bare status says nothing about where to look. Naming the variable turns
	// a support conversation into a one-line check.
	if e.Status == http.StatusUnauthorized {
		return "discord: the bot token was rejected (HTTP 401) — check " +
			"DISCORD_BOT_TOKEN; it must be the bot token from Bot → Reset Token, " +
			"not the client secret, and it is invalidated whenever it is reset"
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
			wait := retryAfter(resp)
			c.log.Warn("rate limited by Discord, retrying", "path", path, "wait", wait)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			apiErr := &APIError{Status: resp.StatusCode}
			_ = json.Unmarshal(payload, apiErr)
			return apiErr
		}

		if out != nil && len(payload) > 0 {
			return json.Unmarshal(payload, out)
		}
		return nil
	}
}

func retryAfter(resp *http.Response) time.Duration {
	for _, header := range []string{"Retry-After", "X-RateLimit-Reset-After"} {
		if v := resp.Header.Get(header); v != "" {
			if seconds, err := strconv.ParseFloat(v, 64); err == nil {
				return time.Duration(seconds * float64(time.Second))
			}
		}
	}
	return time.Second
}
