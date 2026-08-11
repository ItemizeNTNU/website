package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ItemizeNTNU/website/internal/timefmt"
)

// ── Accessors the templates depend on ─────────────────────────────────────

// The template builds edit links and form actions out of HexID. A zero
// identifier is what an unsaved event has, and rendering it as
// "000000000000000000000000" would produce a link to an event that does not
// exist rather than an empty one the template can test for.
func TestHexID(t *testing.T) {
	saved := bson.NewObjectID()

	tests := []struct {
		name string
		ev   Event
		want string
	}{
		{"unsaved event has no identifier to render", Event{}, ""},
		{"a saved event renders its hex form", Event{ID: saved}, saved.Hex()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ev.HexID(); got != tt.want {
				t.Errorf("HexID() = %q, want %q — the edit link on the board page is built from this", got, tt.want)
			}
		})
	}
}

// Past drives whether an event is styled as finished and whether the check-in
// page still accepts scans. A document written before `end` existed decodes
// with a zero time, and treating that as "ended in year 1" would mark every
// legacy event as over.
func TestPast(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		end  time.Time
		want bool
	}{
		{"an event with no end recorded is not treated as finished", time.Time{}, false},
		{"an event that ended an hour ago is finished", now.Add(-time.Hour), true},
		{"an event ending in an hour is not finished", now.Add(time.Hour), false},
		{"an event that ended a decade ago is finished", now.AddDate(-10, 0, 0), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Event{End: tt.end}).Past(); got != tt.want {
				t.Errorf("Past() = %v, want %v — this decides whether the listing shows the event as over", got, tt.want)
			}
		})
	}
}

func TestAttendees(t *testing.T) {
	tests := []struct {
		name string
		in   CheckIn
		want int
	}{
		{"an event nobody has scanned into counts zero", CheckIn{}, 0},
		{"an empty slice counts zero", CheckIn{Attendances: []Attendance{}}, 0},
		{
			"every registered person is counted",
			CheckIn{Attendances: []Attendance{
				{Name: "Kari", UserID: "fa-1"},
				{Name: "Ola", UserID: "fa-2"},
				{Name: "Åse", UserID: "fa-3"},
			}},
			3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Event{CheckIn: tt.in}).Attendees(); got != tt.want {
				t.Errorf("Attendees() = %d, want %d — this number is the attendance figure the board reports", got, tt.want)
			}
		})
	}
}

// HasCheckIn decides two things at once: whether the board is shown a QR code,
// and whether Save mints a fresh code. Getting the "null" sentinel wrong in
// either direction either hands out a QR code pointing at the literal string
// "null", or regenerates a code that is already printed on a poster.
func TestHasCheckIn(t *testing.T) {
	tests := []struct {
		name string
		code string
		want bool
	}{
		{"no code at all", "", false},
		{"the legacy \"null\" sentinel means unassigned", "null", false},
		{"a real code is usable", "5c2b0a1e-0f3f-4c7a-9b21-8a4d9f0e1c33", true},
		// Only the exact sentinel is special: a code that merely contains
		// "null" is a real, printed code.
		{"a code containing null is still a real code", "null-a1b2", true},
		{"NULL in upper case is not the sentinel", "NULL", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Event{CheckIn: CheckIn{Code: tt.code}}).HasCheckIn(); got != tt.want {
				t.Errorf("HasCheckIn(%q) = %v, want %v — a false positive publishes a broken QR code, a false negative reissues one already printed", tt.code, got, tt.want)
			}
		})
	}
}

// ── The end time ──────────────────────────────────────────────────────────

// ComputeEnd is the single definition of when an event finishes; `end` is both
// what the listing filters on and what Discord is told. The arithmetic is done
// on absolute time, which is what makes the daylight-saving cases below come
// out right — an event that runs "two hours" across the spring transition ends
// three wall-clock hours later, because one of those hours does not exist.
func TestComputeEnd(t *testing.T) {
	// The last Sunday in March 2026: 02:00 CET becomes 03:00 CEST.
	springStart := time.Date(2026, 3, 29, 1, 0, 0, 0, timefmt.Oslo)
	// The last Sunday in October 2026: 03:00 CEST becomes 02:00 CET.
	autumnStart := time.Date(2026, 10, 25, 1, 30, 0, 0, timefmt.Oslo)
	plain := time.Date(2026, 9, 1, 17, 15, 0, 0, timefmt.Oslo)

	tests := []struct {
		name     string
		start    time.Time
		duration float64
		want     time.Time
	}{
		{
			"a whole number of hours",
			plain, 3,
			time.Date(2026, 9, 1, 20, 15, 0, 0, timefmt.Oslo),
		},
		{
			// Mongoose stored duration as a double, so half hours have always
			// been expressible and some stored events use them.
			"a half hour is not rounded away",
			plain, 2.5,
			time.Date(2026, 9, 1, 19, 45, 0, 0, timefmt.Oslo),
		},
		{
			// The duration is optional, and blank means zero. An end equal to
			// the start would mark the event "Ferdig" the moment it begins and
			// Discord refuses an event that does not end after it starts, so
			// the unknown end becomes midnight after the start instead.
			"no duration ends at the following midnight",
			plain, 0,
			time.Date(2026, 9, 2, 0, 0, 0, 0, timefmt.Oslo),
		},
		{
			// The start is stored in UTC; the midnight fallback must be the
			// Oslo midnight, not the UTC one. 22:15 UTC on the 1st is already
			// 00:15 on the 2nd in Oslo, so the event ends at midnight on the
			// 3rd — a UTC calculation would end it before it starts.
			"the midnight fallback is Oslo midnight",
			time.Date(2026, 9, 1, 22, 15, 0, 0, time.UTC), 0,
			time.Date(2026, 9, 3, 0, 0, 0, 0, timefmt.Oslo),
		},
		{
			// time.Date normalizes day 32, so a month boundary is ordinary.
			"the midnight fallback crosses a month boundary",
			time.Date(2026, 9, 30, 18, 0, 0, 0, timefmt.Oslo), 0,
			time.Date(2026, 10, 1, 0, 0, 0, 0, timefmt.Oslo),
		},
		{
			// Not reachable through the form, which rejects negatives, but a
			// stored document could carry one. It must not panic; it produces
			// an end before the start, which Past() then reports as finished.
			"a negative duration ends before it starts",
			plain, -1,
			time.Date(2026, 9, 1, 16, 15, 0, 0, timefmt.Oslo),
		},
		{
			"a fractional hour keeps its minutes",
			plain, 1.25,
			time.Date(2026, 9, 1, 18, 30, 0, 0, timefmt.Oslo),
		},
		{
			// 01:00 CET plus two real hours. Local time skips 02:00, so the
			// wall clock reads 04:00 even though the event lasted two hours.
			"crossing the spring transition adds a wall-clock hour",
			springStart, 2,
			time.Date(2026, 3, 29, 4, 0, 0, 0, timefmt.Oslo),
		},
		{
			// 01:30 CEST plus two real hours. The 02:00 hour happens twice, so
			// the wall clock only advances by one hour.
			"crossing the autumn transition loses a wall-clock hour",
			autumnStart, 2,
			autumnStart.Add(2 * time.Hour),
		},
		{
			// The maximum the form allows. Anything longer is rejected there,
			// which is the guard against the overflow the next case shows.
			"a week is still ordinary arithmetic",
			plain, durationMax,
			plain.Add(7 * 24 * time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (Event{Date: tt.start, Duration: tt.duration}).ComputeEnd()
			if !got.Equal(tt.want) {
				t.Errorf("ComputeEnd() = %v, want %v — `end` is what the listing filters on and what Discord is told, so a wrong value either hides the event or announces the wrong span", got, tt.want)
			}
		})
	}
}

// A duration large enough to overflow the int64 nanosecond count wraps around
// and produces an end time in the distant past, which would pin the event to
// the top of every listing forever. Nothing in the write path can produce this
// — the form caps the duration at a week — and this test exists to record why
// that cap is not merely cosmetic.
func TestComputeEndOverflowsOnAnAbsurdDuration(t *testing.T) {
	start := time.Date(2026, 9, 1, 17, 15, 0, 0, time.UTC)
	end := (Event{Date: start, Duration: 1e12}).ComputeEnd()

	if !end.Before(start) {
		t.Skip("time.Duration no longer wraps on overflow; the cap in validate.go is still the real guard")
	}
	if got := (Event{Date: start, End: end}).Past(); !got {
		t.Errorf("an overflowed end (%v) did not read as past; either way the value is nonsense, which is what durationMax exists to prevent", end)
	}
}

// The site is read in Trondheim and the database stores UTC. Both renderings
// have to convert, or every summer event is displayed two hours early.
//
// The date is far in the future so that Smart deterministically takes its
// year-carrying branch no matter when the suite is run.
func TestWhenRendersInOslo(t *testing.T) {
	// 2098-07-01 15:15 UTC is 17:15 CEST. Verified against the calendar:
	// 2098-07-01 is a Tuesday.
	ev := Event{Date: time.Date(2098, 7, 1, 15, 15, 0, 0, time.UTC)}

	if got, want := ev.When(), "tirsdag 1. juli 2098 kl 17:15"; got != want {
		t.Errorf("When() = %q, want %q — a UTC rendering would show every summer event two hours early", got, want)
	}
	if got, want := ev.WhenISO(), "2098-07-01T17:15:00+02:00"; got != want {
		t.Errorf("WhenISO() = %q, want %q — this fills the datetime attribute a calendar import reads", got, want)
	}
}

// An event with a duration shows its full span; one without shows only the
// start, since a zero-hour "span" would render a meaningless "17:15–17:15".
func TestWhenShowsSpanOnlyWithADuration(t *testing.T) {
	start := time.Date(2098, 7, 1, 15, 15, 0, 0, time.UTC) // 17:15 CEST, a Tuesday

	tests := []struct {
		name     string
		duration float64
		want     string
	}{
		{
			"no duration shows just the start",
			0,
			"tirsdag 1. juli 2098 kl 17:15",
		},
		{
			"a same-day duration collapses the end to a clock time",
			2.5,
			"tirsdag 1. juli 2098 kl 17:15–19:45",
		},
		{
			"a duration past midnight spells out both ends",
			12,
			"tirsdag 1. juli 2098 kl 17:15 – onsdag 2. juli 2098 kl 05:15",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := Event{Date: start, Duration: tt.duration}
			if got := ev.When(); got != tt.want {
				t.Errorf("When() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ── BSON round-trips ──────────────────────────────────────────────────────

// BSON datetimes hold milliseconds, so fixtures use whole-millisecond times:
// a nanosecond component would be truncated by the storage format itself and
// the comparison would fail for a reason that has nothing to do with the code
// under test.
func fixture() Event {
	when := time.Date(2026, 9, 1, 17, 15, 0, 0, time.UTC)
	edited := when.Add(time.Hour)
	version := 0

	return Event{
		ID:          bson.NewObjectID(),
		Name:        "Pizza og CTF på Sørhaugen",
		Location:    Place{Name: "Rådssalen, Hovedbygget", URL: "https://kart.ntnu.no/rådssalen"},
		RegisterURL: "https://itemize.no/påmelding",
		Date:        when,
		Duration:    2.5,
		End:         when.Add(150 * time.Minute),
		CTF:         Place{Name: "Julekalender-CTF", URL: "https://ctf.itemize.no/øvelse"},
		Info:        "Ta med laptop.\nVi bestiller pizza kl 18.\nHilsen styret ✌️",
		Hidden:      false,
		Discord:     true,

		DiscordEventID: "1234567890123456789",
		Created:        when.Add(-72 * time.Hour),
		Edited:         &edited,
		CheckIn: CheckIn{
			Code: "5c2b0a1e-0f3f-4c7a-9b21-8a4d9f0e1c33",
			Attendances: []Attendance{
				{ID: bson.NewObjectID(), Name: "Kari Nordmann", UserID: "fa-1", Registered: when},
				{ID: bson.NewObjectID(), Name: "Øyvind Ås", UserID: "fa-2", Registered: when.Add(time.Minute)},
			},
		},
		SchemaVersion: &version,
	}
}

// Every event on the site makes this trip on every read and every write. A
// field lost here is a field silently erased from the database the first time
// a board member edits an event.
func TestEventBSONRoundTrip(t *testing.T) {
	original := fixture()

	raw, err := bson.Marshal(original)
	if err != nil {
		t.Fatalf("marshalling an event: %v", err)
	}
	var got Event
	if err := bson.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshalling an event: %v", err)
	}

	if got.ID != original.ID {
		t.Errorf("_id = %v, want %v", got.ID, original.ID)
	}
	// Norwegian letters and emoji go through the form, into BSON, and back out
	// onto the page. Any encoding mistake here is visible to every visitor.
	if got.Name != original.Name {
		t.Errorf("name = %q, want %q — æøå must survive storage untouched", got.Name, original.Name)
	}
	if got.Info != original.Info {
		t.Errorf("info = %q, want %q — newlines and non-ASCII in board prose must survive storage", got.Info, original.Info)
	}
	if got.Location != original.Location || got.CTF != original.CTF {
		t.Errorf("location/ctf = %+v / %+v, want %+v / %+v", got.Location, got.CTF, original.Location, original.CTF)
	}
	if got.RegisterURL != original.RegisterURL {
		t.Errorf("register_url = %q, want %q", got.RegisterURL, original.RegisterURL)
	}
	if got.Duration != original.Duration {
		t.Errorf("duration = %v, want %v — a rounded duration moves the end time", got.Duration, original.Duration)
	}
	if !got.Date.Equal(original.Date) || !got.End.Equal(original.End) || !got.Created.Equal(original.Created) {
		t.Errorf("times = %v/%v/%v, want %v/%v/%v", got.Date, got.End, got.Created, original.Date, original.End, original.Created)
	}
	if got.Edited == nil || !got.Edited.Equal(*original.Edited) {
		t.Errorf("edited = %v, want %v", got.Edited, original.Edited)
	}
	if got.Hidden != original.Hidden || got.Discord != original.Discord {
		t.Errorf("hidden/discord = %v/%v, want %v/%v", got.Hidden, got.Discord, original.Hidden, original.Discord)
	}
	if got.DiscordEventID != original.DiscordEventID {
		t.Errorf("discordEventId = %q, want %q — losing it orphans the Discord event", got.DiscordEventID, original.DiscordEventID)
	}
	if got.CheckIn.Code != original.CheckIn.Code {
		t.Errorf("check_in.code = %q, want %q — changing it invalidates printed QR codes", got.CheckIn.Code, original.CheckIn.Code)
	}
	if got.SchemaVersion == nil || *got.SchemaVersion != 0 {
		t.Errorf("__v = %v, want a pointer to 0 — every existing document has that exact value", got.SchemaVersion)
	}

	if len(got.CheckIn.Attendances) != len(original.CheckIn.Attendances) {
		t.Fatalf("attendances = %d, want %d — losing one erases somebody's attendance record",
			len(got.CheckIn.Attendances), len(original.CheckIn.Attendances))
	}
	for i, want := range original.CheckIn.Attendances {
		a := got.CheckIn.Attendances[i]
		if a.ID != want.ID || a.Name != want.Name || a.UserID != want.UserID || !a.Registered.Equal(want.Registered) {
			t.Errorf("attendance %d = %+v, want %+v", i, a, want)
		}
	}
}

// omitempty on these three fields is what lets a new event be inserted without
// claiming a zero _id, and what keeps Mongoose's __v out of documents that
// never had one. Emitting them would either collide on insert or add keys the
// old application does not expect.
func TestEventBSONOmitsUnsetOptionalFields(t *testing.T) {
	raw, err := bson.Marshal(Event{})
	if err != nil {
		t.Fatalf("marshalling the zero event: %v", err)
	}
	doc := bson.Raw(raw)

	for _, absent := range []struct{ path, why string }{
		{"_id", "an explicit zero _id would be written as a real identifier instead of letting one be generated"},
		{"edited", "an unedited event would claim to have been edited in year 1"},
		{"__v", "a version key would be added to documents that never had one"},
	} {
		if _, err := doc.LookupErr(absent.path); err == nil {
			t.Errorf("%s was written for an unset field: %s", absent.path, absent.why)
		}
	}

	// The required fields are always present, even at their zero values —
	// omitempty on `hidden` would make a visible event indistinguishable from a
	// legacy document, and omitempty on `duration` would drop a zero-length one.
	for _, present := range []string{"name", "date", "duration", "end", "hidden", "discord", "discordEventId", "created"} {
		if _, err := doc.LookupErr(present); err != nil {
			t.Errorf("%s was omitted at its zero value; the field must always be written", present)
		}
	}

	// Nobody has checked in yet, so the array is absent rather than null —
	// which is the shape $push expects to create.
	if _, err := doc.LookupErr("check_in", "attendances"); err == nil {
		t.Error("an empty attendance list was written out; it should be absent until somebody checks in")
	}
}

// A zero __v is a legitimate value — it is what every document written by the
// previous application carries — which is the entire reason the field is a
// pointer rather than an int.
func TestSchemaVersionZeroIsWritten(t *testing.T) {
	zero := 0
	raw, err := bson.Marshal(Event{SchemaVersion: &zero})
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	v, err := bson.Raw(raw).LookupErr("__v")
	if err != nil {
		t.Fatal("__v = 0 was dropped; a plain int with omitempty would do this, and the version key would vanish from every document on the first edit")
	}
	if got, ok := v.AsInt64OK(); !ok || got != 0 {
		t.Errorf("__v = %v, want 0", v)
	}
}

// A new attendance is given its _id by the repository, not by the caller, so
// the zero value must not be written — Mongoose stamps one onto every
// subdocument and a document with `_id: ObjectID(0)` would not match that shape.
func TestAttendanceBSONOmitsZeroID(t *testing.T) {
	raw, err := bson.Marshal(Attendance{Name: "Kari", UserID: "fa-1"})
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if _, err := bson.Raw(raw).LookupErr("_id"); err == nil {
		t.Error("a zero _id was written for an attendance; every record would then share the same subdocument identifier")
	}

	id := bson.NewObjectID()
	raw, err = bson.Marshal(Attendance{ID: id, Name: "Kari", UserID: "fa-1"})
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if got, ok := bson.Raw(raw).Lookup("_id").ObjectIDOK(); !ok || got != id {
		t.Errorf("_id = %v, want %v", bson.Raw(raw).Lookup("_id"), id)
	}
}

// Documents on disk predate this struct. They carry keys it does not model,
// omit keys it does, and store numbers in whatever type Mongoose chose at the
// time. Decoding must cope with all of it rather than failing the page.
func TestDecodingLegacyDocuments(t *testing.T) {
	when := time.Date(2019, 11, 5, 17, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		doc   bson.M
		check func(t *testing.T, e Event)
	}{
		{
			"a document written before the hidden field existed",
			bson.M{"name": "Gammelt", "date": when},
			func(t *testing.T, e Event) {
				if e.Hidden {
					t.Error("a missing hidden key decoded as hidden; every legacy event would disappear from the listing")
				}
			},
		},
		{
			// Mongoose's Number is a double, but a hand-edited document or a
			// mongosh session can leave an int behind.
			"an integer duration",
			bson.M{"name": "Heltall", "duration": int32(2)},
			func(t *testing.T, e Event) {
				if e.Duration != 2 {
					t.Errorf("duration = %v, want 2 — refusing to decode an int would take the whole listing down", e.Duration)
				}
			},
		},
		{
			"a 64-bit integer duration",
			bson.M{"name": "Heltall", "duration": int64(3)},
			func(t *testing.T, e Event) {
				if e.Duration != 3 {
					t.Errorf("duration = %v, want 3", e.Duration)
				}
			},
		},
		{
			"keys this struct does not model are ignored rather than fatal",
			bson.M{"name": "Ukjent felt", "slug": "noe", "attendeeLimit": 40},
			func(t *testing.T, e Event) {
				if e.Name != "Ukjent felt" {
					t.Errorf("name = %q; an unmodelled key must not stop the rest of the document decoding", e.Name)
				}
			},
		},
		{
			"the legacy \"null\" check-in sentinel decodes as a string",
			bson.M{"name": "Uten kode", "check_in": bson.M{"code": "null"}},
			func(t *testing.T, e Event) {
				if e.CheckIn.Code != "null" {
					t.Errorf("check_in.code = %q, want the literal sentinel preserved", e.CheckIn.Code)
				}
				if e.HasCheckIn() {
					t.Error("the sentinel was treated as a usable code; the QR code would point at nothing")
				}
			},
		},
		{
			"an attendance without the fields we added",
			bson.M{
				"name":     "Med oppmøte",
				"check_in": bson.M{"code": "kode", "attendances": []bson.M{{"name": "Kari"}}},
			},
			func(t *testing.T, e Event) {
				if e.Attendees() != 1 {
					t.Fatalf("attendees = %d, want 1", e.Attendees())
				}
				if e.CheckIn.Attendances[0].Name != "Kari" {
					t.Errorf("name = %q, want Kari", e.CheckIn.Attendances[0].Name)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := bson.Marshal(tt.doc)
			if err != nil {
				t.Fatalf("building the fixture: %v", err)
			}
			var e Event
			if err := bson.Unmarshal(raw, &e); err != nil {
				t.Fatalf("decoding a document already on disk: %v", err)
			}
			tt.check(t, e)
		})
	}
}

// Nothing serves an Event as JSON today. This pins that one could: the
// identifier and the timestamps survive the trip, so a future feed or a debug
// dump does not silently emit an empty object for the _id.
func TestEventJSONRoundTrip(t *testing.T) {
	original := fixture()

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshalling to JSON: %v", err)
	}
	var got Event
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshalling from JSON: %v", err)
	}

	if got.ID != original.ID {
		t.Errorf("ID = %v, want %v — an ObjectID that does not round-trip through JSON makes the payload useless for addressing the event", got.ID, original.ID)
	}
	if !got.Date.Equal(original.Date) || !got.End.Equal(original.End) {
		t.Errorf("times = %v/%v, want %v/%v", got.Date, got.End, original.Date, original.End)
	}
	if got.Name != original.Name || got.Info != original.Info {
		t.Errorf("text did not survive: %q / %q", got.Name, got.Info)
	}
	if !strings.Contains(string(raw), "Sørhaugen") {
		t.Errorf("Norwegian letters were escaped out of recognition: %s", raw)
	}
}
