package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Every status and error in this package goes out through the same
// single-field envelope, because clients key off `message` and the shape
// predates the rewrite. A response that has grown a second field is harmless; a
// response that has lost this one is a silent break in something we cannot see.
func TestWriteJSONEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()

	writeJSON(rec, http.StatusTeapot, message{"Noe gikk galt"})

	if rec.Code != http.StatusTeapot {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusTeapot)
	}
	wantJSON(t, rec)
	// The encoder appends a newline, which is worth pinning: a client reading
	// the stream line by line depends on it being there.
	if got := rec.Body.String(); got != `{"message":"Noe gikk galt"}`+"\n" {
		t.Errorf("the envelope encoded as %q", got)
	}
}

// The Content-Type is set before the status line, which is the only order that
// works: headers written after WriteHeader are discarded, and the response
// would go out as text/plain with the body a JSON client never parses.
func TestWriteJSONSetsTheContentTypeBeforeTheStatus(t *testing.T) {
	rec := httptest.NewRecorder()

	writeJSON(rec, http.StatusNotFound, message{"borte"})

	if got := rec.Result().Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("the committed response carries Content-Type %q; a header set after "+
			"the status line is dropped", got)
	}
}

// Go's encoder escapes the runes that are significant in HTML, so a message
// carrying caller-supplied text — which the check-in code is — cannot close a
// script element if the response is ever interpolated into a page. Norwegian
// characters are left alone, which is the other half of what makes the messages
// readable.
func TestWriteJSONEscaping(t *testing.T) {
	// The runes that must never reach the body unescaped. Written as runes
	// rather than as their \u forms so the assertion is about the property —
	// "this character is not in the output" — rather than about which of the
	// several legal escapes the encoder happens to pick.
	htmlSignificant := []string{"<", ">", "&"}

	for _, tc := range []struct {
		name string
		in   string
		// literal marks the inputs that must survive as themselves. Escaping
		// them would still be valid JSON but unreadable in a log or a terminal,
		// which is where these messages are usually read.
		literal bool
	}{
		{name: "a script tag", in: "<script>alert(1)</script>"},
		{name: "an ampersand", in: "Kari & Ola"},
		{name: "an HTML comment", in: "<!-- skjult -->"},
		// JSON requires escaping these two, so only the round trip is asserted.
		{name: "a quote", in: `si "hei"`},
		{name: "a newline", in: "linje\nto"},
		{name: "Norwegian characters", in: "Ærlig øl på Åsen", literal: true},
		{name: "an emoji", in: "ferdig 🎉", literal: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()

			writeJSON(rec, http.StatusOK, message{tc.in})
			body := rec.Body.String()

			for _, char := range htmlSignificant {
				if strings.Contains(tc.in, char) && strings.Contains(body, char) {
					t.Errorf("%q reached the body unescaped: %s", char, body)
				}
			}
			if tc.literal && !strings.Contains(body, tc.in) {
				t.Errorf("%q was escaped rather than written literally: %s", tc.in, body)
			}

			// Whatever the escaping, it has to decode back to the original.
			var out message
			if err := json.Unmarshal([]byte(body), &out); err != nil {
				t.Fatalf("the encoded body no longer parses: %v", err)
			}
			if out.Message != tc.in {
				t.Errorf("decoded to %q, want %q", out.Message, tc.in)
			}
		})
	}
}

// A value that cannot be encoded arrives after the status line has been sent,
// so there is nothing to salvage — but it must not panic and take the
// connection with it, and it must not be silent either.
func TestWriteJSONWithAnUnencodableBody(t *testing.T) {
	// The failure is logged through the package-level logger, which would
	// otherwise print to stderr during the run.
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	rec := httptest.NewRecorder()

	writeJSON(rec, http.StatusOK, map[string]any{"ch": make(chan int)})

	if rec.Code != http.StatusOK {
		t.Errorf("the status is %d; it was already sent and cannot change", rec.Code)
	}
	// The truncated body is the honest outcome: the header promised JSON and the
	// encoder produced none.
	if body := strings.TrimSpace(rec.Body.String()); body != "" {
		t.Errorf("an unencodable value produced the partial body %q", body)
	}
}
