package fusionauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Identifiers reach this client from a URL path, and Go's ServeMux unescapes
// path segments after routing — so a percent-encoded "../" arrives intact in
// PathValue. Concatenated into the upstream URL it becomes a request to a
// different FusionAuth endpoint, carrying the admin API key: /api/key lists
// API keys, /api/user/search dumps the directory. A bare "?" appends query
// parameters to whatever call is being made.
//
// The endpoint that takes this identifier needs no login, so this was
// reachable by anyone.
func TestIdentifierCannotEscapeItsPathSegment(t *testing.T) {
	var wire string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// RequestURI is what actually went over the connection, before any
		// decoding this side might do — which is what the upstream server
		// resolves.
		wire = r.RequestURI
		_, _ = io.WriteString(w, `{"user":{"id":"x"}}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "admin-api-key")

	hostile := []string{
		"../../api/key",
		"..%2f..%2fapi%2fkey",
		"../api/user/search",
		"nobody?queryParam=1",
		"nobody#fragment",
		"nobody/../../api/key",
	}

	for _, id := range hostile {
		t.Run(id, func(t *testing.T) {
			wire = ""
			_, err := c.GetUser(context.Background(), id)

			if err == nil {
				t.Fatalf("a malformed identifier was accepted")
			}
			// Best outcome: no request is made at all.
			if wire != "" {
				t.Errorf("a request was sent for a malformed identifier: %s", wire)
			}
		})
	}
}

// The same guard must apply to writes, or clearing a Discord link becomes a
// PATCH against an arbitrary endpoint.
func TestPatchRejectsMalformedIdentifiers(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = io.WriteString(w, `{"user":{"id":"x"}}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "admin-api-key")
	if _, err := c.PatchUser(context.Background(), "../../api/key", nil); err == nil {
		t.Error("PatchUser accepted a traversal")
	}
	if called {
		t.Error("PatchUser sent a request for a malformed identifier")
	}
}

// A real identifier must still work, and must go out intact.
func TestValidIdentifierIsUnchanged(t *testing.T) {
	const id = "00000000-1111-2222-3333-444444444444"

	var wire string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wire = r.RequestURI
		_, _ = io.WriteString(w, `{"user":{"id":"x"}}`)
	}))
	defer srv.Close()

	if _, err := New(srv.URL, "k").GetUser(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if want := "/api/user/" + id; wire != want {
		t.Errorf("sent %q, want %q", wire, want)
	}
	if strings.Contains(wire, "%") {
		t.Errorf("a well-formed identifier was needlessly escaped: %q", wire)
	}
}

func TestValidID(t *testing.T) {
	valid := []string{
		"00000000-1111-2222-3333-444444444444",
		"AbCdEf01-2345-6789-abcd-ef0123456789",
	}
	invalid := []string{
		"", "not-a-uuid", "../../api/key", "nobody?q=1",
		"00000000-1111-2222-3333-44444444444",   // too short
		"00000000-1111-2222-3333-4444444444444", // too long
		"00000000_1111_2222_3333_444444444444",  // wrong separator
	}
	for _, id := range valid {
		if !ValidID(id) {
			t.Errorf("ValidID(%q) = false, want true", id)
		}
	}
	for _, id := range invalid {
		if ValidID(id) {
			t.Errorf("ValidID(%q) = true, want false", id)
		}
	}
}
