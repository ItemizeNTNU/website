package web

import (
	"errors"
	"net/http"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ItemizeNTNU/website/internal/events"
	"github.com/ItemizeNTNU/website/internal/timefmt"
	"github.com/ItemizeNTNU/website/internal/validate"
)

// eventForm is the admin form's state, so a rejected submission can be
// re-rendered with what was typed rather than making the board start over.
type eventForm struct {
	ID           string
	Name         string
	LocationURL  string
	LocationName string
	RegisterURL  string
	Date         string
	Duration     string
	CTFName      string
	CTFURL       string
	Info         string
	Hidden       bool
	Discord      bool
}

// formFromEvent pre-fills the form for editing.
func formFromEvent(e *events.Event) eventForm {
	return eventForm{
		ID:           e.HexID(),
		Name:         e.Name,
		LocationName: e.Location.Name,
		LocationURL:  e.Location.URL,
		RegisterURL:  e.RegisterURL,
		Date:         timefmt.FormValue(e.Date),
		Duration:     trimFloat(e.Duration),
		CTFName:      e.CTF.Name,
		CTFURL:       e.CTF.URL,
		Info:         e.Info,
		Hidden:       e.Hidden,
		Discord:      e.Discord,
	}
}

// parseID turns a hex identifier into an ObjectID.
//
// A malformed value is treated as "no such event" rather than a bad request:
// the two are indistinguishable to a visitor, and answering differently
// confirms the identifier format to anyone probing.
func parseID(s string) (bson.ObjectID, bool) {
	if s == "" {
		return bson.ObjectID{}, false
	}
	id, err := bson.ObjectIDFromHex(s)
	if err != nil {
		return bson.ObjectID{}, false
	}
	return id, true
}

// saveEvent handles the admin form.
func (s *Server) saveEvent(w http.ResponseWriter, r *http.Request) {
	if s.eventSvc == nil {
		s.ErrorPage(w, r, http.StatusServiceUnavailable)
		return
	}

	in, verr := events.FromForm(r.PostForm)

	var id bson.ObjectID
	if raw := r.PostFormValue("id"); raw != "" {
		parsed, ok := parseID(raw)
		if !ok {
			s.ErrorPage(w, r, http.StatusNotFound)
			return
		}
		id = parsed
	}

	if verr.Any() {
		s.renderEventsPage(w, r, http.StatusUnprocessableEntity, formFromSubmission(r), verr)
		return
	}

	_, err := s.eventSvc.Save(r.Context(), id, in)
	switch {
	case errors.Is(err, events.ErrNotFound):
		s.ErrorPage(w, r, http.StatusNotFound)
		return
	case errors.Is(err, events.ErrDiscordSync):
		// The event is stored; only the announcement failed. Say precisely
		// that, rather than implying the whole operation was lost.
		SetFlash(w, "warning", "Arrangementet er lagret, men Discord ble ikke oppdatert.")
	case err != nil:
		s.log.Error("saving the event failed", "err", err)
		s.renderEventsPage(w, r, http.StatusInternalServerError, formFromSubmission(r),
			validate.Errors{"": "Arrangementet kunne ikke lagres. Prøv igjen."})
		return
	default:
		if id.IsZero() {
			SetFlash(w, "success", "Arrangementet er lagt til.")
		} else {
			SetFlash(w, "success", "Arrangementet er oppdatert.")
		}
	}

	// Redirect after post, so a refresh does not resubmit.
	http.Redirect(w, r, "/arrangementer", http.StatusSeeOther)
}

// formFromSubmission rebuilds the form from what was posted, so a rejected
// submission comes back populated.
func formFromSubmission(r *http.Request) eventForm {
	return eventForm{
		ID:           r.PostFormValue("id"),
		Name:         r.PostFormValue("name"),
		LocationName: r.PostFormValue("location.name"),
		LocationURL:  r.PostFormValue("location.url"),
		RegisterURL:  r.PostFormValue("register_url"),
		Date:         r.PostFormValue("date"),
		Duration:     r.PostFormValue("duration"),
		CTFName:      r.PostFormValue("ctf.name"),
		CTFURL:       r.PostFormValue("ctf.url"),
		Info:         r.PostFormValue("info"),
		Hidden:       r.PostFormValue("hidden") != "",
		Discord:      r.PostFormValue("discord") != "",
	}
}

// confirmDelete renders the confirmation page.
//
// A page rather than only a dialog, so deleting works without JavaScript and
// is honest about being irreversible. With scripting the link is intercepted
// and the same wording appears in a dialog instead.
func (s *Server) confirmDelete(w http.ResponseWriter, r *http.Request) {
	if s.eventSvc == nil {
		s.ErrorPage(w, r, http.StatusServiceUnavailable)
		return
	}
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		s.ErrorPage(w, r, http.StatusNotFound)
		return
	}

	event, err := s.events.ByID(r.Context(), id)
	if err != nil {
		s.ErrorPage(w, r, http.StatusNotFound)
		return
	}

	view := struct {
		Page
		Event *events.Event
	}{Page: s.page(r, "Slett arrangement", "arrangementer"), Event: event}
	view.CSRF = s.csrf(w, r)

	s.render(w, r, http.StatusOK, "slett-arrangement", view)
}

// deleteEvent removes an event.
func (s *Server) deleteEvent(w http.ResponseWriter, r *http.Request) {
	if s.eventSvc == nil {
		s.ErrorPage(w, r, http.StatusServiceUnavailable)
		return
	}
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		s.ErrorPage(w, r, http.StatusNotFound)
		return
	}

	switch err := s.eventSvc.Delete(r.Context(), id); {
	case errors.Is(err, events.ErrNotFound):
		s.ErrorPage(w, r, http.StatusNotFound)
		return
	case err != nil:
		s.log.Error("deleting the event failed", "err", err)
		SetFlash(w, "error", "Arrangementet kunne ikke slettes.")
	default:
		SetFlash(w, "success", "Arrangementet er slettet.")
	}

	http.Redirect(w, r, "/arrangementer", http.StatusSeeOther)
}

// trimFloat renders a duration without a trailing ".0", so a two-hour event
// shows "2" rather than "2.0" in the form. Half-hour durations still render
// as "2.5".
func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
