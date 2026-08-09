package store

import (
	"context"
	"strings"
	"testing"

	"github.com/ItemizeNTNU/website/internal/config"
)

// Connect is the first thing the binary does with the deployment's
// configuration. Everything here runs offline: a malformed connection string
// is rejected by the driver's parser before anything is dialled, and the
// reachability cases use an already-cancelled context, which fails at server
// selection. The unix-socket address they name does not exist, so no network
// connection is attempted either.

// A connection string the driver cannot parse has to fail here, at rollout,
// rather than at the first visitor. Returning a usable client for a nonsense
// URI would defer the failure to the events page.
func TestConnectRejectsMalformedConnectionStrings(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{"an empty connection string", ""},
		{"a bare hostname with no scheme", "mongo.example.no:27017"},
		{"the wrong scheme entirely", "http://mongo.example.no"},
		{"a scheme with no host", "mongodb://"},
		{"a port that is not a number", "mongodb://mongo.example.no:notaport/db"},
		{"an SRV connection string with several hosts", "mongodb+srv://u:p@h1.example.no,h2.example.no/db"},
		// A URI that parses but names an unreachable host is a different
		// failure, reported by the ping rather than the parser; it is covered
		// separately below. Only strings the parser itself rejects belong here.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, db, err := Connect(context.Background(), config.Mongo{URI: tt.uri, Database: "itemize"})
			if err == nil {
				t.Fatalf("Connect accepted %q; a broken connection string would not surface until the first visitor hit the events page", tt.uri)
			}
			if !strings.HasPrefix(err.Error(), "connecting to MongoDB:") {
				t.Errorf("error = %q, want it to name the step that failed so a deployment failure is self-explanatory", err)
			}
			// A caller that checks err and then uses the handles would panic
			// instead of reporting the problem.
			if client != nil || db != nil {
				t.Errorf("Connect returned handles alongside an error (client=%v db=%v)", client != nil, db != nil)
			}
		})
	}
}

// A server that cannot be reached must fail at startup, and the error goes
// into whatever log aggregator the deployment ships to. It has to name the
// host — an operator debugging connectivity needs it — without the password
// that sits next to it in the same string.
func TestConnectReportsAnUnreachableServerWithoutLeakingTheCredentials(t *testing.T) {
	const password = "sup3rhemmelig-passord"
	uri := "mongodb://itemize:" + password + "@%2Fnonexistent%2Fitemize-tests.sock/itemize?directConnection=true"

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client, db, err := Connect(ctx, config.Mongo{URI: uri, Database: "itemize"})
	if err == nil {
		t.Fatal("Connect succeeded against an address that does not exist")
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("the password appears in the error, and from there in every log line it reaches: %v", err)
	}
	if !strings.Contains(err.Error(), "****") {
		t.Errorf("error = %q, want the credentials replaced with a marker rather than dropped silently", err)
	}
	if !strings.Contains(err.Error(), "itemize-tests.sock") {
		t.Errorf("error = %q, want the host kept so an operator can tell which server is unreachable", err)
	}
	if client != nil || db != nil {
		t.Errorf("Connect returned handles alongside an error (client=%v db=%v); the caller would use a dead client", client != nil, db != nil)
	}
}

// The whole point of redactURI is that nothing recognisable as a secret
// survives it. The cases in mongo_test.go check the shape of the output; this
// checks the property that matters, across the connection-string forms a
// deployment might realistically use.
func TestRedactedURIsNeverContainTheSecret(t *testing.T) {
	const password = "P@ssord-med-tegn/og:mer"

	tests := []struct {
		name string
		uri  string
	}{
		{"a single host", "mongodb://itemize:" + password + "@mongo:27017/itemize"},
		{"a replica set", "mongodb://itemize:" + password + "@a:27017,b:27017/itemize?replicaSet=rs0"},
		{"an SRV record", "mongodb+srv://itemize:" + password + "@cluster.example.no/itemize?retryWrites=true"},
		{"authentication options after the database", "mongodb://itemize:" + password + "@mongo/itemize?authSource=admin&tls=true"},
		{"a unix domain socket", "mongodb://itemize:" + password + "@%2Ftmp%2Fmongo.sock/itemize"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactURI(tt.uri)
			if strings.Contains(got, password) {
				t.Fatalf("redactURI(%q) = %q — the password survived into something meant for a log line", tt.uri, got)
			}
			if strings.Contains(got, "itemize:") {
				t.Errorf("redactURI left the username in: %q", got)
			}
			if !strings.Contains(got, "****") {
				t.Errorf("redactURI(%q) = %q, want the mask present so it is obvious something was removed", tt.uri, got)
			}
		})
	}
}

func TestRedactURIFurtherShapes(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want string
	}{
		{
			"a username with no password is still masked",
			"mongodb://itemize@mongo/db",
			"mongodb://****@mongo/db",
		},
		{
			"an IPv6 host has no credentials to remove",
			"mongodb://[::1]:27017/db",
			"mongodb://[::1]:27017/db",
		},
		{
			"a replica set keeps every host",
			"mongodb://u:p@a:27017,b:27017/db?replicaSet=rs0",
			"mongodb://****@a:27017,b:27017/db?replicaSet=rs0",
		},
		{
			// The scheme is whatever precedes the first "://", so a password
			// that itself contains one still gets cut in the right place.
			"a password containing a scheme separator",
			"mongodb://u:p://q@mongo/db",
			"mongodb://****@mongo/db",
		},
		{
			// Not reachable in practice: Connect only calls redactURI once the
			// driver has accepted the URI, and it insists on a mongodb scheme.
			// Pinned so that a future caller who redacts something else knows
			// the function assumes a scheme is present.
			"a credential-bearing string with no scheme passes through untouched",
			"itemize:hemmelig@mongo/db",
			"itemize:hemmelig@mongo/db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactURI(tt.uri); got != tt.want {
				t.Errorf("redactURI(%q) = %q, want %q", tt.uri, got, tt.want)
			}
		})
	}
}

// cutAfter and lastIndexByte are hand-rolled because redactURI is the only
// caller. These are the inputs redactURI actually hands them at the edges.
func TestCutAfterAndLastIndexByteEdges(t *testing.T) {
	t.Run("an empty string has nothing to split", func(t *testing.T) {
		before, after, found := cutAfter("", "://")
		if before != "" || after != "" || found {
			t.Errorf("cutAfter(\"\", \"://\") = (%q, %q, %v), want the whole string back unsplit", before, after, found)
		}
	})

	t.Run("a string shorter than the separator", func(t *testing.T) {
		if _, _, found := cutAfter("ab", "://"); found {
			t.Error("a separator longer than the string was reported as found, which would index out of range")
		}
	})

	t.Run("only the first separator splits", func(t *testing.T) {
		before, after, found := cutAfter("a://b://c", "://")
		if before != "a" || after != "b://c" || !found {
			t.Errorf("cutAfter = (%q, %q, %v), want the scheme taken from the first separator only", before, after, found)
		}
	})

	t.Run("the last byte of the string", func(t *testing.T) {
		if got := lastIndexByte("mongo@", '@'); got != 5 {
			t.Errorf("lastIndexByte = %d, want 5 — redactURI slices one past this, which must stay in range", got)
		}
	})

	t.Run("an empty string has no bytes", func(t *testing.T) {
		if got := lastIndexByte("", '@'); got != -1 {
			t.Errorf("lastIndexByte(\"\") = %d, want -1", got)
		}
	})
}
