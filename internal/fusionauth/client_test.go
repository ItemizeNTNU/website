package fusionauth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Field errors arrive as a JSON object. JavaScript's Object.keys() is
// insertion-ordered, so the previous implementation happened to be stable; Go
// map iteration is deliberately random. Without sorting, the same rejected
// registration shows a different message each time — untestable, and
// bewildering when somebody reports it.
// A realistic identifier: the client validates the shape, so the placeholders
// this file used before would now be refused before any request is made.
const testID = "00000000-1111-2222-3333-444444444444"

func TestUserMessageIsDeterministic(t *testing.T) {
	err := &APIError{
		Status: 400,
		FieldErrors: map[string][]errorDetail{
			"user.username": {{Message: "Username is taken"}},
			"user.email":    {{Message: "Email is already in use"}},
			"user.password": {{Message: "Too short"}},
		},
	}

	first := err.UserMessage()
	for i := 0; i < 50; i++ {
		if got := err.UserMessage(); got != first {
			t.Fatalf("message changed between calls: %q then %q", first, got)
		}
	}
	// Sorted, so user.email wins.
	if first != "Email is already in use" {
		t.Errorf("got %q", first)
	}
}

func TestUserMessagePrefersGeneralErrors(t *testing.T) {
	err := &APIError{
		Status:        400,
		GeneralErrors: []errorDetail{{Message: "Something general"}},
		FieldErrors:   map[string][]errorDetail{"user.email": {{Message: "Field level"}}},
	}
	if got := err.UserMessage(); got != "Something general" {
		t.Errorf("got %q, want the general error", got)
	}
}

func TestUserMessageFallsBack(t *testing.T) {
	err := &APIError{Status: 503}
	if err.UserMessage() == "" {
		t.Error("no message for an empty error body")
	}
}

// The API key goes in the Authorization header bare. Adding a "Bearer" prefix
// looks like the obvious fix and produces a 401.
func TestAuthorizationHeaderHasNoBearerPrefix(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"user":{"id":"00000000-1111-2222-3333-444444444444"}}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "secret-api-key")
	if _, err := c.GetUser(context.Background(), testID); err != nil {
		t.Fatal(err)
	}
	if got != "secret-api-key" {
		t.Errorf("Authorization = %q, want the bare key", got)
	}
}

func TestNotConfigured(t *testing.T) {
	c := New("https://auth.example", "")
	if c.Configured() {
		t.Error("a client without a token reported itself configured")
	}
	if _, err := c.GetUser(context.Background(), testID); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("got %v, want ErrNotConfigured", err)
	}
}

func TestNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL, "token")
	if _, err := c.GetUser(context.Background(), testID); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestCreateUserSendsSetPasswordEmail(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = io.WriteString(w, `{"user":{"id":"fa-new"}}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "token")
	u, err := c.CreateUser(context.Background(), User{Email: "kari@example.no"})
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != "fa-new" {
		t.Errorf("id = %q", u.ID)
	}
	// Without this the new member never receives a way to set a password.
	if body["sendSetPasswordEmail"] != true {
		t.Error("sendSetPasswordEmail was not requested")
	}
}

// FusionAuth's PATCH is a merge, so an explicit null is what deletes a key.
// Nothing on this path may use omitempty, or clearing the Discord link would
// silently do nothing.
func TestPatchSendsExplicitNull(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{"user":{"id":"00000000-1111-2222-3333-444444444444"}}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "token")
	_, err := c.PatchUser(context.Background(), testID,
		map[string]any{"data": map[string]any{"discord": nil}})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(raw), `"discord":null`) {
		t.Errorf("expected an explicit null, got %s", raw)
	}
}

func contains(h, n string) bool {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}
