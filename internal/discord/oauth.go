package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// authorizeURL is the OAuth2 authorization endpoint. It lives outside the
// versioned API path, unlike everything else here.
const authorizeURL = "https://discord.com/oauth2/authorize"

// AuthorizeURL builds the link a member follows to grant access.
//
// The only scope requested is identify: enough to learn who they are, and
// nothing more. state must be unpredictable and bound to the session — the
// previous server hardcoded it to "123", with a comment acknowledging as much.
func (c *Client) AuthorizeURL(state, redirectURI string) string {
	q := url.Values{
		"client_id":     {c.cfg.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {"identify"},
		"state":         {state},
		"prompt":        {"consent"},
	}
	return authorizeURL + "?" + q.Encode()
}

// ErrDenied is returned when the member declined the authorization.
var ErrDenied = errors.New("discord authorization was declined")

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

// Exchange trades the authorization code for an access token and returns the
// account it belongs to.
func (c *Client) Exchange(ctx context.Context, code, redirectURI string) (*User, error) {
	if code == "" {
		return nil, ErrDenied
	}

	form := url.Values{
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		APIBase+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	// The token endpoint authenticates with the client credentials in the
	// body, not the bot token every other call uses.
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discord: exchanging the code: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The client secret and the code are what this request carried, so a
		// 401 here is about one of those and never about the bot token.
		return nil, newAPIError(resp.StatusCode, payload, credentialClientSecret)
	}

	var token tokenResponse
	if err := json.Unmarshal(payload, &token); err != nil {
		return nil, err
	}
	if token.AccessToken == "" {
		return nil, errors.New("discord: no access token in the response")
	}

	return c.currentUser(ctx, token.AccessToken)
}

// currentUser reads the account behind an access token.
func (c *Client) currentUser(ctx context.Context, accessToken string) (*User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, APIBase+"/users/@me", nil)
	if err != nil {
		return nil, err
	}
	// A user token, not the bot token.
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discord: fetching the account: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// This leg carries the member's bearer token, not the bot token.
		return nil, newAPIError(resp.StatusCode, payload, credentialAccessToken)
	}

	var u User
	if err := json.Unmarshal(payload, &u); err != nil {
		return nil, err
	}
	if u.ID == "" {
		return nil, errNoAccountID
	}
	return &u, nil
}

// errNoAccountID is returned when Discord answers an account lookup without an
// identifier — a 204, or a 200 carrying `{}`. The id is the only field the link
// cannot do without: stored blank, it makes every later guild and role call
// fail on an empty identifier.
var errNoAccountID = errors.New("discord: the account response carried no id")

// GetUser reads an account by identifier, using the bot's own credentials.
func (c *Client) GetUser(ctx context.Context, id string) (*User, error) {
	if !ValidID(id) {
		return nil, ErrInvalidID
	}
	var u User
	if err := c.do(ctx, http.MethodGet, "/users/"+id, nil, &u); err != nil {
		return nil, err
	}
	// The same guard currentUser applies. do leaves the destination untouched
	// on an empty 2xx body, so without this a 204 would come back as a
	// perfectly valid-looking account with no id in it.
	if u.ID == "" {
		return nil, errNoAccountID
	}
	return &u, nil
}
