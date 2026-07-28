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

	_, err := s.fusion.CreateUser(r.Context(), reg.ToFusionAuth())
	if err != nil {
		var apiErr *fusionauth.APIError
		switch {
		case errors.As(err, &apiErr):
			// FusionAuth's own message is the useful one here — "email already
			// in use" is something the person can act on.
			view.Errors = validate.Errors{"": apiErr.UserMessage()}
			s.render(w, r, http.StatusUnprocessableEntity, "registrer", view)
		default:
			s.log.Error("creating the user failed", "err", err)
			view.Errors = validate.Errors{"": "Ups. Noe gikk galt :/"}
			s.render(w, r, http.StatusInternalServerError, "registrer", view)
		}
		return
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
