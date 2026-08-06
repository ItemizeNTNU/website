package api

import (
	"errors"
	"net/http"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/events"
)

// getCheckIn returns an event by its check-in code, including the attendance
// register.
//
// Board only. The previous version served this to anyone: the response
// contains the check-in code itself — which is the credential that registers
// attendance — along with every attendee's name and FusionAuth identifier.
// Its only caller was a page already restricted to the board, so nothing is
// lost by enforcing that on the server too.
func (s *Server) getCheckIn(w http.ResponseWriter, r *http.Request) {
	event, err := s.events.ByCheckInCode(r.Context(), r.PathValue("code"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, message{"Event not found"})
		return
	}
	writeJSON(w, http.StatusOK, toDTO(*event, true))
}

// postCheckIn registers the caller's attendance.
func (s *Server) postCheckIn(w http.ResponseWriter, r *http.Request) {
	user := auth.FromRequest(r)
	code := r.PathValue("code")

	err := s.events.AddAttendance(r.Context(), code, events.Attendance{
		Name:   nameFor(user),
		UserID: user.ID,
	})
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, message{"Success"})
	case errors.Is(err, events.ErrAlreadyCheckedIn):
		writeJSON(w, http.StatusConflict,
			message{"You have already registered your attendance for this event"})
	case errors.Is(err, events.ErrNotFound):
		writeJSON(w, http.StatusNotFound,
			message{"Event not found with check_in code \"" + code + "\""})
	default:
		s.log.Error("registering attendance failed", "code", code, "err", err)
		writeJSON(w, http.StatusInternalServerError, message{"Something broke :/"})
	}
}

func nameFor(u *auth.User) string {
	if u == nil {
		return ""
	}
	if u.FullName != "" {
		return u.FullName
	}
	return u.DisplayName()
}
