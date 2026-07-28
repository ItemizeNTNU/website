package web

import (
	"github.com/ItemizeNTNU/website/content"
	"github.com/ItemizeNTNU/website/internal/auth"
)

// NavItem is one entry in the main navigation.
//
// The labels are rendered as filesystem paths — the previous site derived them
// by substituting characters at runtime, which is preserved here explicitly so
// the terminal motif survives without the string surgery. They pair with the
// "> _" prompt on the wordmark.
type NavItem struct {
	Key   string
	Label string
	Href  string
}

// NavItems is the main navigation. It lives in Go rather than in the content
// YAML because adding an entry means adding a handler, and keeping the two
// together is what stops a nav link pointing at a 404.
var NavItems = []NavItem{
	{Key: "index", Label: "~/", Href: "/"},
	{Key: "om-itemize", Label: "/om_itemize/", Href: "/om-itemize"},
	{Key: "historie", Label: "/historie/", Href: "/historie"},
	{Key: "for-bedrifter", Label: "/for_bedrifter/", Href: "/for-bedrifter"},
	{Key: "arrangementer", Label: "/arrangementer/", Href: "/arrangementer"},
	{Key: "ressurser", Label: "/ressurser/", Href: "/ressurser"},
}

// Toast is a transient message shown after a redirect.
type Toast struct {
	Kind string // success, error, info, warning
	Text string
}

// Page is the data every template can rely on. Page-specific view models
// embed it, so a template sees .User and its own fields at the same level.
type Page struct {
	// Title is the page's own name; the layout appends the site name.
	Title string
	// Desc fills the meta description when set.
	Desc string
	// Nav is the key of the active navigation entry.
	Nav string
	// BodyClass scopes page-specific styling, e.g. "p-historie".
	BodyClass string

	NavItems []NavItem
	Site     *content.Site
	User     *auth.User
	Flash    []Toast
	CSRF     string
}
