package web

import "net/http"

// staticPage describes a page whose content lives entirely in its template.
type staticPage struct {
	route    string
	template string
	navKey   string
	title    string
	desc     string
}

// staticPages are the pages with no data behind them beyond the shared
// content files. Registering them from a table keeps the route, the template
// and the active navigation key in one place, where a mismatch is visible.
var staticPages = []staticPage{
	{
		route: "/om-itemize", template: "om-itemize", navKey: "om-itemize",
		title: "Om Itemize",
		desc: "Itemize NTNU er en frivillig interesseorganisasjon for studenter og " +
			"ansatte ved NTNU Trondheim som vil lære om informasjonssikkerhet og hacking.",
	},
	{
		route: "/historie", template: "historie", navKey: "historie",
		title: "Historie",
		desc:  "Itemize NTNU ble startet i oktober 2014. Dette er historien.",
	},
	{
		route: "/for-bedrifter", template: "for-bedrifter", navKey: "for-bedrifter",
		title: "For Bedrifter",
		desc: "Itemize NTNU samarbeider gjerne med bedrifter om kurs, " +
			"CTF-konkurranser og andre arrangementer.",
	},
	{
		route: "/ressurser", template: "ressurser", navKey: "ressurser",
		title: "Ressurser",
		desc:  "Lenker til CTF-plattformer, YouTube-kanaler og verktøy vi anbefaler.",
	},
	{
		route: "/registrert", template: "registrert", navKey: "registrert",
		title: "Vellykket brukerregistrering!",
	},
}

func (s *Server) registerStaticPages(mux *http.ServeMux) {
	for _, p := range staticPages {
		mux.Handle("GET "+p.route, s.staticHandler(p))
	}
	mux.HandleFunc("GET /utmelding", s.utmelding)
}

func (s *Server) staticHandler(p staticPage) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := s.page(r, p.title, p.navKey)
		page.Desc = p.desc
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
	s.render(w, r, http.StatusOK, "utmelding", view)
}
