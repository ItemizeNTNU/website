// Package users handles member registration.
package users

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/ItemizeNTNU/website/internal/fusionauth"
	"github.com/ItemizeNTNU/website/internal/validate"
)

// Membership types.
const (
	TypeStudent  = "student"
	TypeAlumni   = "alumni"
	TypeEmployee = "employee"
)

// FoundedYear is the earliest year anyone can claim to have joined.
const FoundedYear = 2014

// Field limits, matching the previous schema so nothing that used to be
// accepted stops being accepted.
const (
	fullNameMin, fullNameMax       = 3, 64
	displayNameMin, displayNameMax = 3, 32
	programMin, programMax         = 3, 64
	titleMin, titleMax             = 3, 64
	studyYearMin, studyYearMax     = 1, 100
)

// studEmailSuffix is the address we refuse. The message is preserved verbatim,
// including its reasoning — a member who reads it understands why rather than
// just being blocked.
const studEmailSuffix = "@stud.ntnu.no"

const studEmailMessage = "Vennligst ikke bruk din stud e-post adresse, " +
	"da du mister tilgang til denne etter fullført utdannelse."

// emailMax is the longest address that can actually be delivered: RFC 5321
// caps the whole envelope path at 254 characters. Every other field here has a
// ceiling; without one the address is forwarded verbatim to FusionAuth, which
// sends a password-setting email to it.
const emailMax = 254

// Registration is a validated signup, ready to send to FusionAuth.
type Registration struct {
	FullName    string
	Email       string
	DisplayName string
	Type        string

	// Only the fields belonging to Type are populated.
	Program            string
	StudyYear          int
	ExpectedFinishYear int
	JoinYear           int
	Title              string
}

// FromForm validates a submitted registration.
//
// The conditional fields are the reason this is hand-written. Inactive fields
// must be *dropped*, not merely ignored: the result goes straight into
// FusionAuth's user.data, and an alumnus arriving with an empty study year
// would store that emptiness permanently. Reading only the fields belonging to
// the chosen type makes that structural rather than something to remember.
//
// It also means the server does not trust the form's own idea of which fields
// matter. The markup hides the inactive ones with CSS, but a hidden input
// still submits — so a crafted request carrying every field at once is handled
// by simply not reading the ones that do not apply.
func FromForm(form url.Values, now time.Time) (*Registration, validate.Errors) {
	var e validate.Errors
	r := &Registration{}

	r.FullName = e.Text("fullName", "Fullt navn", form.Get("fullName"), fullNameMin, fullNameMax)
	r.DisplayName = e.Text("displayName", "Visningsnavn", form.Get("displayName"),
		displayNameMin, displayNameMax)
	r.Email = validateEmail(&e, form.Get("email"))
	r.Type = e.OneOf("type", "Medlemstype", form.Get("type"),
		TypeStudent, TypeAlumni, TypeEmployee)

	switch r.Type {
	case TypeStudent:
		r.Program = e.Text("study.program", "Studieprogram", form.Get("study.program"),
			programMin, programMax)
		r.StudyYear = e.Int("study.year", "Studieår", form.Get("study.year"),
			studyYearMin, studyYearMax)
		r.ExpectedFinishYear = e.Int("study.expectedFinishYear", "Forventet ferdig år",
			form.Get("study.expectedFinishYear"), now.Year(), now.Year()+15)

	case TypeAlumni:
		r.Program = e.Text("study.program", "Studieprogram", form.Get("study.program"),
			programMin, programMax)
		r.JoinYear = e.Int("alumni.joinYear", "Medlemsår", form.Get("alumni.joinYear"),
			FoundedYear, now.Year())

	case TypeEmployee:
		r.Title = e.Text("employee.title", "Tittel", form.Get("employee.title"),
			titleMin, titleMax)
	}

	return r, e
}

// validateEmail checks the address and refuses student addresses.
func validateEmail(e *validate.Errors, raw string) string {
	addr := strings.TrimSpace(raw)
	if addr == "" {
		e.Add("email", "E-postadresse må fylles ut.")
		return addr
	}
	if len([]rune(addr)) > emailMax {
		e.Add("email", fmt.Sprintf("E-postadresse kan ikke være lengre enn %d tegn.", emailMax))
		return addr
	}
	local, domain, ok := strings.Cut(addr, "@")
	if !ok || local == "" || domain == "" || !strings.Contains(domain, ".") ||
		strings.IndexFunc(addr, notAllowedInEmail) >= 0 {
		e.Add("email", "E-postadressen ser ikke gyldig ut.")
		return addr
	}
	// A student address stops working the moment they graduate, which is
	// exactly when an alumni membership becomes interesting.
	if strings.HasSuffix(strings.ToLower(addr), studEmailSuffix) {
		e.Add("email", studEmailMessage)
	}
	return addr
}

// notAllowedInEmail reports the characters an address must never contain.
//
// The check used to name a space and a tab, which left \n, \r and NUL to pass
// straight through. That matters more here than in an ordinary field: the
// address is handed to FusionAuth, which builds an email from it, so a line
// break is not merely malformed — it is a character with meaning to everything
// downstream. IsControl covers U+0000–U+001F and U+007F; IsSpace covers the
// separators TrimSpace leaves behind in the middle of the string, including
// the non-breaking space a paste from a PDF carries.
func notAllowedInEmail(r rune) bool {
	return unicode.IsControl(r) || unicode.IsSpace(r)
}

// ToFusionAuth builds the user record.
//
// The data map contains only the keys for the chosen membership type. The
// previous implementation built a map with every branch present and then
// walked it removing empty objects; constructing exactly what should be sent
// removes the need for that pass, and with it the chance of it missing one.
func (r *Registration) ToFusionAuth() fusionauth.User {
	data := map[string]any{
		"displayName": r.DisplayName,
		"type":        r.Type,
	}

	switch r.Type {
	case TypeStudent:
		data["study"] = map[string]any{
			"program": r.Program,
			"year":    r.StudyYear,
			// Stored as a date rather than a year, matching what the previous
			// site wrote: the first of June, roughly when a Norwegian academic
			// year ends.
			"expectedFinishYear": finishDate(r.ExpectedFinishYear),
		}
	case TypeAlumni:
		data["study"] = map[string]any{"program": r.Program}
		data["alumni"] = map[string]any{"joinYear": r.JoinYear}
	case TypeEmployee:
		data["employee"] = map[string]any{"title": r.Title}
	}

	return fusionauth.User{
		Email:    r.Email,
		FullName: r.FullName,
		Data:     data,
	}
}

func finishDate(year int) string {
	return fmt.Sprintf("%04d-06-01T00:00:00Z", year)
}
