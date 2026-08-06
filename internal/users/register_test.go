package users

import (
	"net/url"
	"testing"
	"time"
)

var now = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func base() url.Values {
	return url.Values{
		"fullName":    {"Kari Nordmann"},
		"email":       {"kari@example.no"},
		"displayName": {"kari"},
	}
}

func TestStudentRegistration(t *testing.T) {
	f := base()
	f.Set("type", "student")
	f.Set("study.program", "Kommunikasjonsteknologi")
	f.Set("study.year", "3")
	f.Set("study.expectedFinishYear", "2028")

	r, verr := FromForm(f, now)
	if verr.Any() {
		t.Fatalf("valid student rejected: %v", verr)
	}

	data := r.ToFusionAuth().Data
	study, ok := data["study"].(map[string]any)
	if !ok {
		t.Fatal("study block missing")
	}
	if study["year"] != 3 {
		t.Errorf("year = %v", study["year"])
	}
	// The previous site stored a date, not a year — the first of June, roughly
	// when a Norwegian academic year ends.
	if got := study["expectedFinishYear"]; got != "2028-06-01T00:00:00Z" {
		t.Errorf("expectedFinishYear = %v", got)
	}
	// A student is not an alumnus or an employee.
	if _, present := data["alumni"]; present {
		t.Error("alumni block leaked into a student registration")
	}
	if _, present := data["employee"]; present {
		t.Error("employee block leaked into a student registration")
	}
}

// The inactive fields are hidden with CSS, but a hidden input still submits.
// The server must read only what belongs to the chosen type — otherwise an
// alumnus is stored carrying an empty study year forever.
func TestInactiveFieldsAreDroppedNotStored(t *testing.T) {
	f := base()
	f.Set("type", "alumni")
	f.Set("study.program", "Kommunikasjonsteknologi")
	f.Set("alumni.joinYear", "2016")
	// A crafted submission carrying every field at once.
	f.Set("study.year", "3")
	f.Set("study.expectedFinishYear", "2028")
	f.Set("employee.title", "Professor")

	r, verr := FromForm(f, now)
	if verr.Any() {
		t.Fatalf("valid alumni rejected: %v", verr)
	}
	if r.StudyYear != 0 || r.Title != "" {
		t.Errorf("fields from other types were read: year=%d title=%q", r.StudyYear, r.Title)
	}

	data := r.ToFusionAuth().Data
	if _, present := data["employee"]; present {
		t.Error("employee block leaked into an alumni registration")
	}
	study := data["study"].(map[string]any)
	if _, present := study["year"]; present {
		t.Error("study year leaked into an alumni registration")
	}
	if data["alumni"].(map[string]any)["joinYear"] != 2016 {
		t.Errorf("joinYear = %v", data["alumni"])
	}
}

func TestEmployeeRegistration(t *testing.T) {
	f := base()
	f.Set("type", "employee")
	f.Set("employee.title", "Førsteamanuensis")

	r, verr := FromForm(f, now)
	if verr.Any() {
		t.Fatalf("valid employee rejected: %v", verr)
	}
	data := r.ToFusionAuth().Data
	if _, present := data["study"]; present {
		t.Error("study block leaked into an employee registration")
	}
	if data["employee"].(map[string]any)["title"] != "Førsteamanuensis" {
		t.Errorf("title = %v", data["employee"])
	}
}

// The whole point of the rule, with the reasoning preserved in the message.
func TestStudentEmailIsRefused(t *testing.T) {
	for _, addr := range []string{
		"kari@stud.ntnu.no",
		"KARI@STUD.NTNU.NO",
		"Kari.Nordmann@Stud.Ntnu.No",
	} {
		f := base()
		f.Set("type", "employee")
		f.Set("employee.title", "Professor")
		f.Set("email", addr)

		_, verr := FromForm(f, now)
		if verr["email"] != studEmailMessage {
			t.Errorf("%s: got %q, want the stud-address message", addr, verr["email"])
		}
	}
}

// An ordinary NTNU address is fine; only the student one is refused.
func TestStaffNTNUEmailIsAccepted(t *testing.T) {
	f := base()
	f.Set("type", "employee")
	f.Set("employee.title", "Professor")
	f.Set("email", "kari@ntnu.no")

	if _, verr := FromForm(f, now); verr.Any() {
		t.Errorf("a staff address was refused: %v", verr)
	}
}

func TestInvalidEmails(t *testing.T) {
	for _, addr := range []string{"", "kari", "kari@", "@example.no", "kari@example", "a b@c.no"} {
		f := base()
		f.Set("type", "employee")
		f.Set("employee.title", "Professor")
		f.Set("email", addr)

		if _, verr := FromForm(f, now); verr["email"] == "" {
			t.Errorf("%q was accepted as an email address", addr)
		}
	}
}

func TestJoinYearBounds(t *testing.T) {
	tests := map[string]bool{
		"2013": false, // before the organisation existed
		"2014": true,  // the founding year
		"2026": true,  // this year
		"2027": false, // the future
	}
	for year, valid := range tests {
		f := base()
		f.Set("type", "alumni")
		f.Set("study.program", "Kommunikasjonsteknologi")
		f.Set("alumni.joinYear", year)

		_, verr := FromForm(f, now)
		if got := verr["alumni.joinYear"] == ""; got != valid {
			t.Errorf("joinYear %s: accepted=%v, want %v", year, got, valid)
		}
	}
}

func TestUnknownTypeIsRefused(t *testing.T) {
	f := base()
	f.Set("type", "styremedlem")
	if _, verr := FromForm(f, now); verr["type"] == "" {
		t.Error("an unknown membership type was accepted")
	}
}

func TestMissingRequiredFieldsAreAllReported(t *testing.T) {
	_, verr := FromForm(url.Values{"type": {"student"}}, now)
	for _, field := range []string{"fullName", "email", "displayName", "study.program"} {
		if _, ok := verr[field]; !ok {
			t.Errorf("no error for %q; got %v", field, verr)
		}
	}
}
