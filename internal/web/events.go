package web

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/events"
	"github.com/ItemizeNTNU/website/internal/validate"
)

type eventsView struct {
	Page
	Events  []events.Event
	ShowOld bool
	// Unavailable is set when the database could not be reached, so the page
	// can say so instead of claiming there are no events.
	Unavailable bool

	// The subscribe box's three flavours of the same feed. FeedURL is the
	// plain https:// address for copying into any client; WebcalURL swaps the
	// scheme for webcal://, which Apple Calendar and Outlook treat as "add a
	// subscription" where an https:// link would just download a file; and
	// GoogleCalURL wraps the webcal address in Google Calendar's add-by-URL
	// page, whose cid parameter is how a calendar is handed to it.
	//
	// WebcalURL is a template.URL because html/template only trusts http,
	// https and mailto in an href and would rewrite webcal:// to #ZgotmplZ.
	// Marking it trusted is sound here: the value is derived entirely from
	// the configured base URL, never from user input.
	FeedURL      string
	WebcalURL    template.URL
	GoogleCalURL string

	// Admin form state. Only rendered for the board.
	Form     eventForm
	Errors   validate.Errors
	Editing  bool
	FormOpen bool
}

// arrangementer lists the calendar.
//
// "Show past events" is a query parameter rather than a script toggle, so the
// view is bookmarkable, linkable and works without JavaScript. Editing works
// the same way: ?rediger=<id> renders the form pre-filled and open.
func (s *Server) arrangementer(w http.ResponseWriter, r *http.Request) {
	s.renderEventsPage(w, r, http.StatusOK, eventForm{}, nil)
}

// renderEventsPage draws the listing, optionally with the admin form in a
// given state. Shared so a rejected submission redisplays the page exactly as
// it looked, rather than bouncing through a redirect and losing the input.
func (s *Server) renderEventsPage(
	w http.ResponseWriter, r *http.Request, status int,
	form eventForm, verr validate.Errors,
) {
	user := auth.FromRequest(r)
	styret := user.IsStyret()
	showOld := r.URL.Query().Has("old")

	view := eventsView{
		Page:     s.page(r, "Arrangementer", "arrangementer"),
		ShowOld:  showOld,
		Form:     form,
		Errors:   verr,
		FormOpen: verr.Any() || form.ID != "",
	}
	view.Desc = "Oversikt over kommende arrangementer i Itemize NTNU."
	view.Command = "./events --upcoming"
	if showOld {
		view.Command = "./events --all"
	}

	// Computed before the s.events nil check below: the feed address is
	// derived from configuration alone, so the subscribe box keeps working
	// while the database is down — which is exactly when a visitor is best
	// served by a copy of the calendar that updates itself.
	view.FeedURL = s.baseURL + "/api/events/ical"
	webcal := "webcal://" + strings.TrimPrefix(strings.TrimPrefix(view.FeedURL, "https://"), "http://")
	view.WebcalURL = template.URL(webcal)
	view.GoogleCalURL = "https://calendar.google.com/calendar/r?cid=" + url.QueryEscape(webcal)

	if styret {
		view.CSRF = s.csrf(w, r)
	}

	if s.events == nil {
		view.Unavailable = true
		s.render(w, r, status, "arrangementer", view)
		return
	}

	// ?rediger=<id> pre-fills the form. Ignored for anyone but the board, so a
	// crafted link does not reveal a draft's contents.
	if styret && !view.FormOpen {
		if id, ok := parseID(r.URL.Query().Get("rediger")); ok {
			if existing, err := s.events.ByID(r.Context(), id); err == nil {
				view.Form = formFromEvent(existing)
				view.FormOpen = true
			}
		}
	}
	view.Editing = view.Form.ID != ""

	list, err := s.events.List(r.Context(), events.Filter{
		// Drafts are for the board only.
		IncludeHidden: styret,
		IncludeOld:    showOld,
	})
	if err != nil {
		s.log.Error("listing events failed", "err", err)
		view.Unavailable = true
		s.render(w, r, status, "arrangementer", view)
		return
	}

	view.Events = list
	s.render(w, r, status, "arrangementer", view)
}
