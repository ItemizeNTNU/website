package store

import "testing"

// redactURI is what keeps database credentials out of error messages and log
// lines. If it ever passes a password through, the secret ends up in whatever
// log aggregator the deployment ships to.
func TestRedactURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want string
	}{
		{
			"no scheme separator passes through untouched",
			"just-a-hostname",
			"just-a-hostname",
		},
		{
			"URI without credentials is unchanged",
			"mongodb://host:27017/db",
			"mongodb://host:27017/db",
		},
		{
			"credentials are masked",
			"mongodb://user:pass@host/db",
			"mongodb://****@host/db",
		},
		{
			// The last @ separates credentials from host, so a password
			// containing @ must not leak its tail.
			"password containing @ is fully masked",
			"mongodb://u:p@ss@host/db",
			"mongodb://****@host/db",
		},
		{
			"empty string is unchanged",
			"",
			"",
		},
		{
			"mongodb+srv scheme is preserved",
			"mongodb+srv://u:p@host/db",
			"mongodb+srv://****@host/db",
		},
		{
			// An @ in the query string with no credentials present makes
			// redactURI mask everything up to that @, mangling the host and
			// options. That over-redacts — the operator loses context — but it
			// fails safe: nothing that could be a secret survives. This case
			// pins the current behaviour rather than endorsing it.
			"@ in the query string over-redacts but never leaks",
			"mongodb://host/db?opt=a@b",
			"mongodb://****@b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactURI(tt.uri); got != tt.want {
				t.Errorf("redactURI(%q) = %q, want %q — a wrong redaction either leaks a credential into logs or hides the host an operator needs to debug connectivity", tt.uri, got, tt.want)
			}
		})
	}
}

func TestCutAfter(t *testing.T) {
	tests := []struct {
		name   string
		s, sep string
		before string
		after  string
		found  bool
	}{
		{"separator in the middle", "mongodb://host", "://", "mongodb", "host", true},
		{"separator absent", "no-scheme-here", "://", "no-scheme-here", "", false},
		{"separator at position zero", "://rest", "://", "", "rest", true},
		{"separator at the end", "abc://", "://", "abc", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, after, found := cutAfter(tt.s, tt.sep)
			if before != tt.before || after != tt.after || found != tt.found {
				t.Errorf("cutAfter(%q, %q) = (%q, %q, %v), want (%q, %q, %v) — redactURI relies on this split to keep the scheme intact while masking what follows",
					tt.s, tt.sep, before, after, found, tt.before, tt.after, tt.found)
			}
		})
	}
}

func TestLastIndexByte(t *testing.T) {
	tests := []struct {
		name string
		s    string
		b    byte
		want int
	}{
		{"byte absent", "hostname", '@', -1},
		{"single occurrence", "user@host", '@', 4},
		{"multiple occurrences finds the last", "u:p@ss@host", '@', 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lastIndexByte(tt.s, tt.b); got != tt.want {
				t.Errorf("lastIndexByte(%q, %q) = %d, want %d — finding anything but the last @ would leave part of a password visible after redaction",
					tt.s, tt.b, got, tt.want)
			}
		})
	}
}
