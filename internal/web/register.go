package web

import (
	"errors"
	"net/http"
	"time"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/fusionauth"
	"github.com/ItemizeNTNU/website/internal/users"
	"github.com/ItemizeNTNU/website/internal/validate"
)

// upstreamDownMessage is what somebody sees when the registration failed on
// our side of the form: no service address, no status code, and advice worth
// following — what they typed was fine. Kept word for word in step with the
// JSON path in internal/api/users.go so the same outage reads the same way
// whichever entry point a member came through.
const upstreamDownMessage = "Innloggingstjenesten svarer ikke akkurat nå. Prøv igjen om litt."

type registerView struct {
	Page
	Form   map[string]string
	Type   string
	Errors validate.Errors
	// Unavailable is set when no API token is configured, so the page explains
	// itself instead of failing on submit.
	Unavailable bool
	MaxJoinYear int
	MinFinish   int
	MaxFinish   int
}

func (s *Server) newRegisterView(r *http.Request) registerView {
	now := time.Now()
	return registerView{
		Page:        s.page(r, "Bli medlem!", "registrer"),
		Form:        map[string]string{},
		Type:        users.TypeStudent,
		Unavailable: !s.fusion.Configured(),
		MaxJoinYear: now.Year(),
		MinFinish:   now.Year(),
		MaxFinish:   now.Year() + 15,
	}
}

// registrer shows the signup form.
func (s *Server) registrer(w http.ResponseWriter, r *http.Request) {
	// Someone already signed in has nothing to do here.
	if auth.FromRequest(r) != nil {
		http.Redirect(w, r, "/profil", http.StatusSeeOther)
		return
	}

	view := s.newRegisterView(r)
	view.Desc = "Bli medlem av Itemize NTNU."
	view.Command = "./register --new"
	view.CSRF = s.csrf(w, r)
	s.render(w, r, http.StatusOK, "registrer", view)
}

// submitRegistration creates the member in FusionAuth.
func (s *Server) submitRegistration(w http.ResponseWriter, r *http.Request) {
	if auth.FromRequest(r) != nil {
		http.Redirect(w, r, "/profil", http.StatusSeeOther)
		return
	}

	view := s.newRegisterView(r)
	view.CSRF = s.csrf(w, r)
	view.Form = submittedFields(r)
	if t := r.PostFormValue("type"); t != "" {
		view.Type = t
	}

	if view.Unavailable {
		s.render(w, r, http.StatusServiceUnavailable, "registrer", view)
		return
	}

	reg, verr := users.FromForm(r.PostForm, time.Now())
	if verr.Any() {
		view.Errors = verr
		s.render(w, r, http.StatusUnprocessableEntity, "registrer", view)
		return
	}

	created, err := s.fusion.CreateUser(r.Context(), reg.ToFusionAuth())
	if err != nil {
		// FusionAuth's parser wraps every non-2xx reply in *APIError, a 5xx
		// included, so the status has to be checked as well. Without that an
		// outage upstream came back as a 422 on the form, telling somebody whose
		// details were fine to correct them — and hiding the outage from
		// anything watching for 5xx. Same split as internal/api/users.go.
		var apiErr *fusionauth.APIError
		if errors.As(err, &apiErr) && apiErr.Status < http.StatusInternalServerError {
			// FusionAuth's own message is the useful one here — "email already
			// in use" is something the person can act on.
			view.Errors = validate.Errors{"": apiErr.UserMessage()}
			s.render(w, r, http.StatusUnprocessableEntity, "registrer", view)
			return
		}
		s.log.Error("creating the user failed", "err", err)
		view.Errors = validate.Errors{"": upstreamDownMessage}
		s.render(w, r, http.StatusInternalServerError, "registrer", view)
		return
	}

	// The membership now exists; nothing below may stop the confirmation. The
	// sealed cookie lets /registrert offer linking Discord right away, without
	// a session — sealing failures are logged and swallowed inside the helper,
	// because the convenience of that offer is worth strictly less than the
	// registration it rides on. With Discord unconfigured no cookie is set and
	// this redirect is all that happens, exactly as before.
	if s.discordSvc.Available() {
		s.setRegLinkCookie(w, created.ID)
	}

	http.Redirect(w, r, "/registrert", http.StatusSeeOther)
}

// submittedFields echoes the submission back so a rejected registration does
// not make somebody retype the whole form.
func submittedFields(r *http.Request) map[string]string {
	fields := map[string]string{}
	for _, name := range []string{
		"fullName", "email", "displayName",
		"study.program", "study.year", "study.expectedFinishYear",
		"alumni.joinYear", "employee.title",
	} {
		fields[name] = r.PostFormValue(name)
	}
	return fields
}
