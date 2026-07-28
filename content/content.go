// Package content holds the parts of the site that change without the code
// changing: the board, the resource directory, contact details and social
// links.
//
// These live as YAML rather than inside templates so that updating the board
// after a general assembly is a one-file, reviewable edit that needs no Go
// knowledge. YAML rather than JSON for two reasons that matter to whoever
// edits it: it takes comments — including the one warning not to delete a
// certain hidden value — and it takes folded block scalars, so a paragraph of
// Norwegian stays readable and diffable instead of collapsing to one escaped
// line.
//
// Everything is validated at startup. A malformed URL fails the deploy rather
// than rendering a broken page.
package content

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed *.yaml
var embedded embed.FS

// Person is one member of the board.
type Person struct {
	Name     string `yaml:"name"`
	Position string `yaml:"position"`
	Mail     string `yaml:"mail"`
}

// Social is one external profile shown in the footer.
type Social struct {
	// Icon must match a <symbol id="i-…"> in the icon sprite; this is checked
	// at startup so a typo cannot ship as an invisible link.
	Icon  string `yaml:"icon"`
	Label string `yaml:"label"`
	URL   string `yaml:"url"`
}

// Contact holds the organisation's addresses and identifiers.
type Contact struct {
	Email           string `yaml:"email"`
	InvoiceEmail    string `yaml:"invoice_email"`
	CompanyEmail    string `yaml:"company_email"`
	MembershipEmail string `yaml:"membership_email"`
	OrgNumber       string `yaml:"org_number"`
	DiscordInvite   string `yaml:"discord_invite"`
	BylawsURL       string `yaml:"bylaws_url"`

	// The site renders the postal address two ways — abbreviated in the
	// footer, spelled out on the about page — so both are stored rather than
	// one being derived from the other.
	AddressShort []string `yaml:"address_short"`
	AddressFull  []string `yaml:"address_full"`
}

// Para is one paragraph of a resource description.
//
// Some descriptions contain their own links, so a plain string would either
// lose them or open a hole for raw HTML from a data file. Instead the text
// carries {label} placeholders and Refs maps each label to a URL; HTML escapes
// every literal run and turns only the placeholders into anchors, which makes
// injection structurally impossible rather than merely unlikely.
type Para struct {
	Text string            `yaml:"text"`
	Refs map[string]string `yaml:"refs,omitempty"`
}

// HTML renders the paragraph with its inline links resolved.
func (p Para) HTML() template.HTML {
	var b strings.Builder
	rest := p.Text
	for {
		before, after, found := strings.Cut(rest, "{")
		template.HTMLEscape(&b, []byte(before))
		if !found {
			break
		}
		label, tail, closed := strings.Cut(after, "}")
		if !closed {
			// An unmatched brace is literal text, not a broken placeholder.
			b.WriteString("{")
			rest = after
			continue
		}
		if href, ok := p.Refs[label]; ok {
			b.WriteString(`<a href="`)
			template.HTMLEscape(&b, []byte(href))
			b.WriteString(`" rel="noopener noreferrer">`)
			template.HTMLEscape(&b, []byte(label))
			b.WriteString(`</a>`)
		} else {
			b.WriteString("{")
			template.HTMLEscape(&b, []byte(label))
			b.WriteString("}")
		}
		rest = tail
	}
	return template.HTML(b.String())
}

// Link is one entry in the resource directory.
type Link struct {
	Title      string `yaml:"title"`
	URL        string `yaml:"url"`
	Paragraphs []Para `yaml:"paragraphs"`
	// HiddenNote is rendered into the page but not shown. There is exactly one
	// of these and it is deliberate — see the comment in ressurser.yaml.
	HiddenNote string `yaml:"hidden_note,omitempty"`
}

// Category groups resource links under an addressable heading.
type Category struct {
	// ID is the anchor. Existing values are kept verbatim, typos included,
	// because they may be linked from outside.
	ID       string `yaml:"id"`
	Heading  string `yaml:"heading"`
	NavLabel string `yaml:"nav_label"`
	Links    []Link `yaml:"links"`
}

// Site is everything loaded from content/.
type Site struct {
	Contact    Contact
	Socials    []Social
	Board      []Person
	Categories []Category
}

// KnownIcons is the set of icon names the sprite defines. Set by the web
// package at startup so social entries can be validated against it.
var KnownIcons = map[string]bool{
	"facebook":  true,
	"instagram": true,
	"discord":   true,
	"github":    true,
	"mail":      true,
	"link":      true,
}

var (
	once   sync.Once
	loaded *Site
	loadNo error
)

// Load reads and validates the content files. In dev mode they are read from
// disk on every call so edits appear on reload.
func Load(dev bool) (*Site, error) {
	if dev {
		return load(os.DirFS("content"))
	}
	once.Do(func() { loaded, loadNo = load(embedded) })
	return loaded, loadNo
}

func load(fsys fs.FS) (*Site, error) {
	var site Site

	if err := unmarshal(fsys, "kontakt.yaml", &site.Contact); err != nil {
		return nil, err
	}
	var socials struct {
		Socials []Social `yaml:"socials"`
	}
	if err := unmarshal(fsys, "socials.yaml", &socials); err != nil {
		return nil, err
	}
	site.Socials = socials.Socials

	var board struct {
		Board []Person `yaml:"board"`
	}
	if err := unmarshal(fsys, "styret.yaml", &board); err != nil {
		return nil, err
	}
	site.Board = board.Board

	var resources struct {
		Categories []Category `yaml:"categories"`
	}
	if err := unmarshal(fsys, "ressurser.yaml", &resources); err != nil {
		return nil, err
	}
	site.Categories = resources.Categories

	if err := site.validate(); err != nil {
		return nil, err
	}
	return &site, nil
}

func unmarshal(fsys fs.FS, name string, into any) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("reading content/%s: %w", name, err)
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	// Reject unknown keys: a misspelt field would otherwise vanish silently.
	dec.KnownFields(true)
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("parsing content/%s: %w", name, err)
	}
	return nil
}

func (s *Site) validate() error {
	var problems []string

	for field, addr := range map[string]string{
		"email":            s.Contact.Email,
		"invoice_email":    s.Contact.InvoiceEmail,
		"company_email":    s.Contact.CompanyEmail,
		"membership_email": s.Contact.MembershipEmail,
	} {
		if !strings.Contains(addr, "@") {
			problems = append(problems,
				fmt.Sprintf("kontakt.yaml: %s is not an email address: %q", field, addr))
		}
	}
	if len(s.Contact.AddressShort) == 0 || len(s.Contact.AddressFull) == 0 {
		problems = append(problems, "kontakt.yaml: both address_short and address_full are required")
	}
	for field, raw := range map[string]string{
		"discord_invite": s.Contact.DiscordInvite,
		"bylaws_url":     s.Contact.BylawsURL,
	} {
		if err := checkURL(raw); err != nil {
			problems = append(problems, fmt.Sprintf("kontakt.yaml: %s: %v", field, err))
		}
	}

	if len(s.Socials) == 0 {
		problems = append(problems, "socials.yaml: no socials defined")
	}
	for i, social := range s.Socials {
		if !KnownIcons[social.Icon] {
			problems = append(problems, fmt.Sprintf(
				"socials.yaml: entry %d uses unknown icon %q — it must match a <symbol id=\"i-…\"> in partials/icons.html",
				i+1, social.Icon))
		}
		if social.Label == "" {
			problems = append(problems, fmt.Sprintf("socials.yaml: entry %d has no label", i+1))
		}
		if err := checkURL(social.URL); err != nil {
			problems = append(problems, fmt.Sprintf("socials.yaml: entry %d: %v", i+1, err))
		}
	}

	if len(s.Board) == 0 {
		problems = append(problems, "styret.yaml: no board members defined")
	}
	for i, p := range s.Board {
		if p.Name == "" || p.Position == "" {
			problems = append(problems, fmt.Sprintf("styret.yaml: entry %d needs both name and position", i+1))
		}
		if p.Mail != "" && !strings.Contains(p.Mail, "@") {
			problems = append(problems, fmt.Sprintf("styret.yaml: entry %d has an invalid mail: %q", i+1, p.Mail))
		}
	}

	if len(s.Categories) == 0 {
		problems = append(problems, "ressurser.yaml: no categories defined")
	}
	seenID := map[string]bool{}
	for _, cat := range s.Categories {
		switch {
		case cat.ID == "":
			problems = append(problems, "ressurser.yaml: a category has no id")
		case seenID[cat.ID]:
			problems = append(problems, fmt.Sprintf("ressurser.yaml: duplicate category id %q", cat.ID))
		}
		seenID[cat.ID] = true

		if cat.Heading == "" || cat.NavLabel == "" {
			problems = append(problems, fmt.Sprintf(
				"ressurser.yaml: category %q needs both heading and nav_label", cat.ID))
		}
		for _, link := range cat.Links {
			if link.Title == "" {
				problems = append(problems, fmt.Sprintf("ressurser.yaml: a link in %q has no title", cat.ID))
			}
			if err := checkURL(link.URL); err != nil {
				problems = append(problems, fmt.Sprintf(
					"ressurser.yaml: %q → %q: %v", cat.ID, link.Title, err))
			}
			for _, para := range link.Paragraphs {
				for label, href := range para.Refs {
					if !strings.Contains(para.Text, "{"+label+"}") {
						problems = append(problems, fmt.Sprintf(
							"ressurser.yaml: %q → %q: ref %q is never used in the text",
							cat.ID, link.Title, label))
					}
					if err := checkURL(href); err != nil {
						problems = append(problems, fmt.Sprintf(
							"ressurser.yaml: %q → %q: ref %q: %v", cat.ID, link.Title, label, err))
					}
				}
			}
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid content:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

func checkURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("is not a valid URL: %w", err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("must be an absolute https URL, got %q", raw)
	}
	return nil
}
