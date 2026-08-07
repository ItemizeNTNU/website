package timefmt

import (
	"testing"
	"time"
)

// Weekday and Month must convert to Oslo before looking up the name. A time
// stored as UTC just before midnight is already the next day in Oslo, and
// naming the UTC day would put an event on the wrong weekday on the site.
func TestWeekdayAndMonth(t *testing.T) {
	tests := []struct {
		name    string
		when    time.Time
		weekday string
		month   string
	}{
		{
			// 2026-02-28 23:30 UTC is 2026-03-01 00:30 CET (+01:00).
			"winter time crosses into March",
			time.Date(2026, 2, 28, 23, 30, 0, 0, time.UTC),
			"søndag", "mars",
		},
		{
			// 2026-06-30 22:30 UTC is 2026-07-01 00:30 CEST (+02:00).
			"summer time crosses into July",
			time.Date(2026, 6, 30, 22, 30, 0, 0, time.UTC),
			"onsdag", "juli",
		},
		{
			// 2025-12-31 23:30 UTC is 2026-01-01 00:30 CET — a new year in
			// Oslo while UTC is still in the old one.
			"winter time crosses the year boundary",
			time.Date(2025, 12, 31, 23, 30, 0, 0, time.UTC),
			"torsdag", "januar",
		},
		{
			// 2026-05-15 22:30 UTC is 2026-05-16 00:30 CEST.
			"summer time crosses into a Saturday",
			time.Date(2026, 5, 15, 22, 30, 0, 0, time.UTC),
			"lørdag", "mai",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Weekday(tt.when); got != tt.weekday {
				t.Errorf("Weekday(%v) = %q, want %q — naming the UTC day instead of the Oslo day would list an event under the wrong weekday", tt.when, got, tt.weekday)
			}
			if got := Month(tt.when); got != tt.month {
				t.Errorf("Month(%v) = %q, want %q — a UTC time near midnight on the month boundary would show the wrong month", tt.when, got, tt.month)
			}
		})
	}
}

// ISO output feeds datetime attributes and iCal, so it must carry the real
// Oslo offset: +01:00 in winter, +02:00 in summer. A fixed offset would shift
// every event in the other half of the year by an hour.
func TestISO(t *testing.T) {
	tests := []struct {
		name string
		when time.Time
		want string
	}{
		{
			"winter time carries +01:00",
			time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
			"2026-01-15T13:00:00+01:00",
		},
		{
			"summer time carries +02:00",
			time.Date(2026, 7, 2, 15, 15, 0, 0, time.UTC),
			"2026-07-02T17:15:00+02:00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ISO(tt.when); got != tt.want {
				t.Errorf("ISO(%v) = %q, want %q — calendar imports and datetime attributes would be an hour off", tt.when, got, tt.want)
			}
		})
	}
}

// Range calls Smart, which compares against the real time.Now(). Using dates
// in 2098–2099 keeps t.Year() != now.Year() true for the life of the project,
// so Smart deterministically takes its full year-carrying branch and the
// expected strings never rot as the calendar advances.
//
// Verified against the real calendar: 2098-05-01 is a Thursday, 2098-05-02 a
// Friday, and 2099-05-01 a Friday.
func TestRange(t *testing.T) {
	tests := []struct {
		name       string
		start, end time.Time
		want       string
	}{
		{
			"same-day span collapses the end to a bare clock time",
			time.Date(2098, 5, 1, 17, 0, 0, 0, Oslo),
			time.Date(2098, 5, 1, 19, 30, 0, 0, Oslo),
			"torsdag 1. mai 2098 kl 17:00–19:30",
		},
		{
			"cross-day span spells out both ends",
			time.Date(2098, 5, 1, 20, 0, 0, 0, Oslo),
			time.Date(2098, 5, 2, 2, 0, 0, 0, Oslo),
			"torsdag 1. mai 2098 kl 20:00 – fredag 2. mai 2098 kl 02:00",
		},
		{
			// Both dates are day 121 of their year. Collapsing on YearDay
			// alone would render a year-long span as a few hours.
			"same YearDay in different years does not collapse",
			time.Date(2098, 5, 1, 17, 0, 0, 0, Oslo),
			time.Date(2099, 5, 1, 17, 0, 0, 0, Oslo),
			"torsdag 1. mai 2098 kl 17:00 – fredag 1. mai 2099 kl 17:00",
		},
		{
			"crossing midnight by a minute spells out both ends",
			time.Date(2098, 5, 1, 23, 30, 0, 0, Oslo),
			time.Date(2098, 5, 2, 0, 1, 0, 0, Oslo),
			"torsdag 1. mai 2098 kl 23:30 – fredag 2. mai 2098 kl 00:01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Range(tt.start, tt.end); got != tt.want {
				t.Errorf("Range() = %q, want %q — a wrongly collapsed or expanded span misstates when an event ends", got, tt.want)
			}
		})
	}
}
