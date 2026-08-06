// Package fusionauth is a client for the FusionAuth user API.
package fusionauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"time"
)

// maxBody caps how much of a response is read.
const maxBody = 1 << 20

// ErrNotConfigured is returned when no API token is set. Login still works
// without one; only the calls that read or write user records do not.
var ErrNotConfigured = errors.New("FusionAuth API token is not configured")

// ErrNotFound is returned when no such user exists.
var ErrNotFound = errors.New("user not found")

// ErrInvalidID is returned for an identifier that is not a FusionAuth user id.
var ErrInvalidID = errors.New("not a valid user id")

// uuidPattern matches the canonical form FusionAuth issues.
//
// Identifiers reach this package from a URL path, and Go's ServeMux unescapes
// path segments after routing — so a percent-encoded "../" arrives intact.
// Concatenated into the upstream URL that becomes a request to a different
// FusionAuth endpoint, carrying the admin API key: /api/key lists API keys,
// /api/user/search dumps the directory. A bare "?" is enough to append query
// parameters to whatever call is being made.
//
// Escaping the segment (below) already prevents that. Checking the shape as
// well means a malformed identifier is refused here rather than becoming a
// request at all.
var uuidPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidID reports whether id is a well-formed FusionAuth user identifier.
func ValidID(id string) bool { return uuidPattern.MatchString(id) }

// Client talks to the FusionAuth user API.
type Client struct {
	host  string
	token string
	http  *http.Client
}

// New builds a client. An empty token yields a client whose calls all return
// ErrNotConfigured, so a contributor can run the site without a production
// API key and see a clear 503 rather than a confusing failure.
func New(host, token string) *Client {
	return &Client{
		host:  host,
		token: token,
		http:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Configured reports whether calls will work.
func (c *Client) Configured() bool { return c != nil && c.token != "" }

// User is the subset of a FusionAuth user this site handles.
type User struct {
	ID       string         `json:"id,omitempty"`
	Email    string         `json:"email"`
	FullName string         `json:"fullName"`
	ImageURL string         `json:"imageUrl,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

type userEnvelope struct {
	User User `json:"user"`
}

type createRequest struct {
	SendSetPasswordEmail bool `json:"sendSetPasswordEmail"`
	User                 User `json:"user"`
}

// CreateUser registers a new member and asks FusionAuth to email them a
// password-setting link.
func (c *Client) CreateUser(ctx context.Context, u User) (*User, error) {
	var out userEnvelope
	err := c.do(ctx, http.MethodPost, "/api/user",
		createRequest{SendSetPasswordEmail: true, User: u}, &out)
	if err != nil {
		return nil, err
	}
	return &out.User, nil
}

// GetUser fetches a user by identifier.
func (c *Client) GetUser(ctx context.Context, id string) (*User, error) {
	if !ValidID(id) {
		return nil, ErrInvalidID
	}
	var out userEnvelope
	if err := c.do(ctx, http.MethodGet, "/api/user/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out.User, nil
}

// PatchUser merges changes into a user record.
//
// FusionAuth's PATCH is a merge, so an explicit JSON null deletes a key —
// which is how the Discord link is cleared. Callers must therefore pass a
// literal nil in the map rather than omitting the field, and nothing on this
// path may use omitempty.
func (c *Client) PatchUser(ctx context.Context, id string, changes map[string]any) (*User, error) {
	if !ValidID(id) {
		return nil, ErrInvalidID
	}
	var out userEnvelope
	err := c.do(ctx, http.MethodPatch, "/api/user/"+url.PathEscape(id),
		map[string]any{"user": changes}, &out)
	if err != nil {
		return nil, err
	}
	return &out.User, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	if !c.Configured() {
		return ErrNotConfigured
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.host+path, reader)
	if err != nil {
		return err
	}
	// FusionAuth expects the API key as the bare header value. There is no
	// "Bearer" prefix, and adding one — which looks like the obvious fix —
	// produces a 401.
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("fusionauth: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseError(resp.StatusCode, payload)
	}

	if out != nil && len(payload) > 0 {
		return json.Unmarshal(payload, out)
	}
	return nil
}

// APIError is a validation or conflict response from FusionAuth.
type APIError struct {
	Status        int
	GeneralErrors []errorDetail            `json:"generalErrors"`
	FieldErrors   map[string][]errorDetail `json:"fieldErrors"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string { return e.UserMessage() }

// UserMessage extracts the message worth showing to the person filling in the
// form — most often "email already in use".
//
// Field errors arrive as a JSON object. The previous implementation took
// Object.keys()[0], which in JavaScript is insertion order; Go map iteration
// is deliberately random, so without sorting the same rejected registration
// would show a different message each time. That is untestable and bewildering
// to support.
func (e *APIError) UserMessage() string {
	if len(e.GeneralErrors) > 0 && e.GeneralErrors[0].Message != "" {
		return e.GeneralErrors[0].Message
	}
	if len(e.FieldErrors) > 0 {
		fields := make([]string, 0, len(e.FieldErrors))
		for field := range e.FieldErrors {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		if details := e.FieldErrors[fields[0]]; len(details) > 0 {
			return details[0].Message
		}
	}
	return fmt.Sprintf("Uventet svar fra innloggingstjenesten (HTTP %d).", e.Status)
}

func parseError(status int, payload []byte) error {
	apiErr := &APIError{Status: status}
	_ = json.Unmarshal(payload, apiErr)
	return apiErr
}
