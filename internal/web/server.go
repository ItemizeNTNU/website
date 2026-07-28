package web

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ItemizeNTNU/website/content"
	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/events"
	"github.com/ItemizeNTNU/website/internal/fusionauth"
	"github.com/ItemizeNTNU/website/internal/httpx"
	"github.com/ItemizeNTNU/website/internal/users"
)

// Server renders the HTML side of the site.
type Server struct {
	renderer *Renderer
	assets   AssetResolver
	site     *content.Site
	// events may be nil when the database is unreachable. Pages that need it
	// say so rather than pretending the calendar is empty.
	events     events.Repository
	eventSvc   *events.Service
	fusion     *fusionauth.Client
	discordSvc *users.DiscordService
	baseURL    string
	// secureCookies mirrors whether the site is served over TLS.
	secureCookies bool
	// signupLimit throttles the endpoints that cause an email to be sent.
	signupLimit *httpx.RateLimiter
	log         *slog.Logger
	dev         bool
}

// NewServer parses the templates and loads the editable content.
func NewServer(fsys fs.FS, assets AssetResolver, repo events.Repository, svc *events.Service, fusion *fusionauth.Client, discordSvc *users.DiscordService, baseURL string, log *slog.Logger, dev bool) (*Server, error) {
	site, err := content.Load(dev)
	if err != nil {
		return nil, err
	}
	renderer, err := NewRenderer(fsys, Funcs(assets), dev, log)
	if err != nil {
		return nil, err
	}
	return &Server{
		renderer:      renderer,
		assets:        assets,
		site:          site,
		events:        repo,
		eventSvc:      svc,
		fusion:        fusion,
		discordSvc:    discordSvc,
		baseURL:       baseURL,
		secureCookies: strings.HasPrefix(baseURL, "https://"),
		// Generous for a person filling in a form, useless for a script.
		signupLimit: httpx.NewRateLimiter(5, time.Hour),
		log:         log,
		dev:         dev,
	}, nil
}

// csrf issues (or reuses) the token for a page that carries a form.
func (s *Server) csrf(w http.ResponseWriter, r *http.Request) string {
	return auth.CSRFToken(w, r)
}

// page builds the data every template expects, so no handler has to remember
// to thread the current user or the navigation through by hand.
func (s *Server) page(r *http.Request, title, navKey string) Page {
	site := s.site
	if s.dev {
		// Pick up content edits without a restart. A broken file during
		// development should not take the page down, so the last good copy
		// stays in use and the error is logged.
		if fresh, err := content.Load(true); err != nil {
			s.log.Error("reloading content failed, serving previous copy", "err", err)
		} else {
			site = fresh
		}
	}
	return Page{
		Title:     title,
		Nav:       navKey,
		BodyClass: "p-" + navKey,
		NavItems:  NavItems,
		Site:      site,
		User:      userFrom(r),
		Flash:     takeFlash(r),
	}
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, page string, data any) {
	// Must happen before rendering: the asset fingerprints are written into
	// the HTML, so a table rebuilt only on the asset request would leave every
	// page pointing at the previous hash.
	if err := s.assets.Refresh(); err != nil {
		s.log.Error("rebuilding assets failed, serving previous copy", "err", err)
	}
	s.renderer.Render(w, r, status, page, data)
}

// errorView is the data for the error page.
type errorView struct {
	Page
	Status     int
	StatusText string
	Message    string
}

// norwegianStatus maps the handful of statuses a visitor can actually reach to
// something readable. Anything else falls back to the English reason phrase.
var norwegianStatus = map[int][2]string{
	http.StatusNotFound: {
		"Ikke funnet",
		"Denne siden finnes ikke. Kanskje den har flyttet på seg?",
	},
	http.StatusForbidden: {
		"Ingen tilgang",
		"Du har ikke tilgang til denne siden.",
	},
	http.StatusUnauthorized: {
		"Ikke logget inn",
		"Du må være logget inn for å se denne siden.",
	},
	http.StatusInternalServerError: {
		"Noe gikk galt",
		"Noe gikk galt på vår side. Prøv igjen om litt.",
	},
	http.StatusMethodNotAllowed: {
		"Ugyldig metode",
		"Denne siden svarer ikke på den forespørselen.",
	},
}

// ErrorPage renders the error page. Its signature matches what the recovery
// middleware expects, so a panic renders the same page as a 404 does.
func (s *Server) ErrorPage(w http.ResponseWriter, r *http.Request, status int) {
	text, message := http.StatusText(status), http.StatusText(status)
	if v, ok := norwegianStatus[status]; ok {
		text, message = v[0], v[1]
	}

	view := errorView{
		Page:       s.page(r, text, ""),
		Status:     status,
		StatusText: text,
		Message:    message,
	}
	s.render(w, r, status, "error", view)
}

// NotFound renders the 404 page.
func (s *Server) NotFound(w http.ResponseWriter, r *http.Request) {
	s.ErrorPage(w, r, http.StatusNotFound)
}

// Routes registers the HTML routes on mux.
//
// Patterns are canonical and carry no trailing slash; httpx.TrailingSlash
// redirects the slashed form, which is what the previous site's navigation
// linked to.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /arrangementer", s.arrangementer)
	mux.Handle("GET /profil", auth.RequireLogin(http.HandlerFunc(s.profil)))

	// Event administration. Board only; the CSRF check runs on the mutating
	// routes, where a cross-site submission would otherwise be possible now
	// that these are ordinary form posts rather than scripted JSON.
	styret := auth.RequireRole(auth.RoleStyret, s.ErrorPage)
	mux.Handle("POST /arrangementer",
		styret(auth.CSRF(http.HandlerFunc(s.saveEvent))))
	mux.Handle("GET /arrangementer/{id}/slett",
		styret(http.HandlerFunc(s.confirmDelete)))
	mux.Handle("POST /arrangementer/{id}/slett",
		styret(auth.CSRF(http.HandlerFunc(s.deleteEvent))))

	// Check-in. The board holds up the QR code; a member scanning it lands on
	// the second route, which registers their attendance.
	mux.Handle("GET /innsjekk/{code}", styret(http.HandlerFunc(s.innsjekk)))
	mux.Handle("GET /innsjekk-qr/{code}",
		auth.RequireLogin(http.HandlerFunc(s.innsjekkQR)))

	// Registration. Open to anyone, so it carries the CSRF check on its own
	// rather than inheriting one from a role gate — and a rate limit, because
	// submitting it makes FusionAuth send mail to whatever address was given.
	mux.HandleFunc("GET /registrer", s.registrer)
	mux.Handle("POST /registrer",
		s.signupLimit.Limit(auth.CSRF(http.HandlerFunc(s.submitRegistration))))

	// Discord account linking. The two GETs are the OAuth round trip, so they
	// carry their own state parameter rather than a form token; the two writes
	// are ordinary form posts and take the CSRF check.
	login := auth.RequireLogin
	mux.Handle("GET /api/discord/link", login(http.HandlerFunc(s.discordLink)))
	mux.Handle("GET "+discordCallbackPath, login(http.HandlerFunc(s.discordCallback)))
	mux.Handle("POST /profil/discord/oppdater",
		login(auth.CSRF(http.HandlerFunc(s.discordRefresh))))
	mux.Handle("POST /profil/discord/koble-fra",
		login(auth.CSRF(http.HandlerFunc(s.discordUnlink))))
	s.registerStaticPages(mux)
	s.registerRedirects(mux)

	// Everything that does not match a more specific pattern.
	//
	// Registered for all methods, not just GET. A GET-only catch-all conflicts
	// with the API's own "/api/" fallback — that pattern has the more specific
	// path but matches more methods, so ServeMux can rank neither above the
	// other and panics at registration. Matching every method also means a
	// stray POST to a dead URL gets the 404 page rather than a bare 405.
	mux.HandleFunc("/", s.NotFound)
}

// brainfuckHello prints "Itemize NTNU".
//
// Carried over from the previous site, where it drifted behind the logo as
// texture. The front page now shows it being run, so it is rendered here
// rather than only assembled in script — with JavaScript off the hero is a
// finished terminal session instead of an empty frame.
const brainfuckHello = "++++++++[->++++++++<]>+++++++++.<++++++[->++++++<]>++++" +
	"+++.<+++[->---<]>------.++++++++.----.<++++[->++++<]>+.<++++[->----<]>--" +
	"---.<++++++++[->--------<]>-----.<++++++[->++++++<]>++++++++++.++++++.--" +
	"----.+++++++.<++++++++[->--------<]>--------.---.>"

type indexView struct {
	Page
	Brainfuck string
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	view := indexView{
		Page:      s.page(r, "", "index"),
		Brainfuck: brainfuckHello,
	}
	view.Desc = "Itemize NTNU er en studentorganisasjon ved NTNU Trondheim for " +
		"informasjonssikkerhet, hacking og CTF-konkurranser."
	s.render(w, r, http.StatusOK, "index", view)
}

// registerRedirects preserves the URLs the previous site answered on. They are
// permanent because they have been stable for years and are linked from
// posters, chat logs and bookmarks.
func (s *Server) registerRedirects(mux *http.ServeMux) {
	// "påmelding" is registered in both Unicode normalisations of å:
	// composed (U+00E5), which is what almost everything sends, and decomposed
	// (a + U+030A), which some macOS clients still produce. ServeMux compares
	// unescaped paths byte for byte, so one form does not match the other.
	// Written as escapes so that no editor or formatter can normalise one into
	// the other and silently collapse them into a duplicate pattern, which
	// ServeMux panics on.
	const (
		pameldingNFC = "/p\u00e5melding"
		pameldingNFD = "/pa\u030amelding"
	)

	for from, to := range map[string]string{
		"/qr":                          "/arrangementer",
		"/register":                    "/arrangementer",
		"/registrering":                "/arrangementer",
		pameldingNFC:                   "/arrangementer",
		pameldingNFD:                   "/arrangementer",
		"/den-ekte-registreringssiden": "/registrer",
		"/vedtekter":                   s.site.Contact.BylawsURL,
	} {
		mux.Handle("GET "+from, redirect(to))
	}
}

func redirect(to string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, to, http.StatusMovedPermanently)
	})
}

// Assets exposes the resolver so main can register the asset routes.
func (s *Server) Assets() AssetResolver { return s.assets }
