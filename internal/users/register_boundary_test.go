package users

// Input boundaries for the registration form. Everything here is about what a
// real browser, a Norwegian keyboard, a paste from a PDF or a crafted request
// can actually put in a field — and what ends up in FusionAuth's user.data
// afterwards, which nothing ever cleans up.

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// studentForm is a valid student submission, ready to have one field broken.
func studentForm() url.Values {
	f := base()
	f.Set("type", "student")
	f.Set("study.program", "Kommunikasjonsteknologi")
	f.Set("study.year", "3")
	f.Set("study.expectedFinishYear", "2028")
	return f
}

// employeeForm is a valid employee submission, which is the shortest one and
// therefore the least noisy fixture for testing a shared field.
func employeeForm() url.Values {
	f := base()
	f.Set("type", "employee")
	f.Set("employee.title", "Professor")
	return f
}

// A field holding nothing but whitespace is empty, not filled in. The
// non-breaking space matters most: it is what a paste from a PDF or a Word
// document carries, it renders identically to a space, and a member who typed
// nothing would otherwise be registered under a name made of invisible
// characters.
func TestWhitespaceOnlyValuesAreEmpty(t *testing.T) {
	blanks := map[string]string{
		"spaces":             "   ",
		"tab":                "\t",
		"newline":            "\n",
		"non-breaking space": "\u00a0",
		"mixed invisibles":   " \t\n\u00a0 ",
	}
	fields := map[string]string{
		"fullName":       "må fylles ut",
		"displayName":    "må fylles ut",
		"email":          "må fylles ut",
		"employee.title": "må fylles ut",
	}

	for blankName, blank := range blanks {
		for field, want := range fields {
			t.Run(blankName+"/"+field, func(t *testing.T) {
				f := employeeForm()
				f.Set(field, blank)

				r, verr := FromForm(f, now)
				msg := verr[field]
				if msg == "" {
					t.Fatalf("%q was accepted as %s; the member would be stored "+
						"under a name made of invisible characters", blank, field)
				}
				if !strings.Contains(msg, want) {
					t.Errorf("message %q does not say the field is required (%q)", msg, want)
				}
				if field == "fullName" && r.FullName != "" {
					t.Errorf("FullName = %q, want the whitespace dropped rather than "+
						"carried into FusionAuth", r.FullName)
				}
			})
		}
	}
}

// Surrounding whitespace is stripped rather than rejected, and it does not eat
// into the length budget — a name pasted with a trailing space must not be
// refused for being one character too long.
func TestValuesAreTrimmedNotRejected(t *testing.T) {
	f := employeeForm()
	f.Set("fullName", "  "+strings.Repeat("ø", fullNameMax)+"\t")
	f.Set("displayName", " kari ")
	f.Set("employee.title", "\tProfessor\n")
	f.Set("email", "  kari@example.no  ")

	r, verr := FromForm(f, now)
	if verr.Any() {
		t.Fatalf("a submission padded with whitespace was rejected: %v", verr)
	}
	if got := len([]rune(r.FullName)); got != fullNameMax {
		t.Errorf("FullName kept %d runes, want %d — the padding is being counted "+
			"against the limit the member was told about", got, fullNameMax)
	}
	if r.DisplayName != "kari" || r.Title != "Professor" {
		t.Errorf("DisplayName = %q, Title = %q; padding reached the stored record",
			r.DisplayName, r.Title)
	}
	if r.Email != "kari@example.no" {
		t.Errorf("Email = %q, want it trimmed — an address with a leading space "+
			"never matches on a later lookup", r.Email)
	}
}

// A padded, differently-cased stud address is the same stud address. Whichever
// way it is typed, the member has to be told why it is refused rather than
// discovering it when the account stops working after graduation.
func TestStudentEmailIsRefusedThroughPaddingAndCase(t *testing.T) {
	for _, addr := range []string{
		"  kari@stud.ntnu.no ",
		"\tKari.Nordmann@STUD.Ntnu.No\n",
	} {
		f := employeeForm()
		f.Set("email", addr)
		if _, verr := FromForm(f, now); verr["email"] != studEmailMessage {
			t.Errorf("%q: got %q, want the stud-address message", addr, verr["email"])
		}
	}
}

// The lower bound of the name fields, which the existing tests only cover from
// above. Three runes is not much of a name, but it is what the form promises.
func TestNameLengthLowerBounds(t *testing.T) {
	tests := []struct {
		field string
		value string
		valid bool
	}{
		{"fullName", "", false},
		{"fullName", "A", false},
		{"fullName", "Bo", false},
		{"fullName", "Ali", true},
		{"fullName", "Åge", true}, // three runes, five bytes
		// The cost of the three-rune minimum: a two-character Chinese name is a
		// perfectly ordinary one, and it does not fit. The limit is inherited
		// from the previous schema, so this is pinned rather than fixed — but
		// it is what a member with such a name runs into.
		{"fullName", "李雷", false},
		{"displayName", "ab", false},
		{"displayName", "abc", true},
		{"employee.title", "IT", false},
		{"employee.title", "CTO", true},
	}

	for _, tt := range tests {
		t.Run(tt.field+"/"+tt.value, func(t *testing.T) {
			f := employeeForm()
			f.Set(tt.field, tt.value)

			_, verr := FromForm(f, now)
			if accepted := verr[tt.field] == ""; accepted != tt.valid {
				t.Errorf("%s = %q: accepted=%v, want %v", tt.field, tt.value, accepted, tt.valid)
			}
		})
	}
}

// Members are not all called Kari Nordmann. Anything a person can plausibly be
// named has to fit through, which means counting runes and never assuming
// Latin letters — an exchange student whose name is refused by the signup form
// has no way around it.
func TestNamesFromAnyScriptAreAccepted(t *testing.T) {
	names := []string{
		"Bjørn Ærlig Ås",
		"Kari-Anne O'Brien",
		"José María Fernández",
		"Ægir Þórsson",
		"李雷明",
		"Ольга Иванова",
		"Ólafur Jóhann Ólafsson",
		// Four bytes per rune: an emoji is well within the limit by rune count
		// and well past it by byte count.
		strings.Repeat("🐧", displayNameMax),
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			f := employeeForm()
			f.Set("fullName", name)
			f.Set("displayName", name)

			r, verr := FromForm(f, now)
			if verr.Any() {
				t.Fatalf("%q was refused: %v — a member with this name could not "+
					"sign up at all", name, verr)
			}
			if r.FullName != name {
				t.Errorf("FullName = %q, want it stored unchanged", r.FullName)
			}
		})
	}
}

// A pasted essay must be refused, not silently cut down to size. Storing a
// truncated name would be worse than refusing it: the member would never know
// what was kept.
func TestOverlongInputIsRefusedNotTruncated(t *testing.T) {
	huge := strings.Repeat("a", 100_000)

	f := employeeForm()
	f.Set("fullName", huge)

	r, verr := FromForm(f, now)
	if verr["fullName"] == "" {
		t.Fatal("a 100 000 character name was accepted")
	}
	if !strings.Contains(verr["fullName"], "lengre enn 64") {
		t.Errorf("message %q does not tell the member the limit", verr["fullName"])
	}
	if len(r.FullName) != len(huge) {
		t.Errorf("the rejected value came back at %d characters instead of %d; "+
			"a truncated echo would silently rewrite what the member typed",
			len(r.FullName), len(huge))
	}
}

// The address check is a first pass: it catches the shapes a person actually
// mistypes, and FusionAuth is the authority on the rest. The point of pinning
// both lists is that tightening the check stays a deliberate decision.
func TestEmailShapes(t *testing.T) {
	accepted := []string{
		"kari@example.no",
		"kari+ctf@example.no",
		"kari.nordmann@student.example.co.uk",
		"KARI@EXAMPLE.NO",
		"kari@sub.domain.example.no",
		"øystein@eksempel.no", // a non-ASCII local part is a real address
		"k@e.no",
	}
	for _, addr := range accepted {
		t.Run("accepted/"+addr, func(t *testing.T) {
			f := employeeForm()
			f.Set("email", addr)
			if _, verr := FromForm(f, now); verr["email"] != "" {
				t.Errorf("%q was refused (%q); this is an address a member could "+
					"really have", addr, verr["email"])
			}
		})
	}

	rejected := []string{
		"kari",                     // no domain at all
		"kari@",                    // nothing after the @
		"@example.no",              // nothing before it
		"kari@example",             // no dot in the domain
		"kari nordmann@example.no", // a space, most often a stray paste
		"kari@exa mple.no",
		"kari\t@example.no",
	}
	for _, addr := range rejected {
		t.Run("rejected/"+addr, func(t *testing.T) {
			f := employeeForm()
			f.Set("email", addr)
			_, verr := FromForm(f, now)
			if verr["email"] == "" {
				t.Fatalf("%q was accepted as an email address", addr)
			}
			if !strings.Contains(verr["email"], "ser ikke gyldig ut") {
				t.Errorf("message %q is not the malformed-address one", verr["email"])
			}
		})
	}

	// Shapes this first pass lets through. They are not valid addresses, but
	// catching them here would mean writing an address parser; FusionAuth
	// refuses them and the handler shows its message. The list is pinned so
	// that tightening the check is a deliberate edit rather than a surprise —
	// if one of these is now refused locally, move it into the rejected list
	// above.
	lenient := []string{"kari@b@c.no", "kari@.no", "kari@example."}
	for _, addr := range lenient {
		t.Run("left to FusionAuth/"+addr, func(t *testing.T) {
			f := employeeForm()
			f.Set("email", addr)
			if _, verr := FromForm(f, now); verr["email"] != "" {
				t.Errorf("%q is now refused locally (%q); that is an improvement, "+
					"but this test pins the boundary — move the address into the "+
					"rejected list", addr, verr["email"])
			}
		})
	}
}

// The address is stored exactly as typed. FusionAuth decides what counts as
// the same account, and lowercasing here would mean the address in a member's
// welcome email differs from the one they entered.
func TestEmailCaseIsPreserved(t *testing.T) {
	f := employeeForm()
	f.Set("email", "Kari.Nordmann@Example.NO")

	r, verr := FromForm(f, now)
	if verr.Any() {
		t.Fatalf("a mixed-case address was refused: %v", verr)
	}
	if r.Email != "Kari.Nordmann@Example.NO" {
		t.Errorf("Email = %q, want it stored exactly as typed", r.Email)
	}
}

// Whole-number fields come off a form as text, and the browser's number input
// is only advisory — a crafted request carries anything at all.
func TestWholeNumberParsing(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
		want  int
	}{
		{"a plain year", "3", true, 3},
		{"padded with spaces", " 3 ", true, 3},
		{"leading zeroes", "003", true, 3},
		{"an explicit plus", "+3", true, 3},
		{"empty", "", false, 0},
		{"a decimal point", "3.0", false, 0},
		{"a Norwegian decimal comma", "3,5", false, 0},
		{"scientific notation", "1e2", false, 0},
		{"a thousands separator", "1 000", false, 0},
		// Parsed fine, then refused for being out of range — so the number is
		// still handed back. The returned Registration is only meaningful when
		// no errors were reported, and this pins that.
		{"negative", "-1", false, -1},
		{"hexadecimal", "0x3", false, 0},
		// Beyond int64: Atoi reports a range error, which must read as "not a
		// whole number" rather than crashing or wrapping around.
		{"far beyond any integer", "99999999999999999999999", false, 0},
		{"not a number at all", "tre", false, 0},
		{"Arabic-Indic digits", "٣", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := studentForm()
			f.Set("study.year", tt.value)

			r, verr := FromForm(f, now)
			if accepted := verr["study.year"] == ""; accepted != tt.valid {
				t.Fatalf("study.year = %q: accepted=%v (%q), want %v",
					tt.value, accepted, verr["study.year"], tt.valid)
			}
			if r.StudyYear != tt.want {
				t.Errorf("StudyYear = %d, want %d — a rejected field must not leave "+
					"a half-parsed number behind", r.StudyYear, tt.want)
			}
		})
	}
}

// A form can carry the same field twice, and nothing stops a crafted request
// from appending a second value to one the page rendered. Only the first is
// read, so the extra one changes nothing.
func TestRepeatedFieldsUseTheFirstValue(t *testing.T) {
	f := employeeForm()
	f["email"] = []string{"kari@example.no", "kari@stud.ntnu.no"}
	f["fullName"] = []string{"Kari Nordmann", "Ola Nordmann"}
	f["type"] = []string{"employee", "student"}

	r, verr := FromForm(f, now)
	if verr.Any() {
		t.Fatalf("a submission with repeated fields was rejected: %v", verr)
	}
	if r.Email != "kari@example.no" || r.FullName != "Kari Nordmann" {
		t.Errorf("read email=%q fullName=%q, want the first value of each",
			r.Email, r.FullName)
	}
	if r.Type != TypeEmployee {
		t.Errorf("Type = %q, want the first value; a trailing duplicate must not "+
			"change which set of fields is stored", r.Type)
	}
	if _, present := r.ToFusionAuth().Data["study"]; present {
		t.Error("a study block was written from the duplicate type value")
	}
}

// The membership type decides which fields are stored forever, so it is
// matched exactly. Surrounding whitespace is stripped like every other field;
// a different case is a different value.
func TestMembershipTypeMatching(t *testing.T) {
	tests := []struct {
		value string
		valid bool
	}{
		{"student", true},
		{"alumni", true},
		{"employee", true},
		{" student ", true}, // padding is stripped, as everywhere else
		{"student\n", true}, // a trailing newline from a crafted request
		{"Student", false},  // case is significant
		{"STUDENT", false},
		{"students", false},
		{"stud", false},
		{"", false},
		{"styremedlem", false},
	}

	for _, tt := range tests {
		t.Run("type="+tt.value, func(t *testing.T) {
			f := base()
			f.Set("type", tt.value)
			f.Set("study.program", "Kommunikasjonsteknologi")
			f.Set("study.year", "3")
			f.Set("study.expectedFinishYear", "2028")
			f.Set("alumni.joinYear", "2016")
			f.Set("employee.title", "Professor")

			_, verr := FromForm(f, now)
			if accepted := verr["type"] == ""; accepted != tt.valid {
				t.Errorf("type %q: accepted=%v, want %v", tt.value, accepted, tt.valid)
			}
		})
	}
}

// A registration whose type was refused still has ToFusionAuth called on it by
// nothing in production — but the method must not invent a block if it ever
// is. An empty study or employee object would sit in user.data permanently.
func TestToFusionAuthWithoutAType(t *testing.T) {
	r := &Registration{
		FullName: "Kari Nordmann", Email: "kari@example.no", DisplayName: "kari",
		// Everything below belongs to a type that was never chosen.
		Program: "Kommunikasjonsteknologi", StudyYear: 3, JoinYear: 2016, Title: "Professor",
	}

	data := r.ToFusionAuth().Data
	if len(data) != 2 || data["displayName"] != "kari" || data["type"] != "" {
		t.Errorf("data = %v, want only displayName and an empty type", data)
	}
	for _, key := range []string{"study", "alumni", "employee"} {
		if _, present := data[key]; present {
			t.Errorf("a %q block was written for a registration with no type", key)
		}
	}
}

// The finish year is stored as a date, not a year, and every value the form
// accepts has to produce one that parses. A year outside the four-digit range
// would silently change the shape of the string.
func TestFinishDateParsesForTheWholeWindow(t *testing.T) {
	for year := now.Year(); year <= now.Year()+15; year++ {
		f := studentForm()
		f.Set("study.expectedFinishYear", strconv.Itoa(year))

		r, verr := FromForm(f, now)
		if verr.Any() {
			t.Fatalf("finish year %d is inside the window but was rejected: %v", year, verr)
		}
		stored, ok := r.ToFusionAuth().Data["study"].(map[string]any)["expectedFinishYear"].(string)
		if !ok {
			t.Fatalf("expectedFinishYear was not stored as a string for %d", year)
		}
		parsed, err := time.Parse(time.RFC3339, stored)
		if err != nil {
			t.Fatalf("%q does not parse as RFC3339: %v — records written by this "+
				"server must stay comparable with the ones the previous site wrote",
				stored, err)
		}
		if parsed.Year() != year || parsed.Month() != time.June || parsed.Day() != 1 {
			t.Errorf("%q is not the first of June %d", stored, year)
		}
		if parsed.Location() != time.UTC {
			t.Errorf("%q is not in UTC", stored)
		}
	}
}

// The join-year window ends at the current year, and the current year comes
// from the clock rather than a constant. A hardcoded upper bound would start
// refusing this year's members on the first of January.
func TestJoinYearWindowFollowsTheClock(t *testing.T) {
	later := time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)

	f := base()
	f.Set("type", "alumni")
	f.Set("study.program", "Kommunikasjonsteknologi")
	f.Set("alumni.joinYear", "2027")

	if _, verr := FromForm(f, now); verr["alumni.joinYear"] == "" {
		t.Error("2027 was accepted with the clock at 2026; a member cannot have " +
			"joined in the future")
	}
	if _, verr := FromForm(f, later); verr["alumni.joinYear"] != "" {
		t.Errorf("2027 was refused with the clock at 2031 (%q); the window is not "+
			"following the clock", verr["alumni.joinYear"])
	}
}

// Every message is shown next to its own input, so it has to name the field in
// the words the form uses. A generic "invalid" leaves the member guessing
// which of eight inputs to change.
func TestEveryRejectionNamesItsField(t *testing.T) {
	labels := map[string]string{
		"fullName":                 "Fullt navn",
		"displayName":              "Visningsnavn",
		"email":                    "E-post",
		"study.program":            "Studieprogram",
		"study.year":               "Studieår",
		"study.expectedFinishYear": "Forventet ferdig",
	}

	_, verr := FromForm(url.Values{"type": {"student"}}, now)
	for field, label := range labels {
		msg, reported := verr[field]
		if !reported {
			t.Errorf("no error for the missing %q; the form would submit and fail "+
				"upstream instead", field)
			continue
		}
		if !strings.Contains(msg, label) {
			t.Errorf("message for %q is %q, which never mentions %q", field, msg, label)
		}
		if !strings.HasSuffix(msg, ".") {
			t.Errorf("message for %q is %q, which is not a sentence", field, msg)
		}
	}

	_, typeErr := FromForm(url.Values{"type": {"styremedlem"}}, now)
	if msg := typeErr["type"]; !strings.Contains(msg, "Medlemstype") {
		t.Errorf("the type message is %q, which never names the selector", msg)
	}
}
