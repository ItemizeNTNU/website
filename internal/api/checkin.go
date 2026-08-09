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
	code := r.PathValue("code")
	event, err := s.events.ByCheckInCode(r.Context(), code)
	if err != nil {
		// A storage failure is still answered as "not found", on purpose:
		// telling the two apart would confirm to somebody working through
		// guesses which of them hit a real code. It does have to be logged
		// though — swallowing it made an outage on this path look to everyone,
		// operators included, like a mistyped code.
		if !errors.Is(err, events.ErrNotFound) {
			s.log.Error("looking up the check-in code failed", "code", code, "err", err)
		}
		writeJSON(w, http.StatusNotFound, message{"Event not found"})
		return
	}
	writeJSON(w, http.StatusOK, toDTO(*event, true))
}

// postCheckIn registers the caller's attendance.
func (s *Server) postCheckIn(w http.ResponseWriter, r *http.Request) {
	user := auth.FromRequest(r)
	if user == nil {
		// Unreachable through Routes, which mounts this behind
		// RequireLoginAPI. It is here so the handler is safe wherever it is
		// mounted: user.ID below is a nil dereference, so a future route
		// registration that forgets the wrapper would turn a missing gate into
		// a panic in the request goroutine rather than a 401.
		writeJSON(w, http.StatusUnauthorized, message{"You are not logged in"})
		return
	}
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
