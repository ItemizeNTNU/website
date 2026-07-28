package web

import (
	"net/http"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/events"
)

type eventsView struct {
	Page
	Events  []events.Event
	ShowOld bool
	// Unavailable is set when the database could not be reached, so the page
	// can say so instead of claiming there are no events.
	Unavailable bool
}

// arrangementer lists the calendar.
//
// "Show past events" is a query parameter rather than a script toggle, so the
// view is bookmarkable, linkable and works without JavaScript.
func (s *Server) arrangementer(w http.ResponseWriter, r *http.Request) {
	user := auth.FromRequest(r)
	showOld := r.URL.Query().Has("old")

	view := eventsView{
		Page:    s.page(r, "Arrangementer", "arrangementer"),
		ShowOld: showOld,
	}
	view.Desc = "Oversikt over kommende arrangementer i Itemize NTNU."

	if s.events == nil {
		view.Unavailable = true
		s.render(w, r, http.StatusOK, "arrangementer", view)
		return
	}

	list, err := s.events.List(r.Context(), events.Filter{
		// Drafts are for the board only.
		IncludeHidden: user.IsStyret(),
		IncludeOld:    showOld,
	})
	if err != nil {
		s.log.Error("listing events failed", "err", err)
		view.Unavailable = true
		s.render(w, r, http.StatusOK, "arrangementer", view)
		return
	}

	view.Events = list
	s.render(w, r, http.StatusOK, "arrangementer", view)
}
