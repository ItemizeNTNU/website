// Package api serves the JSON and calendar endpoints.
//
// These exist because things outside this repository depend on them: the iCal
// feed is subscribed in people's calendars, and the URL has to keep working
// across the rewrite whatever else changes.
package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/events"
	"github.com/ItemizeNTNU/website/internal/fusionauth"
	"github.com/ItemizeNTNU/website/internal/httpx"
	"github.com/ItemizeNTNU/website/internal/ical"
)

// Server holds the API's dependencies.
type Server struct {
	events      events.Repository
	fusion      *fusionauth.Client
	baseURL     string
	signupLimit *httpx.RateLimiter
	log         *slog.Logger
}

// NewServer builds the API handlers.
func NewServer(repo events.Repository, fusion *fusionauth.Client, baseURL string, log *slog.Logger) *Server {
	return &Server{
		events:      repo,
		fusion:      fusion,
		baseURL:     baseURL,
		signupLimit: httpx.NewRateLimiter(5, time.Hour),
		log:         log,
	}
}

// Routes registers the API on mux.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/events", s.listEvents)
	mux.HandleFunc("GET /api/events/ical", s.icalFeed)

	// Reading the register exposes the check-in code and every attendee's
	// identity, so it is board-only. Writing to it only needs to be somebody.
	mux.Handle("GET /api/checkin/{code}",
		auth.RequireRoleAPI(auth.RoleStyret)(http.HandlerFunc(s.getCheckIn)))
	mux.Handle("POST /api/checkin/{code}",
		auth.RequireLoginAPI(http.HandlerFunc(s.postCheckIn)))

	// Requires a login. The response carries an email address and a legal
	// name, and identifiers leak into places members can see — so serving it
	// to anybody, as the previous site did, made the directory readable to
	// anyone who collected one.
	mux.Handle("GET /api/user/{id}",
		auth.RequireLoginAPI(http.HandlerFunc(s.getUser)))
	// Rate limited for the same reason as the form: this causes FusionAuth to
	// send mail to an address the caller chooses.
	mux.Handle("PUT /api/user", s.signupLimit.Limit(http.HandlerFunc(s.registerUser)))

	// Anything else under /api answers as JSON rather than falling through to
	// the HTML 404 page, matching the previous behaviour.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, message{"API endpoint not found"})
	})
}

// eventDTO is the wire shape. It is deliberately separate from the domain
// struct: this is where identifiers become strings, times become ISO-8601, and
// the check-in block is withheld from anyone who is not on the board.
type eventDTO struct {
	ID             string      `json:"_id"`
	Name           string      `json:"name"`
	Location       placeDTO    `json:"location"`
	RegisterURL    string      `json:"register_url"`
	Date           time.Time   `json:"date"`
	Duration       float64     `json:"duration"`
	End            time.Time   `json:"end"`
	CTF            placeDTO    `json:"ctf"`
	Info           string      `json:"info"`
	Hidden         bool        `json:"hidden"`
	Discord        bool        `json:"discord"`
	DiscordEventID string      `json:"discordEventId,omitempty"`
	Created        time.Time   `json:"created"`
	Edited         *time.Time  `json:"edited,omitempty"`
	CheckIn        *checkInDTO `json:"check_in,omitempty"`
}

type placeDTO struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type checkInDTO struct {
	Code        string          `json:"code"`
	Attendances []attendanceDTO `json:"attendances,omitempty"`
}

type attendanceDTO struct {
	Name       string    `json:"name"`
	UserID     string    `json:"user_id"`
	Registered time.Time `json:"registered"`
}

func toDTO(e events.Event, includeCheckIn bool) eventDTO {
	dto := eventDTO{
		ID:             e.HexID(),
		Name:           e.Name,
		Location:       placeDTO{e.Location.Name, e.Location.URL},
		RegisterURL:    e.RegisterURL,
		Date:           e.Date,
		Duration:       e.Duration,
		End:            e.End,
		CTF:            placeDTO{e.CTF.Name, e.CTF.URL},
		Info:           e.Info,
		Hidden:         e.Hidden,
		Discord:        e.Discord,
		DiscordEventID: e.DiscordEventID,
		Created:        e.Created,
		Edited:         e.Edited,
	}
	if includeCheckIn {
		ci := &checkInDTO{Code: e.CheckIn.Code}
		for _, a := range e.CheckIn.Attendances {
			ci.Attendances = append(ci.Attendances, attendanceDTO{
				Name: a.Name, UserID: a.UserID, Registered: a.Registered,
			})
		}
		dto.CheckIn = ci
	}
	return dto
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	user := auth.FromRequest(r)
	styret := user.IsStyret()

	filter := events.Filter{
		IncludeHidden: styret,
		IncludeOld:    truthy(r.URL.Query().Get("old"), r.URL.Query().Has("old")),
		Page:          pageParam(r.URL.Query().Get("page")) - 1, // the query parameter is one-based
	}

	list, err := s.events.List(r.Context(), filter)
	if err != nil {
		s.log.Error("listing events failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, message{"Something broke :/"})
		return
	}

	// The check-in code is the credential that registers attendance. Serving
	// it to anonymous callers — which the previous version did — let anyone
	// list the events, collect the codes and check themselves in without
	// turning up.
	out := make([]eventDTO, 0, len(list))
	for _, e := range list {
		out = append(out, toDTO(e, styret))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) icalFeed(w http.ResponseWriter, r *http.Request) {
	list, err := s.events.Public(r.Context())
	if err != nil {
		s.log.Error("building calendar feed failed", "err", err)
		http.Error(w, "Kunne ikke bygge kalenderen", http.StatusInternalServerError)
		return
	}

	cal := ical.New("Itemize Arrangementer")
	for _, e := range list {
		stamp := e.Edited
		if stamp == nil {
			stamp = &e.Created
		}
		cal.Add(ical.Event{
			UID:   ical.UID(e.HexID()),
			Start: e.Date,
			// Derived rather than read from the document: legacy events stored
			// before `end` existed have a zero End, and a VEVENT without DTEND
			// is open-ended in most clients. ComputeEnd falls back to midnight
			// in Oslo after the start for exactly this feed's benefit.
			End:         e.ComputeEnd(),
			Stamp:       *stamp,
			Summary:     e.Name,
			Description: e.Info,
			Location:    e.Location.Name,
			URL:         s.baseURL + "/arrangementer",
		})
	}

	w.Header().Set("Content-Type", ical.ContentType)
	w.Header().Set("Content-Disposition", `inline; filename="itemize.ics"`)
	// Subscribers poll this; an hour is well inside how often events change.
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(cal.String()))
}

// maxPage bounds the one-based page number a caller may ask for.
//
// The repository multiplies the page by events.PageSize to get a skip, as an
// int. Without a bound, ?page=99999999999999999999 reaches that multiplication
// as the largest int — strconv.Atoi returns the clamped maximum alongside its
// error — and the product wraps to a negative skip, which the driver rejects.
// That turned a crafted query string from an anonymous caller into a 500.
//
// Anything at or past this bound lands beyond the end of the collection and
// answers with an empty list, which is already what any other too-high page
// does, so the clamp changes nothing a real caller can observe.
const maxPage = 1_000_000

// pageParam turns the ?page argument into a one-based page number in [1,
// maxPage]. Anything unparseable — absent, empty, "abc", "1.5" — is the first
// page, which is what the previous API did, and so is any number below one.
func pageParam(raw string) int {
	page, err := strconv.Atoi(raw)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		page = 1
	}
	// A range error keeps Atoi's clamped value, which the bounds below turn
	// into the first page for a huge negative and the last for a huge positive.
	return min(max(page, 1), maxPage)
}

// truthy reproduces the previous API's argument handling, where ?old,
// ?old=true and ?old=1 all mean the same thing.
func truthy(v string, present bool) bool {
	if !present {
		return false
	}
	switch v {
	case "", "1", "true", "yes":
		return true
	}
	return false
}
