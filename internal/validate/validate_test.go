package validate

import (
	"testing"
)

// A field usually fails one check first and every later check as a
// consequence; showing the second message would point the user at the wrong
// problem.
func TestAddKeepsFirstMessage(t *testing.T) {
	var e Errors
	e.Add("name", "first")
	e.Add("name", "second")
	if e["name"] != "first" {
		t.Errorf("got %q, want the first message to win — a later, derivative error must not replace the root cause", e["name"])
	}
	if len(e) != 1 {
		t.Errorf("len = %d, want 1", len(e))
	}
}

// Callers declare `var e Errors` and go; Add must initialize the nil map
// itself, or every handler would need a constructor call.
func TestAddOnZeroValueInitializes(t *testing.T) {
	var e Errors
	e.Add("field", "msg")
	if e == nil {
		t.Fatal("Add left the map nil — the zero value is documented as ready to use")
	}
	if e["field"] != "msg" {
		t.Errorf("got %q, want %q", e["field"], "msg")
	}
}

func TestAnyAndFirstOnEmpty(t *testing.T) {
	var e Errors
	if e.Any() {
		t.Error("Any() on an empty Errors reported a failure — a valid form would be rejected")
	}
	if got := e.First(); got != "" {
		t.Errorf("First() on an empty Errors = %q, want \"\"", got)
	}
}

// Go map iteration is random; the JSON endpoints return a single message, and
// one that changes between identical requests is untestable and bewildering
// to debug. First must pick by sorted field name, every time.
func TestFirstIsDeterministic(t *testing.T) {
	e := Errors{"b": "msg b", "a": "msg a", "c": "msg c"}
	for i := 0; i < 100; i++ {
		if got := e.First(); got != "msg a" {
			t.Fatalf("call %d: First() = %q, want %q (message for the alphabetically first field)", i, got, "msg a")
		}
	}
}

// Handlers return the Errors value straight up as an error.
func TestErrorsImplementsError(t *testing.T) {
	var err error = Errors{"x": "msg"}
	if err.Error() != "msg" {
		t.Errorf("Error() = %q, want %q", err.Error(), "msg")
	}
}

func TestText(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		min, max int
		wantVal  string
		wantErr  string
	}{
		{
			"empty is required",
			"", 2, 10,
			"", "Navn må fylles ut.",
		},
		{
			// A field of only spaces must not slip past required as a
			// nonempty string.
			"whitespace-only is required, not too-short",
			"   \t ", 2, 10,
			"", "Navn må fylles ut.",
		},
		{
			// Norwegian names are full of æøå; counting bytes would reject
			// them at half their real length.
			"length is counted in runes, not bytes",
			"æøå", 3, 10,
			"æøå", "",
		},
		{
			"exactly min passes",
			"ab", 2, 10,
			"ab", "",
		},
		{
			"exactly max passes",
			"🎈🎈🎈🎈🎈", 1, 5,
			"🎈🎈🎈🎈🎈", "",
		},
		{
			"one emoji over max fails",
			"🎈🎈🎈🎈🎈🎈", 1, 5,
			"🎈🎈🎈🎈🎈🎈", "Navn kan ikke være lengre enn 5 tegn.",
		},
		{
			"below min fails",
			"a", 2, 10,
			"a", "Navn må være minst 2 tegn.",
		},
		{
			// The trimmed value comes back even when validation fails, so the
			// form can be re-rendered with what the user actually typed.
			"trimmed value is returned on failure too",
			"  a  ", 2, 10,
			"a", "Navn må være minst 2 tegn.",
		},
		{
			"surrounding whitespace is trimmed before checking",
			"  ola  ", 2, 10,
			"ola", "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e Errors
			got := e.Text("f", "Navn", tt.in, tt.min, tt.max)
			if got != tt.wantVal {
				t.Errorf("returned %q, want %q", got, tt.wantVal)
			}
			if e["f"] != tt.wantErr {
				t.Errorf("error = %q, want %q", e["f"], tt.wantErr)
			}
		})
	}
}

func TestOptional(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		max     int
		wantVal string
		wantErr string
	}{
		{
			"empty passes without complaint",
			"", 10,
			"", "",
		},
		{
			"over max in runes fails",
			"æøåæøåæøåæø", 10,
			"æøåæøåæøåæø", "Om deg kan ikke være lengre enn 10 tegn.",
		},
		{
			"trims before counting",
			"  hei  ", 10,
			"hei", "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e Errors
			got := e.Optional("f", "Om deg", tt.in, tt.max)
			if got != tt.wantVal {
				t.Errorf("returned %q, want %q", got, tt.wantVal)
			}
			if e["f"] != tt.wantErr {
				t.Errorf("error = %q, want %q", e["f"], tt.wantErr)
			}
		})
	}
}

func TestHTTPSURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantVal string
		wantErr string
	}{
		{
			"empty is allowed — the field is optional",
			"", "", "",
		},
		{
			"leading and trailing space is trimmed, not rejected",
			"  https://itemize.no  ", "https://itemize.no", "",
		},
		{
			// This pins current behavior: the check is prefix-only, so the
			// bare scheme with no host at all sails through.
			"bare https:// passes the prefix-only check",
			"https://", "https://", "",
		},
		{
			"plain http is rejected",
			"http://x", "http://x", "the URL needs to start with https://",
		},
		{
			// The prefix check is case-sensitive, so an uppercased scheme —
			// which browsers would accept — is refused. Pinned as-is.
			"uppercase HTTPS is rejected",
			"HTTPS://x.no", "HTTPS://x.no", "the URL needs to start with https://",
		},
		{
			"regular space inside is rejected",
			"https://x .no", "https://x .no", "the URL can not include whitespace",
		},
		{
			// A URL pasted from a rich-text source really can carry U+00A0.
			"non-breaking space inside is rejected",
			"https://x\u00a0.no", "https://x\u00a0.no", "the URL can not include whitespace",
		},
		{
			"tab inside is rejected",
			"https://x\t.no", "https://x\t.no", "the URL can not include whitespace",
		},
		{
			"newline inside is rejected",
			"https://x\n.no", "https://x\n.no", "the URL can not include whitespace",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e Errors
			got := e.HTTPSURL("f", tt.in)
			if got != tt.wantVal {
				t.Errorf("returned %q, want %q", got, tt.wantVal)
			}
			if e["f"] != tt.wantErr {
				t.Errorf("error = %q, want %q", e["f"], tt.wantErr)
			}
		})
	}
}

func TestNumber(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		min, max float64
		wantN    float64
		wantErr  string
	}{
		{
			"empty is required",
			"", 0, 100,
			0, "Pris må fylles ut.",
		},
		{
			"letters are not a number",
			"abc", 0, 100,
			0, "Pris må være et tall.",
		},
		{
			// A Norwegian keyboard produces the comma decimal separator.
			"comma decimal separator is accepted",
			"2,5", 0, 100,
			2.5, "",
		},
		{
			// Pins a quirk: only the FIRST comma is replaced with a dot, so a
			// thousands-separated "1,000.5" becomes "1.000.5" and fails to
			// parse. Anyone typing US-style thousands separators is rejected.
			"thousands separator is rejected, not parsed",
			"1,000.5", 0, 10000,
			0, "Pris må være et tall.",
		},
		{
			"multiple commas are rejected",
			"1,2,3", 0, 100,
			0, "Pris må være et tall.",
		},
		{
			"exactly min passes",
			"0,5", 0.5, 100,
			0.5, "",
		},
		{
			"exactly max passes",
			"100", 0, 100,
			100, "",
		},
		{
			// %g renders 0.5 as "0.5", not "0.500000".
			"below min fails with a %g-formatted bound",
			"0,4", 0.5, 100,
			0.4, "Pris kan ikke være mindre enn 0.5.",
		},
		{
			"above max fails with a %g-formatted bound",
			"100,5", 0, 100,
			100.5, "Pris kan ikke være større enn 100.",
		},
		{
			"surrounding whitespace is trimmed",
			"  3  ", 0, 100,
			3, "",
		},
		{
			// ParseFloat accepts "NaN", and NaN compares false against both
			// bounds — so without an explicit check it passes every range,
			// however narrow, and reaches whatever the caller does with it.
			"NaN parses but is not a number the range check can hold",
			"NaN", 0, 100,
			0, "Pris må være et tall.",
		},
		{
			"the spelling of NaN is not significant",
			"nan", 0, 100,
			0, "Pris må være et tall.",
		},
		{
			// Infinity is caught by a ceiling but not by its absence, and it is
			// not a quantity anybody can have typed on purpose.
			"positive infinity is refused as not a number",
			"Inf", 0, 100,
			0, "Pris må være et tall.",
		},
		{
			"negative infinity too",
			"-Inf", 0, 100,
			0, "Pris må være et tall.",
		},
		{
			"and the long spelling",
			"infinity", 0, 100,
			0, "Pris må være et tall.",
		},
		{
			// A number this large is finite, so it is a range problem rather than
			// a parse one — the distinction is what keeps the message truthful.
			"a huge but finite number is out of range, not unparseable",
			"1e300", 0, 100,
			1e300, "Pris kan ikke være større enn 100.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e Errors
			got := e.Number("f", "Pris", tt.in, tt.min, tt.max)
			if got != tt.wantN {
				t.Errorf("returned %v, want %v", got, tt.wantN)
			}
			if e["f"] != tt.wantErr {
				t.Errorf("error = %q, want %q", e["f"], tt.wantErr)
			}
		})
	}
}

func TestOptionalNumber(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantN   float64
		wantErr string
	}{
		{"empty is fine and means zero", "", 0, ""},
		{"whitespace only is empty too", "   ", 0, ""},
		{"a value present must still parse", "abc", 0, "Pris må være et tall."},
		{"a value present must still be in range", "101", 101, "Pris kan ikke være større enn 100."},
		{"a valid value passes through", "2,5", 2.5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e Errors
			got := e.OptionalNumber("f", "Pris", tt.in, 0, 100)
			if got != tt.wantN {
				t.Errorf("returned %v, want %v", got, tt.wantN)
			}
			if e["f"] != tt.wantErr {
				t.Errorf("error = %q, want %q", e["f"], tt.wantErr)
			}
		})
	}
}

func TestInt(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		min, max int
		wantN    int
		wantErr  string
	}{
		{
			// A decimal is a valid Number but not a valid Int — the message
			// says heltall, whole number, so the user knows why.
			"a decimal is not a whole number",
			"2.5", 0, 10,
			0, "Årstall må være et heltall.",
		},
		{
			"exactly min passes",
			"1", 1, 5,
			1, "",
		},
		{
			"exactly max passes",
			"5", 1, 5,
			5, "",
		},
		{
			"one below min fails",
			"0", 1, 5,
			0, "Årstall kan ikke være mindre enn 1.",
		},
		{
			"one above max fails",
			"6", 1, 5,
			6, "Årstall kan ikke være større enn 5.",
		},
		{
			"surrounding whitespace is trimmed",
			"  3  ", 1, 5,
			3, "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e Errors
			got := e.Int("f", "Årstall", tt.in, tt.min, tt.max)
			if got != tt.wantN {
				t.Errorf("returned %v, want %v", got, tt.wantN)
			}
			if e["f"] != tt.wantErr {
				t.Errorf("error = %q, want %q", e["f"], tt.wantErr)
			}
		})
	}
}

func TestOneOf(t *testing.T) {
	t.Run("a member is returned trimmed with no error", func(t *testing.T) {
		var e Errors
		got := e.OneOf("type", "Medlemstype", "student", "student", "alumni", "employee")
		if got != "student" {
			t.Errorf("returned %q, want %q", got, "student")
		}
		if e.Any() {
			t.Errorf("a valid choice was rejected: %v", e)
		}
	})

	t.Run("leading and trailing space still matches", func(t *testing.T) {
		var e Errors
		got := e.OneOf("type", "Medlemstype", " student ", "student", "alumni", "employee")
		if got != "student" {
			t.Errorf("returned %q, want %q", got, "student")
		}
		if e.Any() {
			t.Errorf("a padded but valid choice was rejected: %v", e)
		}
	})

	t.Run("a non-member returns empty and records an error", func(t *testing.T) {
		var e Errors
		got := e.OneOf("type", "Medlemstype", "styremedlem", "student", "alumni", "employee")
		// The empty return is load-bearing: users.FromForm switches on the
		// returned value, and "" falls through to the default branch instead
		// of reading fields for a type that does not exist. If OneOf ever
		// returned the raw input on failure, a crafted submission could smuggle
		// data through.
		if got != "" {
			t.Errorf("returned %q, want \"\" — callers switch on the result and rely on rejection returning empty", got)
		}
		if e["type"] != "Medlemstype er ikke et gyldig valg." {
			t.Errorf("error = %q, want the invalid-choice message", e["type"])
		}
	})
}
