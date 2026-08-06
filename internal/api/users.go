package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/fusionauth"
	"github.com/ItemizeNTNU/website/internal/users"
)

// defaultProfileImage is what a member without an avatar gets.
const defaultProfileImage = "/logo-512.png"

type userDTO struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"fullName"`
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl"`
	Type     string `json:"type,omitempty"`
	Discord  string `json:"discord,omitempty"`
}

// getUser returns a member's public profile.
func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	if !s.fusion.Configured() {
		writeJSON(w, http.StatusServiceUnavailable, message{"User directory is unavailable"})
		return
	}

	u, err := s.fusion.GetUser(r.Context(), r.PathValue("id"))
	if errors.Is(err, fusionauth.ErrInvalidID) {
		// Indistinguishable from "no such user", and answering differently
		// would confirm the identifier format to anyone probing.
		writeJSON(w, http.StatusNotFound, message{"User not found"})
		return
	}
	if errors.Is(err, fusionauth.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, message{"User not found"})
		return
	}
	if err != nil {
		s.log.Error("fetching the user failed", "err", err)
		writeJSON(w, http.StatusBadGateway, message{"Error fetching user"})
		return
	}

	dto := userDTO{ID: u.ID, Email: u.Email, FullName: u.FullName, ImageURL: u.ImageURL}
	if dto.ImageURL == "" {
		dto.ImageURL = defaultProfileImage
	}
	// The previous version assigned displayName = fullName || fullName, which
	// discarded the display name entirely and always showed the legal name.
	dto.Name = str(u.Data, "displayName")
	if dto.Name == "" {
		dto.Name = u.FullName
	}
	dto.Type = str(u.Data, "type")
	if discord, ok := u.Data["discord"].(map[string]any); ok {
		dto.Discord, _ = discord["username"].(string)
	}

	writeJSON(w, http.StatusOK, dto)
}

// registerUser creates a member from a JSON body, preserving the previous
// API's contract for anything still calling it.
func (s *Server) registerUser(w http.ResponseWriter, r *http.Request) {
	if auth.FromRequest(r) != nil {
		writeJSON(w, http.StatusBadRequest, message{"You are already registered"})
		return
	}
	if !s.fusion.Configured() {
		writeJSON(w, http.StatusServiceUnavailable, message{"Registration is unavailable"})
		return
	}

	// The previous endpoint accepted the FusionAuth user shape directly. It is
	// flattened into form values here so both entry points run the identical
	// validation rather than drifting apart.
	var body struct {
		FullName string `json:"fullName"`
		Email    string `json:"email"`
		Data     struct {
			DisplayName string `json:"displayName"`
			Type        string `json:"type"`
			Study       struct {
				Program            string `json:"program"`
				Year               any    `json:"year"`
				ExpectedFinishYear string `json:"expectedFinishYear"`
			} `json:"study"`
			Alumni struct {
				JoinYear any `json:"joinYear"`
			} `json:"alumni"`
			Employee struct {
				Title string `json:"title"`
			} `json:"employee"`
		} `json:"data"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, message{"Invalid request body"})
		return
	}

	form := map[string][]string{
		"fullName":                 {body.FullName},
		"email":                    {body.Email},
		"displayName":              {body.Data.DisplayName},
		"type":                     {body.Data.Type},
		"study.program":            {body.Data.Study.Program},
		"study.year":               {num(body.Data.Study.Year)},
		"study.expectedFinishYear": {yearOf(body.Data.Study.ExpectedFinishYear)},
		"alumni.joinYear":          {num(body.Data.Alumni.JoinYear)},
		"employee.title":           {body.Data.Employee.Title},
	}

	reg, verr := users.FromForm(form, time.Now())
	if verr.Any() {
		writeJSON(w, http.StatusBadRequest, message{verr.First()})
		return
	}

	if _, err := s.fusion.CreateUser(r.Context(), reg.ToFusionAuth()); err != nil {
		var apiErr *fusionauth.APIError
		if errors.As(err, &apiErr) {
			writeJSON(w, http.StatusBadRequest, message{apiErr.UserMessage()})
			return
		}
		s.log.Error("creating the user failed", "err", err)
		writeJSON(w, http.StatusBadGateway, message{"Ups. Noe gikk galt :/"})
		return
	}

	writeJSON(w, http.StatusOK, message{"Success"})
}

func str(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// num renders a JSON number, which decodes as float64, without a decimal tail.
func num(v any) string {
	switch n := v.(type) {
	case float64:
		return strconv.Itoa(int(n))
	case string:
		return n
	}
	return ""
}

// yearOf accepts either a bare year or the ISO date the previous API sent.
func yearOf(v string) string {
	if len(v) >= 4 {
		return v[:4]
	}
	return v
}
