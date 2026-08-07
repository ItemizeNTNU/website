package users

import (
	"strings"
	"testing"
)

// The alumni record must carry exactly its own shape: a study block holding
// only the program, and a joinYear stored as a number. Anything extra — a
// stray year key, an employee block — would sit in FusionAuth's user.data
// permanently, because nothing ever cleans that store up.
func TestAlumniToFusionAuth(t *testing.T) {
	f := base()
	f.Set("type", "alumni")
	f.Set("study.program", "Kommunikasjonsteknologi")
	f.Set("alumni.joinYear", "2016")

	r, verr := FromForm(f, now)
	if verr.Any() {
		t.Fatalf("a valid alumni registration was rejected: %v", verr)
	}

	data := r.ToFusionAuth().Data
	study, ok := data["study"].(map[string]any)
	if !ok {
		t.Fatal("the study block is missing — an alumnus would be stored without their programme")
	}
	if study["program"] != "Kommunikasjonsteknologi" {
		t.Errorf("study.program = %v, want the submitted programme", study["program"])
	}
	if len(study) != 1 {
		t.Errorf("study block = %v, want only the program key — a year or expectedFinishYear here would be stored in FusionAuth forever", study)
	}
	alumni, ok := data["alumni"].(map[string]any)
	if !ok {
		t.Fatal("the alumni block is missing — the join year would never reach FusionAuth")
	}
	if joinYear, isInt := alumni["joinYear"].(int); !isInt || joinYear != 2016 {
		t.Errorf("alumni.joinYear = %v (%T), want the int 2016 — a string here would break anything that sorts or compares join years", alumni["joinYear"], alumni["joinYear"])
	}
	if _, present := data["employee"]; present {
		t.Error("an employee block leaked into an alumni registration and would be stored permanently")
	}
}

// The expected finish year window slides with the clock: this year up to
// fifteen years out. With now frozen at 2026-03-01 the window is 2026–2041.
func TestFinishYearBounds(t *testing.T) {
	tests := []struct {
		year  string
		valid bool
	}{
		{"2025", false}, // already past — a student cannot finish last year
		{"2026", true},  // finishing this year is fine
		{"2041", true},  // the far edge of the fifteen-year window
		{"2042", false}, // beyond any plausible degree
	}

	for _, tt := range tests {
		t.Run(tt.year, func(t *testing.T) {
			f := base()
			f.Set("type", "student")
			f.Set("study.program", "Kommunikasjonsteknologi")
			f.Set("study.year", "3")
			f.Set("study.expectedFinishYear", tt.year)

			_, verr := FromForm(f, now)
			msg := verr["study.expectedFinishYear"]
			if accepted := msg == ""; accepted != tt.valid {
				t.Errorf("finish year %s: accepted=%v, want %v — a wrong window either blocks a real student or stores a nonsense graduation date", tt.year, accepted, tt.valid)
			}
			if !tt.valid && !strings.Contains(msg, "Forventet ferdig") {
				t.Errorf("rejection message %q does not name the field \"Forventet ferdig\" — the member cannot tell which input to fix", msg)
			}
		})
	}
}

// FusionAuth stores the finish as a date, not a year — the first of June,
// matching what the previous site wrote. A different shape would make old and
// new records incomparable.
func TestFinishDateFormatting(t *testing.T) {
	f := base()
	f.Set("type", "student")
	f.Set("study.program", "Kommunikasjonsteknologi")
	f.Set("study.year", "3")
	f.Set("study.expectedFinishYear", "2026")

	r, verr := FromForm(f, now)
	if verr.Any() {
		t.Fatalf("a valid student registration was rejected: %v", verr)
	}
	study := r.ToFusionAuth().Data["study"].(map[string]any)
	if got := study["expectedFinishYear"]; got != "2026-06-01T00:00:00Z" {
		t.Errorf("expectedFinishYear = %v, want %q — records written by this server must match the date shape the previous site stored", got, "2026-06-01T00:00:00Z")
	}
}

// The length limits count runes, not bytes. Norwegian names are full of
// multi-byte letters, and counting bytes would reject a name like Bjørnstjerne
// long before the limit a member was told about.
func TestRuneLengthLimits(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value string
		valid bool
	}{
		{"fullName of 64 runes accepted", "fullName", strings.Repeat("ø", 64), true},
		{"fullName of 65 runes rejected", "fullName", strings.Repeat("ø", 65), false},
		{"displayName of 2 runes rejected", "displayName", "ab", false},
		{"displayName of 3 Norwegian runes accepted", "displayName", "æøå", true},
		{"displayName of 33 runes rejected", "displayName", strings.Repeat("å", 33), false},
		{"title of 2 runes rejected", "employee.title", "IT", false},
		{"title of 64 runes accepted", "employee.title", strings.Repeat("ø", 64), true},
		{"program of 2 runes rejected", "study.program", "IT", false},
		{"program of 3 runes accepted", "study.program", "øøø", true},
		{"program of 64 runes accepted", "study.program", strings.Repeat("æ", 64), true},
		{"program of 65 runes rejected", "study.program", strings.Repeat("æ", 65), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := base()
			f.Set("type", "employee")
			f.Set("employee.title", "Professor")
			if tt.field == "study.program" {
				f.Set("type", "alumni")
				f.Del("employee.title")
				f.Set("alumni.joinYear", "2016")
			}
			f.Set(tt.field, tt.value)

			_, verr := FromForm(f, now)
			if accepted := verr[tt.field] == ""; accepted != tt.valid {
				t.Errorf("%s = %d runes: accepted=%v, want %v — counting bytes instead of runes would reject Norwegian names well short of the documented limit", tt.field, len([]rune(tt.value)), accepted, tt.valid)
			}
		})
	}
}

func TestStudyYearBounds(t *testing.T) {
	tests := []struct {
		name  string
		year  string
		valid bool
	}{
		{"zero rejected", "0", false},
		{"first year accepted", "1", true},
		{"hundredth year accepted", "100", true},
		{"beyond the cap rejected", "101", false},
		{"fractional year rejected as not a whole number", "2.5", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := base()
			f.Set("type", "student")
			f.Set("study.program", "Kommunikasjonsteknologi")
			f.Set("study.year", tt.year)
			f.Set("study.expectedFinishYear", "2028")

			_, verr := FromForm(f, now)
			msg := verr["study.year"]
			if accepted := msg == ""; accepted != tt.valid {
				t.Errorf("study year %q: accepted=%v, want %v — a wrong bound stores impossible study years in FusionAuth", tt.year, accepted, tt.valid)
			}
			if tt.year == "2.5" && !strings.Contains(msg, "heltall") {
				t.Errorf("message for a fractional year = %q, want it to say the field needs a whole number (heltall)", msg)
			}
		})
	}
}

// With no type chosen, OneOf reports the type field and returns "", so the
// switch in FromForm matches no branch. The member should see one clear error
// on the type selector — not a wall of complaints about fields the form never
// even showed them.
func TestEmptyTypeIsRefused(t *testing.T) {
	_, verr := FromForm(base(), now)
	if verr["type"] == "" {
		t.Error("a registration with no membership type was accepted — it would reach FusionAuth with no type at all")
	}
	for _, field := range []string{
		"study.program", "study.year", "study.expectedFinishYear",
		"alumni.joinYear", "employee.title",
	} {
		if msg, present := verr[field]; present {
			t.Errorf("type-specific error for %q (%q) reported without a chosen type — the member would be told to fill fields the form never showed", field, msg)
		}
	}
}
