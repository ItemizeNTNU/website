package config

// No test in this file may call t.Parallel: loadDotenv writes to the process
// environment, and the helpers below use t.Setenv to guarantee it is put back.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFile writes body to path, failing the test rather than the code under
// test if the fixture itself cannot be created.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing the fixture %s failed: %v", path, err)
	}
}

// unsetenv removes each key for the duration of the test and restores whatever
// the process had before.
//
// The t.Setenv call is what registers the restore; the immediate Unsetenv is
// what makes the key genuinely absent. That distinction matters: loadDotenv
// skips any key that is already set, and a key exported as the empty string
// still counts as set. Only a genuinely absent key is ever written to, so this
// is the only state in which the parser can be observed at all.
func unsetenv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("clearing %s failed: %v", key, err)
		}
	}
}

// loadFixture writes body to a .env in a fresh directory and loads it.
func loadFixture(t *testing.T, body string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	writeFile(t, path, body)
	return loadDotenv(path)
}

// The .env file is the first thing a new contributor edits and the last thing
// anyone thinks about when a secret stops working. Every case here is a shape
// that has actually appeared in one — quotes copied from a dashboard, a value
// containing an equals sign, a file saved on Windows.
func TestLoadDotenvParsing(t *testing.T) {
	tests := []struct {
		name string
		file string
		// want is checked with LookupEnv, so the empty string is distinct from
		// "never set".
		want map[string]string
		// unset names keys the file must not have produced.
		unset []string
	}{
		{
			name: "a plain assignment",
			file: "ITEMIZE_A=verdi\n",
			want: map[string]string{"ITEMIZE_A": "verdi"},
		},
		{
			name: "several assignments",
			file: "ITEMIZE_A=en\nITEMIZE_B=to\nITEMIZE_C=tre\n",
			want: map[string]string{"ITEMIZE_A": "en", "ITEMIZE_B": "to", "ITEMIZE_C": "tre"},
		},
		{
			name: "blank lines and comments are skipped",
			file: "# hele filen er kommentert\n\n   \nITEMIZE_A=verdi\n\n   # innrykket kommentar\nITEMIZE_B=to\n",
			want: map[string]string{"ITEMIZE_A": "verdi", "ITEMIZE_B": "to"},
		},
		{
			name: "the export prefix is stripped",
			file: "export ITEMIZE_A=verdi\n",
			want: map[string]string{"ITEMIZE_A": "verdi"},
		},
		{
			name: "an indented export prefix is stripped",
			file: "\texport ITEMIZE_A=verdi\n",
			want: map[string]string{"ITEMIZE_A": "verdi"},
		},
		{
			// Only the prefix followed by a space is removed, so a variable
			// that happens to start with those letters survives.
			name: "export is not stripped from the middle of a name",
			file: "exported ITEMIZE_A=verdi\nITEMIZE_EXPORT_B=to\n",
			want: map[string]string{"ITEMIZE_EXPORT_B": "to", "exported ITEMIZE_A": "verdi"},
		},
		{
			name: "double quotes are stripped",
			file: `ITEMIZE_A="verdi"` + "\n",
			want: map[string]string{"ITEMIZE_A": "verdi"},
		},
		{
			name: "single quotes are stripped",
			file: "ITEMIZE_A='verdi'\n",
			want: map[string]string{"ITEMIZE_A": "verdi"},
		},
		{
			name: "an empty quoted value is empty, not unset",
			file: "ITEMIZE_A=\"\"\nITEMIZE_B=''\n",
			want: map[string]string{"ITEMIZE_A": "", "ITEMIZE_B": ""},
		},
		{
			// Only one pair comes off, so a value that is genuinely quoted in
			// the provider's dashboard keeps its inner quotes.
			name: "only one pair of quotes is stripped",
			file: `ITEMIZE_A=""verdi""` + "\n",
			want: map[string]string{"ITEMIZE_A": `"verdi"`},
		},
		{
			name: "mismatched quotes are left alone",
			file: `ITEMIZE_A="verdi` + "\nITEMIZE_B='verdi\"\n",
			want: map[string]string{"ITEMIZE_A": `"verdi`, "ITEMIZE_B": `'verdi"`},
		},
		{
			name: "quotes inside an unquoted value survive",
			file: `ITEMIZE_A=pass"ord` + "\n",
			want: map[string]string{"ITEMIZE_A": `pass"ord`},
		},
		{
			// Escape sequences are deliberately not interpreted. A secret with
			// a backslash in it — and generated secrets do contain them —
			// must survive verbatim, which is worth more than \n support.
			name: "escape sequences are not interpreted",
			file: `ITEMIZE_A="linje1\nlinje2"` + "\n" + `ITEMIZE_B=C:\Users\itemize` + "\n",
			want: map[string]string{"ITEMIZE_A": `linje1\nlinje2`, "ITEMIZE_B": `C:\Users\itemize`},
		},
		{
			// Only the first equals sign separates. Base64 secrets end in
			// padding and connection strings are full of them.
			name: "an equals sign inside the value is kept",
			file: "ITEMIZE_A=a=b=c\nITEMIZE_B=aGVsbG8gd29ybGQ==\n",
			want: map[string]string{"ITEMIZE_A": "a=b=c", "ITEMIZE_B": "aGVsbG8gd29ybGQ=="},
		},
		{
			// A trailing "#" is not a comment. Treating it as one would mangle
			// every secret that happens to contain a hash, which is common.
			name: "a hash inside a value is not a comment",
			file: "ITEMIZE_A=pass#ord\nITEMIZE_B=verdi # ikke en kommentar\n",
			want: map[string]string{"ITEMIZE_A": "pass#ord", "ITEMIZE_B": "verdi # ikke en kommentar"},
		},
		{
			name: "whitespace around the name and the value is trimmed",
			file: "  ITEMIZE_A   =   verdi   \n",
			want: map[string]string{"ITEMIZE_A": "verdi"},
		},
		{
			// Quoting is how a value that really does begin or end with a
			// space is expressed.
			name: "quoting preserves inner whitespace",
			file: `ITEMIZE_A="  verdi  "` + "\n",
			want: map[string]string{"ITEMIZE_A": "  verdi  "},
		},
		{
			name: "an unquoted value that is only whitespace becomes empty",
			file: "ITEMIZE_A=    \n",
			want: map[string]string{"ITEMIZE_A": ""},
		},
		{
			name: "an assignment with nothing after the equals sign",
			file: "ITEMIZE_A=\n",
			want: map[string]string{"ITEMIZE_A": ""},
		},
		{
			// A file saved by a Windows editor, or one that made the trip
			// through a Windows clipboard. The carriage return must not end up
			// inside the value, where it would be sent as part of an HTTP
			// header and rejected.
			name: "CRLF line endings",
			file: "ITEMIZE_A=verdi\r\nITEMIZE_B=to\r\n",
			want: map[string]string{"ITEMIZE_A": "verdi", "ITEMIZE_B": "to"},
		},
		{
			name: "no trailing newline on the last line",
			file: "ITEMIZE_A=en\nITEMIZE_B=to",
			want: map[string]string{"ITEMIZE_A": "en", "ITEMIZE_B": "to"},
		},
		{
			// The first assignment wins, matching the rule that an
			// already-present value is never overwritten.
			name: "a duplicated name keeps the first value",
			file: "ITEMIZE_A=forste\nITEMIZE_A=andre\n",
			want: map[string]string{"ITEMIZE_A": "forste"},
		},
		{
			// A stray line must not abort the file: the assignments around it
			// still have to land, or one typo silently loses a secret.
			name:  "a line with no equals sign is skipped",
			file:  "ITEMIZE_A=en\ndette er ikke en tilordning\nITEMIZE_B=to\n",
			want:  map[string]string{"ITEMIZE_A": "en", "ITEMIZE_B": "to"},
			unset: []string{"dette er ikke en tilordning"},
		},
		{
			name:  "an empty name is skipped",
			file:  "=verdi\n   =verdi\nITEMIZE_A=en\n",
			want:  map[string]string{"ITEMIZE_A": "en"},
			unset: []string{""},
		},
		{
			name: "an empty file",
			file: "",
			want: map[string]string{},
		},
		{
			name:  "a file of only comments",
			file:  "# ingenting her\n# heller ikke her\n",
			unset: []string{"ITEMIZE_A"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var keys []string
			for key := range tt.want {
				keys = append(keys, key)
			}
			keys = append(keys, tt.unset...)
			// The empty name cannot be unset — os.Setenv rejects it — and it
			// is checked below by looking it up rather than by clearing it.
			for _, key := range keys {
				if key != "" {
					unsetenv(t, key)
				}
			}

			if err := loadFixture(t, tt.file); err != nil {
				t.Fatalf("loading the .env failed: %v", err)
			}

			for key, want := range tt.want {
				got, ok := os.LookupEnv(key)
				if !ok {
					t.Errorf("%s was never set, so anything reading it falls back to a default", key)
					continue
				}
				if got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
			for _, key := range tt.unset {
				if _, ok := os.LookupEnv(key); ok {
					t.Errorf("%q was exported into the environment; the parser accepted a line it should have skipped", key)
				}
			}
		})
	}
}

// An already-set variable always wins. That is what makes
// `FUSION_AUTH_SECRET=... ./website` work for a one-off, and what stops a
// forgotten .env from quietly overriding the environment a container was
// started with.
func TestLoadDotenvNeverOverridesTheEnvironment(t *testing.T) {
	unsetenv(t, "ITEMIZE_A", "ITEMIZE_B", "ITEMIZE_C")
	t.Setenv("ITEMIZE_A", "fra-miljoet")
	// Exported as the empty string. This is the interesting case: a
	// docker-compose file that passes through an unset variable produces
	// exactly this, and the deliberate emptiness must still win.
	t.Setenv("ITEMIZE_B", "")

	if err := loadFixture(t, "ITEMIZE_A=fra-filen\nITEMIZE_B=fra-filen\nITEMIZE_C=fra-filen\n"); err != nil {
		t.Fatalf("loading the .env failed: %v", err)
	}

	if got := os.Getenv("ITEMIZE_A"); got != "fra-miljoet" {
		t.Errorf("ITEMIZE_A = %q; the .env file overrode an explicitly exported value", got)
	}
	if got, ok := os.LookupEnv("ITEMIZE_B"); !ok || got != "" {
		t.Errorf("ITEMIZE_B = %q (set=%t); a variable exported as empty was refilled from the file", got, ok)
	}
	if got := os.Getenv("ITEMIZE_C"); got != "fra-filen" {
		t.Errorf("ITEMIZE_C = %q; a variable that was genuinely absent was not filled in from the file", got)
	}
}

// A UTF-8 byte-order mark is what Notepad and some editors on Windows put at
// the front of a saved file. It is invisible, and it becomes part of the first
// variable's name — so the first variable in the file silently does not exist.
// This is a known defect, pinned here so the behaviour cannot change unnoticed.
func TestLoadDotenvByteOrderMark(t *testing.T) {
	const bom = "\ufeff"
	unsetenv(t, "ITEMIZE_A", bom+"ITEMIZE_A", "ITEMIZE_B")

	if err := loadFixture(t, bom+"ITEMIZE_A=en\nITEMIZE_B=to\n"); err != nil {
		t.Fatalf("loading the .env failed: %v", err)
	}

	if _, ok := os.LookupEnv("ITEMIZE_A"); ok {
		t.Error("the byte-order mark is now stripped; this test documents the opposite and should be deleted along with the note in the parser")
	}
	if got := os.Getenv(bom + "ITEMIZE_A"); got != "en" {
		t.Errorf("the first assignment produced neither ITEMIZE_A nor a BOM-prefixed name (%q); the failure mode has changed", got)
	}
	// Only the first line is affected — the rest of the file loads normally,
	// which is what makes the problem so hard to spot.
	if got := os.Getenv("ITEMIZE_B"); got != "to" {
		t.Errorf("ITEMIZE_B = %q, want %q", got, "to")
	}
}

// A missing .env is the normal case: in production the environment comes from
// the orchestrator, and plenty of local checkouts never create one. It must not
// be an error, or the server refuses to start for the most ordinary reason
// there is.
func TestLoadDotenvMissingFileIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := loadDotenv(path); err != nil {
		t.Errorf("a missing .env was reported as an error (%v); the server would refuse to start without one", err)
	}
}

// A path that exists but cannot be read is a different story: the operator
// meant for it to be used, so silently continuing with a half-empty
// configuration is worse than refusing.
func TestLoadDotenvUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes do not restrict reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root, which ignores the permission bits this test relies on")
	}

	path := filepath.Join(t.TempDir(), ".env")
	writeFile(t, path, "ITEMIZE_A=verdi\n")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("making the fixture unreadable failed: %v", err)
	}

	if err := loadDotenv(path); err == nil {
		t.Error("an unreadable .env was silently ignored; the server would start with whatever configuration happened to be in the environment")
	}
}

// A directory where the file should be — an empty Docker volume mounted at
// ./.env is the usual way this happens — must not be mistaken for an empty
// file.
func TestLoadDotenvDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("creating the fixture directory failed: %v", err)
	}

	if err := loadDotenv(path); err == nil {
		t.Error("a directory named .env was accepted as an empty file; a bad volume mount would look like a missing configuration instead of a mounting mistake")
	}
}

// A line longer than the scanner's buffer stops the read dead. Everything after
// it would be missed, so the failure has to be reported rather than swallowed.
func TestLoadDotenvOverlongLine(t *testing.T) {
	unsetenv(t, "ITEMIZE_A", "ITEMIZE_B")

	err := loadFixture(t, "ITEMIZE_A="+strings.Repeat("x", 70_000)+"\nITEMIZE_B=to\n")
	if err == nil {
		t.Fatal("a .env line too long to scan was ignored; every variable after it would be silently missing")
	}
	if _, ok := os.LookupEnv("ITEMIZE_B"); ok {
		t.Error("a variable after the overlong line was applied, which means the load half-succeeded and then reported failure")
	}
}

// A .env saved as UTF-16 — which is what Notepad's "Unicode" option produces —
// is bytes of interleaved NUL, and a NUL in a variable name is the one thing
// os.Setenv refuses. The error has to come back out rather than being counted
// as a successful load of nothing.
func TestLoadDotenvRejectsNulInAName(t *testing.T) {
	unsetenv(t, "ITEMIZE_A")

	// "ITEMIZE_A=verdi", every character followed by the high byte of its
	// UTF-16 code unit.
	var utf16 strings.Builder
	for _, r := range "ITEMIZE_A=verdi" {
		utf16.WriteRune(r)
		utf16.WriteByte(0)
	}

	if err := loadFixture(t, utf16.String()+"\n"); err == nil {
		t.Error("a UTF-16 .env was loaded without complaint; the operator would see a server that behaves as if the file were empty")
	}
}

// unquote is exercised through the parser above, but the boundaries around
// one- and two-character values are easy to get wrong by an index and produce
// a panic rather than a wrong value.
func TestUnquote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: ""},
		{in: `"`, want: `"`},
		{in: "'", want: "'"},
		{in: `""`, want: ""},
		{in: "''", want: ""},
		{in: `"a"`, want: "a"},
		{in: "'a'", want: "a"},
		{in: "a", want: "a"},
		{in: "ab", want: "ab"},
		{in: `"ab`, want: `"ab`},
		{in: `ab"`, want: `ab"`},
		// Mixed quote characters are not a matching pair.
		{in: `"a'`, want: `"a'`},
		{in: `'a"`, want: `'a"`},
		{in: `"a"b"`, want: `a"b`},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := unquote(tt.in); got != tt.want {
				t.Errorf("unquote(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
