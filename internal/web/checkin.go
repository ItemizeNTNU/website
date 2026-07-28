package web

import (
	"errors"
	"html/template"
	"net/http"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/events"
	"github.com/ItemizeNTNU/website/internal/qr"
	"github.com/ItemizeNTNU/website/internal/timefmt"
)

type checkinView struct {
	Page
	Event   *events.Event
	Code    string
	ScanURL string
	QR      template.HTML
}

// innsjekk shows the QR code to hold up at the door, and who has scanned it.
func (s *Server) innsjekk(w http.ResponseWriter, r *http.Request) {
	if s.events == nil {
		s.ErrorPage(w, r, http.StatusServiceUnavailable)
		return
	}

	code := r.PathValue("code")
	event, err := s.events.ByCheckInCode(r.Context(), code)
	if err != nil {
		s.ErrorPage(w, r, http.StatusNotFound)
		return
	}

	scanURL := s.baseURL + "/innsjekk-qr/" + code
	svg, err := qr.SVG(scanURL)
	if err != nil {
		s.log.Error("rendering the QR code failed", "err", err)
		s.ErrorPage(w, r, http.StatusInternalServerError)
		return
	}

	view := checkinView{
		Page:    s.page(r, "Innsjekk", "innsjekk"),
		Event:   event,
		Code:    code,
		ScanURL: scanURL,
		QR:      svg,
	}
	view.Command = "./checkin --qr"
	// A code on screen is a code someone can register attendance with, so it
	// should not sit in a shared cache or a browser's history store.
	w.Header().Set("Cache-Control", "no-store")
	s.render(w, r, http.StatusOK, "innsjekk", view)
}

type scanView struct {
	Page
	Event   *events.Event
	Message string
	OK      bool
}

// innsjekkQR is what a scanned code lands on. It registers the attendance.
//
// This performs a write on a GET, which is not something to do lightly. It
// stays that way because the URL is printed on QR codes already in
// circulation, and because making someone tap a confirm button at a door
// defeats the point. The mitigations are that the write is idempotent — a
// second scan is rejected rather than duplicated — and that the response is
// never cached or referred onward.
func (s *Server) innsjekkQR(w http.ResponseWriter, r *http.Request) {
	if s.events == nil {
		s.ErrorPage(w, r, http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")

	user := auth.FromRequest(r)
	code := r.PathValue("code")

	event, err := s.events.ByCheckInCode(r.Context(), code)
	if err != nil {
		s.ErrorPage(w, r, http.StatusNotFound)
		return
	}

	view := scanView{
		Page:  s.page(r, "Innsjekk", "innsjekk"),
		Event: event,
	}
	view.Command = "./checkin --scan"

	err = s.events.AddAttendance(r.Context(), code, events.Attendance{
		// The attendance list is read by people, so it wants the real name.
		Name:   attendanceName(user),
		UserID: user.ID,
	})
	switch {
	case err == nil:
		view.OK = true
		view.Message = "Du er sjekket inn."
	case errors.Is(err, events.ErrAlreadyCheckedIn):
		// Not an error worth alarming anyone about: they scanned twice.
		view.OK = true
		view.Message = "Du er allerede sjekket inn på dette arrangementet."
	case errors.Is(err, events.ErrNotFound):
		s.ErrorPage(w, r, http.StatusNotFound)
		return
	default:
		s.log.Error("registering attendance failed", "code", code, "err", err)
		view.Message = "Innsjekkingen gikk ikke gjennom. Si fra til noen i styret."
	}

	s.render(w, r, http.StatusOK, "innsjekk-qr", view)
}

// attendanceName picks the most useful name for the attendance list.
//
// The previous server wrote fullName unconditionally, which produced blank
// rows whenever that claim was missing from the token — a failure nobody
// noticed until they read the list afterwards.
func attendanceName(u *auth.User) string {
	if u == nil {
		return ""
	}
	if u.FullName != "" {
		return u.FullName
	}
	if name := u.DisplayName(); name != "" {
		return name
	}
	return u.ID
}

// Registered renders an attendance timestamp the way the rest of the site
// shows times.
func attendanceTime(a events.Attendance) string { return timefmt.Smart(a.Registered) }
