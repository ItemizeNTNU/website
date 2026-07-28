package events

import (
	"net/url"

	"github.com/ItemizeNTNU/website/internal/timefmt"
	"github.com/ItemizeNTNU/website/internal/validate"
)

// Field length limits, matching the previous server's schema exactly so that
// nothing already stored becomes unsavable.
const (
	nameMin, nameMax         = 3, 50
	locationMin, locationMax = 3, 200
	ctfNameMax               = 200
	infoMin, infoMax         = 3, 2000
	// A week. The previous schema had no upper bound, which let a typo in the
	// duration field push an event's end years into the future and keep it
	// pinned to the top of the listing.
	durationMax = 24 * 7
)

// FromForm builds an event from submitted form values, collecting every
// problem rather than stopping at the first.
//
// Unlike the previous version, which validated with abortEarly and made the
// user fix one field per round trip, this reports everything at once so the
// re-rendered form can annotate each field.
//
// Fields the form must not control — the identifier, the check-in code, the
// Discord event id — are deliberately absent. They are carried over from the
// stored event by the service, so a crafted submission cannot reassign an
// event's check-in code or detach it from its Discord event.
func FromForm(form url.Values) (*Event, validate.Errors) {
	var e validate.Errors
	ev := &Event{}

	ev.Name = e.Text("name", "Navn", form.Get("name"), nameMin, nameMax)
	ev.Location = Place{
		Name: e.Text("location.name", "Hvor", form.Get("location.name"), locationMin, locationMax),
		URL:  e.HTTPSURL("location.url", form.Get("location.url")),
	}
	ev.RegisterURL = e.HTTPSURL("register_url", form.Get("register_url"))
	ev.CTF = Place{
		Name: e.Optional("ctf.name", "CTF-navn", form.Get("ctf.name"), ctfNameMax),
		URL:  e.HTTPSURL("ctf.url", form.Get("ctf.url")),
	}
	ev.Info = e.Text("info", "Info", form.Get("info"), infoMin, infoMax)
	ev.Duration = e.Number("duration", "Varighet", form.Get("duration"), 0, durationMax)

	if date, err := timefmt.ParseFormValue(form.Get("date")); err != nil {
		e.Add("date", "Når må fylles ut med gyldig dato og tid.")
	} else {
		ev.Date = date
	}

	// An unchecked checkbox is simply absent from the submission, which is how
	// both of these read as false.
	ev.Hidden = form.Get("hidden") != ""
	ev.Discord = form.Get("discord") != ""

	// A hidden event has nothing to announce. Rather than rejecting the
	// combination, treat hidden as the stronger statement — the service then
	// removes any Discord event that already exists.
	if ev.Hidden {
		ev.Discord = false
	}

	return ev, e
}
