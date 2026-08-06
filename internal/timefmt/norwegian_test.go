package timefmt

import (
	"testing"
	"time"
)

func TestSmart(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, Oslo)

	tests := []struct {
		name string
		when time.Time
		want string
	}{
		{
			"same day drops the date entirely",
			time.Date(2026, 2, 1, 17, 15, 0, 0, Oslo),
			"I dag kl 17:15",
		},
		{
			"another day this year names the weekday and month",
			time.Date(2026, 2, 3, 17, 15, 0, 0, Oslo),
			"tirsdag 3. februar kl 17:15",
		},
		{
			"another year adds the year",
			time.Date(2021, 2, 1, 17, 15, 0, 0, Oslo),
			"mandag 1. februar 2021 kl 17:15",
		},
		{
			"midnight pads both fields",
			time.Date(2026, 3, 9, 0, 5, 0, 0, Oslo),
			"mandag 9. mars kl 00:05",
		},
		{
			"Norwegian letters survive",
			time.Date(2026, 5, 16, 9, 0, 0, 0, Oslo),
			"lørdag 16. mai kl 09:00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := smartAt(tt.when, now); got != tt.want {
				t.Errorf("smartAt() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A time stored in UTC has to be displayed in Oslo, or every summer event
// shows up two hours early.
func TestSmartConvertsToOslo(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, Oslo)
	utc := time.Date(2026, 7, 2, 15, 15, 0, 0, time.UTC)

	// July is CEST, UTC+2.
	if got, want := smartAt(utc, now), "torsdag 2. juli kl 17:15"; got != want {
		t.Errorf("smartAt() = %q, want %q", got, want)
	}
}

func TestSmartAcrossDSTBoundary(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, Oslo)

	// Clocks go forward on the last Sunday in March. An event stored as UTC
	// either side of it must land on the right local hour.
	before := time.Date(2026, 3, 28, 16, 15, 0, 0, time.UTC) // CET, UTC+1
	after := time.Date(2026, 3, 29, 16, 15, 0, 0, time.UTC)  // CEST, UTC+2

	if got, want := smartAt(before, now), "lørdag 28. mars kl 17:15"; got != want {
		t.Errorf("before DST: got %q, want %q", got, want)
	}
	if got, want := smartAt(after, now), "søndag 29. mars kl 18:15"; got != want {
		t.Errorf("after DST: got %q, want %q", got, want)
	}
}

func TestFormValueRoundTrip(t *testing.T) {
	original := time.Date(2026, 2, 1, 17, 15, 0, 0, Oslo)

	parsed, err := ParseFormValue(FormValue(original))
	if err != nil {
		t.Fatalf("ParseFormValue: %v", err)
	}
	if !parsed.Equal(original) {
		t.Errorf("round trip changed the time: got %v, want %v", parsed, original)
	}
}

// A datetime-local field submits wall-clock time with no zone, so it has to be
// read as Oslo time rather than UTC.
func TestParseFormValueIsOsloLocal(t *testing.T) {
	got, err := ParseFormValue("2026-07-02T17:15")
	if err != nil {
		t.Fatalf("ParseFormValue: %v", err)
	}
	if want := time.Date(2026, 7, 2, 15, 15, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("got %v, want %v", got.UTC(), want)
	}
}

func TestParseFormValueRejectsJunk(t *testing.T) {
	if _, err := ParseFormValue("i morgen"); err == nil {
		t.Error("expected an error for unparseable input")
	}
}
