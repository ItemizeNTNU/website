// Package timefmt formats times the way the site displays them: in
// Europe/Oslo, in Norwegian.
//
// The standard library has no locale data, so the month and weekday names are
// a lookup table. That is the whole of what a localisation library would add
// here, and it is less code than depending on one.
package timefmt

import (
	"fmt"
	"time"
)

// Oslo is the timezone every displayed time is rendered in. The organisation
// is in Trondheim and its events are physical, so there is no second zone to
// worry about.
//
// Resolving this needs the tz database, which the binary embeds via a blank
// import of time/tzdata in main — the container's base image carries none.
var Oslo = mustLoad("Europe/Oslo")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		// Unreachable with time/tzdata imported. If it ever fires, every date
		// on the site would silently be an hour or two wrong, which is far
		// worse than not starting.
		panic("timefmt: cannot load " + name + ": " + err.Error())
	}
	return loc
}

var weekdays = [...]string{
	time.Monday:    "mandag",
	time.Tuesday:   "tirsdag",
	time.Wednesday: "onsdag",
	time.Thursday:  "torsdag",
	time.Friday:    "fredag",
	time.Saturday:  "lørdag",
	time.Sunday:    "søndag",
}

var months = [...]string{
	time.January:   "januar",
	time.February:  "februar",
	time.March:     "mars",
	time.April:     "april",
	time.May:       "mai",
	time.June:      "juni",
	time.July:      "juli",
	time.August:    "august",
	time.September: "september",
	time.October:   "oktober",
	time.November:  "november",
	time.December:  "desember",
}

// Weekday returns the Norwegian name of the day, lowercase.
func Weekday(t time.Time) string { return weekdays[t.In(Oslo).Weekday()] }

// Month returns the Norwegian name of the month, lowercase.
func Month(t time.Time) string { return months[t.In(Oslo).Month()] }

// Smart formats a time the way a person would say it, dropping the parts that
// are obvious from context:
//
//	"I dag kl 17:15"
//	"tirsdag 1. februar kl 17:15"
//	"tirsdag 1. februar 2021 kl 17:15"
//
// A port of smartFormat from the previous site, including its quirk of
// comparing month and day rather than the calendar date — which means a time
// exactly one year ago today still reads as "I dag" until the year check
// overrides it.
func Smart(t time.Time) string {
	return smartAt(t, time.Now())
}

// smartAt is Smart with an injectable "now", so the behaviour is testable
// without touching the clock.
func smartAt(t, now time.Time) string {
	t = t.In(Oslo)
	now = now.In(Oslo)

	clock := fmt.Sprintf("kl %02d:%02d", t.Hour(), t.Minute())

	switch {
	case t.Year() != now.Year():
		return fmt.Sprintf("%s %d. %s %d %s",
			Weekday(t), t.Day(), Month(t), t.Year(), clock)
	case t.Month() != now.Month() || t.Day() != now.Day():
		return fmt.Sprintf("%s %d. %s %s", Weekday(t), t.Day(), Month(t), clock)
	default:
		return "I dag " + clock
	}
}

// Range renders a start and end time as a span, collapsing the parts the two
// share. Used where an event's duration matters more than its start.
func Range(start, end time.Time) string {
	start, end = start.In(Oslo), end.In(Oslo)
	if start.YearDay() == end.YearDay() && start.Year() == end.Year() {
		return fmt.Sprintf("%s–%02d:%02d", Smart(start), end.Hour(), end.Minute())
	}
	return Smart(start) + " – " + Smart(end)
}

// ISO renders a time for a machine: datetime attributes, iCal, form values.
func ISO(t time.Time) string { return t.In(Oslo).Format(time.RFC3339) }

// FormValue renders a time for an <input type="datetime-local">, which expects
// local wall-clock time with no zone.
func FormValue(t time.Time) string { return t.In(Oslo).Format("2006-01-02T15:04") }

// ParseFormValue reads what an <input type="datetime-local"> submits,
// interpreting it as Oslo wall-clock time.
func ParseFormValue(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, Oslo); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("ugyldig dato og tid: %q", s)
}
