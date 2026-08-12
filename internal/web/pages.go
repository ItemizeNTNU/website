package web

import (
	"net/http"

	"github.com/ItemizeNTNU/website/internal/auth"
)

// staticPage describes a page whose content lives entirely in its template.
type staticPage struct {
	route    string
	template string
	navKey   string
	title    string
	desc     string
	// command is the shell line the page opens with. It names what the page
	// holds rather than decorating it: a directory listing is ls, a document
	// is cat.
	command string
}

// staticPages are the pages with no data behind them beyond the shared
// content files. Registering them from a table keeps the route, the template
// and the active navigation key in one place, where a mismatch is visible.
var staticPages = []staticPage{
	{
		route: "/om-itemize", template: "om-itemize", navKey: "om-itemize",
		title: "Om Itemize", command: "cat om-itemize.md",
		desc: "Itemize NTNU er en frivillig interesseorganisasjon for studenter og " +
			"ansatte ved NTNU Trondheim som vil lære om informasjonssikkerhet og hacking.",
	},
	{
		route: "/historie", template: "historie", navKey: "historie",
		title: "Historie", command: "cat historie.md",
		desc: "Itemize NTNU ble startet i oktober 2014. Dette er historien.",
	},
	{
		route: "/for-bedrifter", template: "for-bedrifter", navKey: "for-bedrifter",
		title: "For Bedrifter", command: "cat for-bedrifter.md",
		desc: "Itemize NTNU samarbeider gjerne med bedrifter om kurs, " +
			"CTF-konkurranser og andre arrangementer.",
	},
	{
		route: "/ressurser", template: "ressurser", navKey: "ressurser",
		title: "Ressurser", command: "ls -la ressurser/",
		desc: "Lenker til CTF-plattformer, YouTube-kanaler og verktøy vi anbefaler.",
	},
}

func (s *Server) registerStaticPages(mux *http.ServeMux) {
	for _, p := range staticPages {
		mux.Handle("GET "+p.route, s.staticHandler(p))
	}
	mux.HandleFunc("GET /utmelding", s.utmelding)
	mux.HandleFunc("GET /registrert", s.registrert)
}

func (s *Server) staticHandler(p staticPage) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := s.page(r, p.title, p.navKey)
		page.Desc = p.desc
		page.Command = p.command
		s.render(w, r, http.StatusOK, p.template, page)
	})
}

// resignationSubject and resignationBody are the email template offered on
// /utmelding. Kept verbatim from the previous site — a member copying this is
// making a formal request, so the wording matters.
const (
	resignationSubject = "Utmelding av Itemize NTNU"
	resignationBody    = "Hei.\n\n" +
		"Jeg ønsker å melde meg ut av Itemize NTNU. " +
		"Jeg bekrefter at jeg ønsker all brukerinformasjon slettet.\n\n" +
		"Hilsen XXXX"
)

// registrertView is the confirmation page's data. OfferDiscord gates the
// "link Discord now" panel: true only when the integration is configured and
// the sealed registration cookie proves a signup just happened here — anyone
// else typing /registrert into the address bar gets the plain confirmation,
// with no button leading into a flow that would bounce them straight back.
type registrertView struct {
	Page
	OfferDiscord bool
}

// registrert confirms a completed signup. Someone already signed in has no
// business here, matching the previous site.
func (s *Server) registrert(w http.ResponseWriter, r *http.Request) {
	if auth.FromRequest(r) != nil {
		http.Redirect(w, r, "/profil", http.StatusSeeOther)
		return
	}
	view := registrertView{Page: s.page(r, "Vellykket brukerregistrering!", "registrert")}
	view.Command = "./register --confirm"
	if s.discordSvc.Available() {
		_, view.OfferDiscord = s.readRegLinkCookie(r)
	}
	s.render(w, r, http.StatusOK, "registrert", view)
}

type utmeldingView struct {
	Page
	Subject string
	Body    string
}

func (s *Server) utmelding(w http.ResponseWriter, r *http.Request) {
	view := utmeldingView{
		Page:    s.page(r, "Utmelding", "utmelding"),
		Subject: resignationSubject,
		Body:    resignationBody,
	}
	view.Desc = "Slik melder du deg ut av Itemize NTNU."
	view.Command = "cat utmelding.md"
	s.render(w, r, http.StatusOK, "utmelding", view)
}
