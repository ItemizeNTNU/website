package events

import (
	"math"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ItemizeNTNU/website/internal/timefmt"
)

// formWith returns a submission that is valid apart from the one field named.
// Every boundary case below varies a single field, so a failure is never
// ambiguous about which rule fired.
func formWith(t *testing.T, field, value string) url.Values {
	t.Helper()
	form := validForm()
	form.Set(field, value)
	return form
}

// ── Required text fields ──────────────────────────────────────────────────

// The limits are copied from the previous server's schema. They matter in both
// directions: a lower bound that is too strict rejects an event the board has
// already announced elsewhere, and an upper bound that is too loose lets a
// paste of an entire mail thread into the info field break the listing layout.
func TestTextFieldBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   string
		wantErr string
	}{
		// Navn: 3–50.
		{"a name one character below the minimum", "name", "CT", "Navn må være minst 3 tegn."},
		{"a name at the minimum is accepted", "name", "CTF", ""},
		{"a name at the maximum is accepted", "name", strings.Repeat("a", nameMax), ""},
		{"a name one character over the maximum", "name", strings.Repeat("a", nameMax+1), "Navn kan ikke være lengre enn 50 tegn."},
		{"an empty name reports the required message, not the length one", "name", "", "Navn må fylles ut."},
		{
			// Whitespace is trimmed before the emptiness check, so a field
			// containing only spaces is empty rather than "long enough".
			"a name of nothing but spaces counts as empty",
			"name", "     ", "Navn må fylles ut.",
		},
		{"a name padded with spaces is measured after trimming", "name", "  CT  ", "Navn må være minst 3 tegn."},
		{
			// Measured in runes, not bytes. Fifty æ is a hundred bytes; a byte
			// count would reject a perfectly ordinary Norwegian title.
			"fifty Norwegian letters fit exactly",
			"name", strings.Repeat("æ", nameMax), "",
		},
		{"fifty-one Norwegian letters do not", "name", strings.Repeat("ø", nameMax+1), "Navn kan ikke være lengre enn 50 tegn."},
		{"an emoji title is measured in runes too", "name", "🇳🇴 Pizza 🍕", ""},

		// Hvor: 3–200.
		{"a venue below the minimum", "location.name", "A2", "Hvor må være minst 3 tegn."},
		{"a venue at the minimum", "location.name", "A24", ""},
		{"a venue at the maximum", "location.name", strings.Repeat("å", locationMax), ""},
		{"a venue over the maximum", "location.name", strings.Repeat("å", locationMax+1), "Hvor kan ikke være lengre enn 200 tegn."},
		{"a missing venue", "location.name", "", "Hvor må fylles ut."},

		// Info: 3–2000.
		{"info below the minimum", "info", "Ok", "Info må være minst 3 tegn."},
		{"info at the minimum", "info", "Nei", ""},
		{"info at the maximum", "info", strings.Repeat("é", infoMax), ""},
		{"info over the maximum", "info", strings.Repeat("é", infoMax+1), "Info kan ikke være lengre enn 2000 tegn."},
		{"missing info", "info", "", "Info må fylles ut."},
		{"newlines in info are ordinary characters", "info", "Linje 1\nLinje 2\n\nHilsen styret", ""},

		// CTF-navn is optional: no minimum, only a ceiling.
		{"no CTF is a normal event", "ctf.name", "", ""},
		{"a one-character CTF name is fine because the field is optional", "ctf.name", "X", ""},
		{"a CTF name at the maximum", "ctf.name", strings.Repeat("c", ctfNameMax), ""},
		{"a CTF name over the maximum", "ctf.name", strings.Repeat("c", ctfNameMax+1), "CTF-navn kan ikke være lengre enn 200 tegn."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, verr := FromForm(formWith(t, tt.field, tt.value))
			if got := verr[tt.field]; got != tt.wantErr {
				t.Errorf("%s = %q, want %q — the board sees this message next to the field, so a wrong one sends them looking at the wrong problem",
					tt.field, got, tt.wantErr)
			}
		})
	}
}

// The stored value is the trimmed one. Saving the padding would put it into
// the page title, the Discord announcement and the calendar feed.
func TestAcceptedTextIsStoredTrimmed(t *testing.T) {
	form := validForm()
	form.Set("name", "  Pizza og CTF  ")
	form.Set("location.name", "\tSavannen\n")
	form.Set("info", "  Ta med laptop.  ")
	form.Set("ctf.name", "  Julekalender  ")

	ev, verr := FromForm(form)
	if verr.Any() {
		t.Fatalf("padded but otherwise valid input was rejected: %v", verr)
	}
	if ev.Name != "Pizza og CTF" {
		t.Errorf("name = %q, want it trimmed — the padding would end up in the page title and the Discord post", ev.Name)
	}
	if ev.Location.Name != "Savannen" {
		t.Errorf("location.name = %q, want it trimmed", ev.Location.Name)
	}
	if ev.Info != "Ta med laptop." {
		t.Errorf("info = %q, want it trimmed", ev.Info)
	}
	if ev.CTF.Name != "Julekalender" {
		t.Errorf("ctf.name = %q, want it trimmed", ev.CTF.Name)
	}
}

// An empty submission has to annotate every required field at once. The
// previous server stopped at the first, which meant one round trip per field.
func TestEmptySubmissionAnnotatesEveryRequiredField(t *testing.T) {
	_, verr := FromForm(url.Values{})

	for _, field := range []string{"name", "location.name", "info", "duration", "date"} {
		if _, ok := verr[field]; !ok {
			t.Errorf("no message for %q on a wholly empty submission; the board would have to guess", field)
		}
	}
	// The optional fields must stay quiet, or the form is a wall of red for
	// things nobody has to fill in.
	for _, field := range []string{"ctf.name", "ctf.url", "location.url", "register_url"} {
		if msg, ok := verr[field]; ok {
			t.Errorf("optional field %q was flagged with %q on an empty submission", field, msg)
		}
	}
}

// ── Duration ──────────────────────────────────────────────────────────────

func TestDurationBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr string
		want    float64
	}{
		{"a whole number of hours", "3", "", 3},
		{"zero is allowed", "0", "", 0},
		{
			// A Norwegian keyboard produces a comma, and rejecting it is a
			// papercut the board would hit constantly.
			"a comma decimal separator is accepted",
			"2,5", "", 2.5,
		},
		{"a full stop separator is accepted too", "2.5", "", 2.5},
		{"padding is trimmed before parsing", "  4  ", "", 4},
		// An out-of-range number is still returned, so the re-rendered form
		// shows what was typed; only an unparseable one falls back to zero.
		{"a negative duration is rejected", "-1", "Varighet kan ikke være mindre enn 0.", -1},
		{"the maximum of a week is accepted", "168", "", durationMax},
		{
			// The previous schema had no ceiling, so a typo pinned the event to
			// the top of the listing for years.
			"an hour past the maximum is rejected",
			"169", "Varighet kan ikke være større enn 168.", 169,
		},
		{"a typo of several thousand hours is rejected", "2000", "Varighet kan ikke være større enn 168.", 2000},
		{"words are not a duration", "ikke tall", "Varighet må være et tall.", 0},
		{"an empty duration is required, not defaulted to zero", "", "Varighet må fylles ut.", 0},
		{"whitespace only is treated as empty", "   ", "Varighet må fylles ut.", 0},
		{
			// Scientific notation parses, so the ceiling is what catches it.
			"scientific notation is parsed and then rejected by the ceiling",
			"1e3", "Varighet kan ikke være større enn 168.", 1000,
		},
		{"infinity is above the ceiling", "Inf", "Varighet kan ikke være større enn 168.", math.Inf(1)},
		{"negative infinity is below the floor", "-Inf", "Varighet kan ikke være mindre enn 0.", math.Inf(-1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, verr := FromForm(formWith(t, "duration", tt.value))
			if got := verr["duration"]; got != tt.wantErr {
				t.Errorf("duration %q reported %q, want %q", tt.value, got, tt.wantErr)
			}
			if ev.Duration != tt.want {
				t.Errorf("duration %q parsed to %v, want %v — this value decides when the event ends", tt.value, ev.Duration, tt.want)
			}
		})
	}
}

// "NaN" parses as a float, and NaN compares false against both bounds, so it
// slips through untouched. ComputeEnd then produces an end time in the
// eighteenth century, which reads as an event that finished long ago.
//
// This pins the current behaviour rather than endorsing it: it is a real hole
// in the validation, reported alongside these tests. If the range check ever
// learns about NaN, this test is the one to delete.
func TestDurationNaNIsCurrentlyAccepted(t *testing.T) {
	ev, verr := FromForm(formWith(t, "duration", "NaN"))

	if msg, ok := verr["duration"]; ok {
		t.Skipf("NaN is now rejected with %q — the hole this test documented has been closed", msg)
	}
	if !math.IsNaN(ev.Duration) {
		t.Fatalf("duration = %v, want NaN", ev.Duration)
	}
	if end := ev.ComputeEnd(); !end.Before(ev.Date) {
		t.Errorf("end = %v, expected the nonsensical value NaN produces; the consequence of accepting it is what makes this a bug", end)
	}
}

// ── Date ──────────────────────────────────────────────────────────────────

// The date arrives from <input type="datetime-local">, which submits local
// wall-clock time with no zone at all. Reading it as anything but Oslo time
// puts every event an hour or two off.
func TestDateParsing(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr string
		want    time.Time
	}{
		{
			"what the browser normally submits",
			"2026-09-01T17:15", "",
			time.Date(2026, 9, 1, 15, 15, 0, 0, time.UTC),
		},
		{
			// Some browsers include seconds when the step attribute allows it.
			"a value carrying seconds",
			"2026-09-01T17:15:30", "",
			time.Date(2026, 9, 1, 15, 15, 30, 0, time.UTC),
		},
		{
			// Nothing forbids a date in the past: the board edits events after
			// they have happened, to fix a typo or add a writeup link.
			"a date in the past is accepted",
			"2019-11-05T17:00", "",
			time.Date(2019, 11, 5, 16, 0, 0, 0, time.UTC),
		},
		{
			// 02:30 on the spring transition does not exist in Oslo. Go maps it
			// forward rather than failing, so the event lands at 03:30 CEST —
			// an hour later than typed, but a real instant.
			"a wall-clock time inside the spring-forward gap is moved forward",
			"2026-03-29T02:30", "",
			time.Date(2026, 3, 29, 1, 30, 0, 0, time.UTC),
		},
		{
			// 02:30 on the autumn transition happens twice. Go resolves it to
			// the later, winter-time occurrence.
			"an ambiguous autumn wall-clock time resolves to winter time",
			"2026-10-25T02:30", "",
			time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC),
		},
		{"an empty date", "", "Når må fylles ut med gyldig dato og tid.", time.Time{}},
		{"prose instead of a date", "i morgen", "Når må fylles ut med gyldig dato og tid.", time.Time{}},
		{"a day that does not exist", "2026-02-30T10:00", "Når må fylles ut med gyldig dato og tid.", time.Time{}},
		{"a month that does not exist", "2026-13-01T10:00", "Når må fylles ut med gyldig dato og tid.", time.Time{}},
		{"unpadded components are not accepted", "2026-9-1T17:15", "Når må fylles ut med gyldig dato og tid.", time.Time{}},
		{"a date only, with no time", "2026-09-01", "Når må fylles ut med gyldig dato og tid.", time.Time{}},
		{
			// Unlike the text fields, the date is not trimmed before parsing,
			// so a value pasted with a leading space is rejected. Pinned
			// because it is a surprising asymmetry, not because it is desirable.
			"a leading space is not trimmed away",
			" 2026-09-01T17:15", "Når må fylles ut med gyldig dato og tid.", time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, verr := FromForm(formWith(t, "date", tt.value))
			if got := verr["date"]; got != tt.wantErr {
				t.Errorf("date %q reported %q, want %q", tt.value, got, tt.wantErr)
			}
			if !ev.Date.Equal(tt.want) {
				t.Errorf("date %q parsed to %v, want %v — a misread offset shows the event at the wrong hour everywhere it appears",
					tt.value, ev.Date.UTC(), tt.want)
			}
		})
	}
}

// The parsed instant has to be Oslo wall-clock time, which is what the board
// typed and what the site renders back to them. Round-tripping through the
// form value is the proof: what goes into the field comes back out unchanged.
func TestDateRoundTripsThroughTheForm(t *testing.T) {
	for _, when := range []time.Time{
		time.Date(2026, 1, 15, 19, 0, 0, 0, timefmt.Oslo),   // winter, +01:00
		time.Date(2026, 7, 2, 17, 15, 0, 0, timefmt.Oslo),   // summer, +02:00
		time.Date(2026, 12, 31, 23, 59, 0, 0, timefmt.Oslo), // the year boundary
	} {
		ev, verr := FromForm(formWith(t, "date", timefmt.FormValue(when)))
		if verr.Any() {
			t.Fatalf("the site's own rendering of %v was rejected on resubmission: %v", when, verr)
		}
		if !ev.Date.Equal(when) {
			t.Errorf("re-submitting the edit form moved the event from %v to %v", when, ev.Date)
		}
	}
}

// ── URLs ──────────────────────────────────────────────────────────────────

// All three URL fields are rendered as links on a public page, so all three
// need the same guard. Only location.url was covered before, and a rule that
// applies to one field and not its neighbours is exactly the kind of gap that
// lets a javascript: link onto the site.
func TestEveryURLFieldIsValidated(t *testing.T) {
	fields := []string{"location.url", "register_url", "ctf.url"}

	tests := []struct {
		name    string
		value   string
		wantErr string
	}{
		{"empty is allowed everywhere", "", ""},
		{"an ordinary https URL", "https://itemize.no/arrangement?id=1", ""},
		{"a URL with Norwegian letters in the path", "https://itemize.no/påmelding", ""},
		{"surrounding whitespace is trimmed rather than rejected", "  https://itemize.no  ", ""},
		{
			// A pasted URL really can arrive with a non-breaking space in
			// front of it; trimming has to cover more than U+0020.
			"a leading non-breaking space is trimmed",
			" https://itemize.no", "",
		},
		{"plain http is refused", "http://itemize.no", "the URL needs to start with https://"},
		{"a bare hostname has no scheme", "itemize.no", "the URL needs to start with https://"},
		{
			// The reason the rule exists: the value is interpolated into an
			// href on a page the public reads.
			"a javascript: URL is refused",
			"javascript:alert(document.cookie)", "the URL needs to start with https://",
		},
		{"a data: URL is refused", "data:text/html;base64,PHNjcmlwdD4=", "the URL needs to start with https://"},
		{"a scheme-relative URL is refused", "//evil.example/x", "the URL needs to start with https://"},
		{"an https-looking prefix inside another scheme is refused", "javascript:https://itemize.no", "the URL needs to start with https://"},
		{
			// The prefix test is byte-exact, so a shouting paste is refused
			// even though the URL itself is fine.
			"an upper-case scheme is refused",
			"HTTPS://ITEMIZE.NO", "the URL needs to start with https://",
		},
		{"an embedded space", "https://itemize.no/a b", "the URL can not include whitespace"},
		{"an embedded tab", "https://itemize.no/a\tb", "the URL can not include whitespace"},
		{"an embedded newline", "https://itemize.no/a\nb", "the URL can not include whitespace"},
		{"an embedded non-breaking space", "https://itemize.no/a b", "the URL can not include whitespace"},
		{"an embedded line separator", "https://itemize.no/a b", "the URL can not include whitespace"},
		{
			// Nothing checks that a host follows the scheme, so this passes and
			// renders as a dead link. Pinned as the current behaviour, not as
			// a rule worth keeping.
			"a bare scheme with no host is accepted",
			"https://", "",
		},
	}

	for _, field := range fields {
		for _, tt := range tests {
			t.Run(field+"/"+tt.name, func(t *testing.T) {
				_, verr := FromForm(formWith(t, field, tt.value))
				if got := verr[field]; got != tt.wantErr {
					t.Errorf("%s = %q for %q, want %q — an unvalidated URL field is a link the public clicks",
						field, got, tt.value, tt.wantErr)
				}
			})
		}
	}
}

// A rejected URL is still returned so the re-rendered form shows the board
// what they typed rather than blanking the field.
func TestRejectedURLIsEchoedBack(t *testing.T) {
	ev, verr := FromForm(formWith(t, "register_url", "http://itemize.no/påmelding"))
	if !verr.Any() {
		t.Fatal("an http URL was accepted")
	}
	if ev.RegisterURL != "http://itemize.no/påmelding" {
		t.Errorf("register_url = %q, want the submitted value echoed back so the form can be corrected rather than retyped", ev.RegisterURL)
	}
}

// ── Checkboxes and the fields the form must not control ───────────────────

func TestCheckboxes(t *testing.T) {
	tests := []struct {
		name                 string
		hidden, discord      string
		wantHidden, wantSync bool
	}{
		{"neither box ticked", "", "", false, false},
		{"only the Discord box", "", "on", false, true},
		{"only the hidden box", "on", "", true, false},
		{"both boxes: hidden is the stronger statement", "on", "on", true, false},
		{
			// A checkbox submits its value attribute, whatever that is; the
			// test is presence, not truthiness. "0" here means the box was
			// ticked, however odd that reads.
			"any value at all counts as ticked",
			"0", "false", true, false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := validForm()
			form.Set("hidden", tt.hidden)
			form.Set("discord", tt.discord)

			ev, verr := FromForm(form)
			if verr.Any() {
				t.Fatalf("unexpected validation errors: %v", verr)
			}
			if ev.Hidden != tt.wantHidden || ev.Discord != tt.wantSync {
				t.Errorf("hidden=%v discord=%v, want hidden=%v discord=%v — announcing a draft event to the guild is not recoverable, the message is already sent",
					ev.Hidden, ev.Discord, tt.wantHidden, tt.wantSync)
			}
		})
	}
}

// The identifier, the check-in code and the Discord event id are carried over
// from the stored event by the service. FromForm must not populate them at
// all, or a crafted submission could reassign a printed QR code or detach an
// event from its Discord counterpart before the service ever sees it.
func TestFromFormIgnoresFieldsTheSubmissionMustNotControl(t *testing.T) {
	form := validForm()
	form.Set("_id", "5f2b0a1e0f3f4c7a9b218a4d")
	form.Set("id", "5f2b0a1e0f3f4c7a9b218a4d")
	form.Set("check_in.code", "angripers-kode")
	form.Set("code", "angripers-kode")
	form.Set("discordEventId", "999")
	form.Set("end", "2026-09-01T23:00")
	form.Set("created", "2000-01-01T00:00")

	ev, verr := FromForm(form)
	if verr.Any() {
		t.Fatalf("unexpected validation errors: %v", verr)
	}

	if !ev.ID.IsZero() {
		t.Errorf("ID = %v, want zero — a submitted identifier would let one event's form overwrite another", ev.ID)
	}
	if ev.CheckIn.Code != "" {
		t.Errorf("check-in code = %q, want empty — every QR code already handed out would stop working", ev.CheckIn.Code)
	}
	if ev.DiscordEventID != "" {
		t.Errorf("discordEventId = %q, want empty — a submitted value could point at somebody else's guild event", ev.DiscordEventID)
	}
	if !ev.End.IsZero() {
		t.Errorf("end = %v, want zero; it is derived from the duration, not submitted", ev.End)
	}
	if !ev.Created.IsZero() {
		t.Errorf("created = %v, want zero; it is stamped on insert, not submitted", ev.Created)
	}
	if ev.SchemaVersion != nil {
		t.Errorf("__v = %v, want nil", ev.SchemaVersion)
	}
}

// Several fields failing at once must not lose any of the messages, and the
// event returned alongside them must still carry the values that did parse, so
// the re-rendered form is not wiped.
func TestPartialFailureKeepsTheValidValues(t *testing.T) {
	form := validForm()
	form.Set("name", "x")
	form.Set("duration", "9999")
	form.Set("date", "tull")

	ev, verr := FromForm(form)
	if len(verr) != 3 {
		t.Fatalf("errors = %v, want exactly the three broken fields", verr)
	}
	if ev.Location.Name != "Savannen" {
		t.Errorf("location.name = %q, want the valid value preserved so the board does not retype it", ev.Location.Name)
	}
	if ev.Info != "Ta med laptop." {
		t.Errorf("info = %q, want the valid value preserved", ev.Info)
	}
	// First() sorts, so the message shown by the JSON endpoints is the same on
	// every identical submission rather than whichever the map yielded.
	if got, want := verr.First(), verr["date"]; got != want {
		t.Errorf("First() = %q, want %q — an error message that changes between identical requests is impossible to support", got, want)
	}
}
