package web

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/ItemizeNTNU/website/content"
)

// Server renders the HTML side of the site.
type Server struct {
	renderer *Renderer
	assets   AssetResolver
	site     *content.Site
	log      *slog.Logger
	dev      bool
}

// NewServer parses the templates and loads the editable content.
func NewServer(fsys fs.FS, assets AssetResolver, log *slog.Logger, dev bool) (*Server, error) {
	site, err := content.Load(dev)
	if err != nil {
		return nil, err
	}
	renderer, err := NewRenderer(fsys, Funcs(assets), dev, log)
	if err != nil {
		return nil, err
	}
	return &Server{renderer: renderer, assets: assets, site: site, log: log, dev: dev}, nil
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
	s.registerStaticPages(mux)
	s.registerRedirects(mux)

	// Everything that does not match a more specific pattern.
	mux.HandleFunc("GET /", s.NotFound)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	page := s.page(r, "", "index")
	page.Desc = "Itemize NTNU er en studentorganisasjon ved NTNU Trondheim for " +
		"informasjonssikkerhet, hacking og CTF-konkurranser."
	s.render(w, r, http.StatusOK, "index", page)
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
