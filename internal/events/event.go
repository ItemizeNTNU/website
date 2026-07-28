// Package events models the event calendar and its check-in register.
package events

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ItemizeNTNU/website/internal/timefmt"
)

// Collection is the MongoDB collection name. It matches what the previous
// Mongoose model used, because the Go binary reads and writes the same
// documents rather than migrating them.
const Collection = "events"

// Event is one entry in the calendar.
//
// The field names and nesting mirror the Mongoose schema exactly. Several
// details here look redundant but are load-bearing when reading documents the
// old application wrote — see the comments on each.
type Event struct {
	ID          bson.ObjectID `bson:"_id,omitempty"`
	Name        string        `bson:"name"`
	Location    Place         `bson:"location"`
	RegisterURL string        `bson:"register_url"`
	Date        time.Time     `bson:"date"`
	// Duration is in hours. Mongoose's Number is always a BSON double, so a
	// duration of "2" on disk is 2.0, and half-hour durations are expressible.
	Duration float64   `bson:"duration"`
	End      time.Time `bson:"end"`
	CTF      Place     `bson:"ctf"`
	// Info is board-authored prose. It was previously rendered as raw HTML;
	// it is now escaped, and newlines are preserved by the stylesheet.
	Info    string `bson:"info"`
	Hidden  bool   `bson:"hidden"`
	Discord bool   `bson:"discord"`
	// DiscordEventID points at a Discord scheduled event. Losing it orphans
	// that event — we would no longer be able to update or delete it — so it
	// must round-trip untouched even when nothing else about the event changes.
	DiscordEventID string     `bson:"discordEventId"`
	Created        time.Time  `bson:"created"`
	Edited         *time.Time `bson:"edited,omitempty"`
	CheckIn        CheckIn    `bson:"check_in"`
	// SchemaVersion is Mongoose's __v. A pointer so that omitempty does not
	// discard a legitimate zero, which is what every existing document has.
	SchemaVersion *int `bson:"__v,omitempty"`
}

// Place is a name with an optional link — used for both the venue and the CTF.
type Place struct {
	Name string `bson:"name"`
	URL  string `bson:"url"`
}

// CheckIn holds the attendance register for an event.
type CheckIn struct {
	// Code is the UUID embedded in the QR codes handed out at the door. Codes
	// have been printed, so this value can never be regenerated for an event
	// that already has one.
	Code        string       `bson:"code"`
	Attendances []Attendance `bson:"attendances,omitempty"`
}

// Attendance is one person checking in.
type Attendance struct {
	// ID exists because Mongoose stamps an _id onto every subdocument in an
	// array. Without modelling it, a round-trip through Go would silently drop
	// it from every existing record.
	ID     bson.ObjectID `bson:"_id,omitempty"`
	Name   string        `bson:"name"`
	UserID string        `bson:"user_id"`
	// Registered is when they checked in.
	Registered time.Time `bson:"registered"`
}

// The accessors below take value receivers, not pointers, and that is
// load-bearing rather than stylistic.
//
// Templates range over []Event, which yields values, and a value placed into a
// map[string]any is boxed in an interface — which is not addressable. Go
// templates cannot call a pointer-receiver method on a non-addressable value,
// so every one of these would silently become unavailable and the page would
// fail to render with "can't evaluate field". None of them mutate, so there is
// nothing to gain from a pointer.

// HexID renders the identifier for a URL or a form field.
func (e Event) HexID() string {
	if e.ID.IsZero() {
		return ""
	}
	return e.ID.Hex()
}

// Past reports whether the event has finished.
func (e Event) Past() bool { return !e.End.IsZero() && e.End.Before(time.Now()) }

// When renders the start time the way the site displays it.
func (e Event) When() string { return timefmt.Smart(e.Date) }

// WhenISO renders the start time for a machine — the datetime attribute on a
// <time> element.
func (e Event) WhenISO() string { return timefmt.ISO(e.Date) }

// Attendees is the number of people checked in.
func (e Event) Attendees() int { return len(e.CheckIn.Attendances) }

// HasCheckIn reports whether the event has a usable check-in code.
//
// The previous application seeded new events with the literal string "null"
// before replacing it, so that value appears on disk and means "no code".
func (e Event) HasCheckIn() bool {
	return e.CheckIn.Code != "" && e.CheckIn.Code != "null"
}

// ComputeEnd derives the end time from the start and duration. Kept as a
// method so the write path and the tests cannot disagree about it.
func (e Event) ComputeEnd() time.Time {
	return e.Date.Add(time.Duration(e.Duration * float64(time.Hour)))
}
