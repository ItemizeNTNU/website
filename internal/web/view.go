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

// NavItems is the main navigation, rendered as the window's tabs. It lives in
// Go rather than in the content YAML because adding an entry means adding a
// handler, and keeping the two together is what stops a tab pointing at a 404.
//
// The labels lost their surrounding slashes when they became tabs: a tab strip
// already reads as a set of places, and six labels wrapped in punctuation was
// noise competing with the active-tab marker.
var NavItems = []NavItem{
	{Key: "index", Label: "~", Href: "/"},
	{Key: "om-itemize", Label: "om_itemize", Href: "/om-itemize"},
	{Key: "historie", Label: "historie", Href: "/historie"},
	{Key: "for-bedrifter", Label: "for_bedrifter", Href: "/for-bedrifter"},
	{Key: "arrangementer", Label: "arrangementer", Href: "/arrangementer"},
	{Key: "ressurser", Label: "ressurser", Href: "/ressurser"},
}

// NavIndex returns the position of a nav key, or -1. The tab strip uses it to
// work out what the previous and next tabs are.
func NavIndex(key string) int {
	for i, item := range NavItems {
		if item.Key == key {
			return i
		}
	}
	return -1
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

	// Command is the shell line the page opens with — "cat historie.md" — and
	// names what the page is. It is typed out on arrival, which is what makes
	// changing page read as a session continuing rather than a document being
	// replaced.
	Command string
	// Path is the working directory shown in the status line.
	Path string

	NavItems []NavItem
	Site     *content.Site
	User     *auth.User
	Flash    []Toast
	CSRF     string
}
