package content

// Two constraints shape every test in this file:
//
//   - Load(false) memoizes through sync.Once, so the first fsys it ever sees
//     is the one every later caller gets. A synthetic input must therefore
//     never go through Load — it would poison the cache for the whole test
//     binary. All synthetic-input tests call the unexported load(fsys)
//     directly with an fstest.MapFS.
//
//   - Load(true) reads os.DirFS("content") relative to the current working
//     directory, and `go test` runs from the package directory — the path
//     resolves to content/content/, which does not exist. Never call it here.

import (
	"html/template"
	"strings"
	"testing"
	"testing/fstest"
)

// This is the only place YAML-sourced text becomes HTML — a miss here is
// stored XSS from a content file.
func TestParaHTML(t *testing.T) {
	tests := []struct {
		name string
		para Para
		want template.HTML
	}{
		{
			name: "plain text with markup is fully escaped",
			para: Para{Text: "<script>alert(1)</script>"},
			want: "&lt;script&gt;alert(1)&lt;/script&gt;",
		},
		{
			name: "placeholder with matching ref becomes an anchor",
			para: Para{Text: "{ctf}", Refs: map[string]string{"ctf": "https://x.no"}},
			want: `<a href="https://x.no" rel="noopener noreferrer">ctf</a>`,
		},
		{
			name: "hostile href is escaped inside the attribute",
			para: Para{
				Text: "{ctf}",
				Refs: map[string]string{"ctf": `https://x.no/"><script>alert(1)</script>`},
			},
			want: `<a href="https://x.no/&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt;" rel="noopener noreferrer">ctf</a>`,
		},
		{
			name: "label containing markup is escaped in the anchor text",
			para: Para{
				Text: "se {<b>x</b>} her",
				Refs: map[string]string{"<b>x</b>": "https://x.no"},
			},
			want: `se <a href="https://x.no" rel="noopener noreferrer">&lt;b&gt;x&lt;/b&gt;</a> her`,
		},
		{
			name: "placeholder without a matching ref stays literal, escaped",
			para: Para{Text: "se {<b>ukjent</b>} her"},
			want: "se {&lt;b&gt;ukjent&lt;/b&gt;} her",
		},
		{
			name: "lone opening brace is literal text",
			para: Para{Text: "a { b"},
			want: "a { b",
		},
		{
			name: "multiple placeholders in one paragraph",
			para: Para{
				Text: "{a} og {b}",
				Refs: map[string]string{"a": "https://a.no", "b": "https://b.no"},
			},
			want: `<a href="https://a.no" rel="noopener noreferrer">a</a> og <a href="https://b.no" rel="noopener noreferrer">b</a>`,
		},
		{
			name: "empty text renders as nothing",
			para: Para{},
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.para.HTML(); got != tc.want {
				t.Errorf("paragraph rendered wrong — any unescaped markup here is stored XSS from a content file:\n  got:  %s\n  want: %s", got, tc.want)
			}
		})
	}
}

func TestCheckURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"empty is required", "", true},
		{"absolute https is fine", "https://x.no", false},
		{"http is refused", "http://x.no", true},
		{"https with no host is refused", "https://", true},
		{"non-web scheme is refused", "ftp://x", true},
		{"unparseable URL is refused", "https://exa mple.no", true},
		{"relative path is refused", "foo/bar", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkURL(tc.raw)
			if tc.wantErr && err == nil {
				t.Errorf("%q was accepted — a bad URL in a content file should fail the deploy, not render a broken link", tc.raw)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("%q was refused: %v", tc.raw, err)
			}
		})
	}
}

// validFiles returns the smallest four-file content set that load accepts.
// Each test below copies it fresh and breaks exactly one thing.
func validFiles() fstest.MapFS {
	return fstest.MapFS{
		"kontakt.yaml": &fstest.MapFile{Data: []byte(`email: a@x.no
invoice_email: b@x.no
company_email: c@x.no
membership_email: d@x.no
org_number: "1"
discord_invite: https://discord.com/invite/x
bylaws_url: https://cloud.x.no/f/1
address_short: [Itemize NTNU]
address_full: [Itemize NTNU]
`)},
		"socials.yaml": &fstest.MapFile{Data: []byte(`socials:
  - icon: discord
    label: Itemize NTNU
    url: https://discord.com/invite/x
`)},
		"styret.yaml": &fstest.MapFile{Data: []byte(`board:
  - name: Kari Nordmann
    position: Leder
    mail: kari@x.no
`)},
		"ressurser.yaml": &fstest.MapFile{Data: []byte(`categories:
  - id: ctfer
    heading: "CTFer:"
    nav_label: CTFer
    links:
      - title: PicoCTF
        url: https://x.no
        paragraphs:
          - text: "Se {ctf} her."
            refs:
              ctf: https://x.no
`)},
	}
}

func TestLoadValidatesEachRule(t *testing.T) {
	if _, err := load(validFiles()); err != nil {
		t.Fatalf("the baseline fixture must load cleanly before mutations mean anything: %v", err)
	}

	tests := []struct {
		name string
		// mutate breaks one rule in a fresh copy of the valid set. A nil
		// wantSubstring case (empty string with wantOK) is a positive check.
		mutate func(fstest.MapFS)
		// wantSubstring must appear in the error. Empty means the mutation
		// is legal and load must still succeed.
		wantSubstring string
	}{
		{
			name: "unknown YAML key is rejected, not silently dropped",
			mutate: func(m fstest.MapFS) {
				m["kontakt.yaml"] = &fstest.MapFile{Data: append(
					m["kontakt.yaml"].Data, []byte("bogus_key: 1\n")...)}
			},
			wantSubstring: "parsing content/kontakt.yaml",
		},
		{
			name: "malformed YAML is a parse error naming the file",
			mutate: func(m fstest.MapFS) {
				m["styret.yaml"] = &fstest.MapFile{Data: []byte("board: [unclosed\n")}
			},
			wantSubstring: "parsing content/styret.yaml",
		},
		{
			name: "missing file is a read error naming the file",
			mutate: func(m fstest.MapFS) {
				delete(m, "styret.yaml")
			},
			wantSubstring: "reading content/styret.yaml",
		},
		{
			name: "contact email without @ is refused",
			mutate: func(m fstest.MapFS) {
				m["kontakt.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["kontakt.yaml"].Data), "email: a@x.no", "email: not-an-address"))}
			},
			wantSubstring: "is not an email address",
		},
		{
			name: "empty address list is refused",
			mutate: func(m fstest.MapFS) {
				m["kontakt.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["kontakt.yaml"].Data), "address_short: [Itemize NTNU]", "address_short: []"))}
			},
			wantSubstring: "both address_short and address_full are required",
		},
		{
			name: "non-https discord invite is refused",
			mutate: func(m fstest.MapFS) {
				m["kontakt.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["kontakt.yaml"].Data),
					"discord_invite: https://discord.com/invite/x",
					"discord_invite: http://discord.com/invite/x"))}
			},
			wantSubstring: "discord_invite",
		},
		{
			name: "missing bylaws URL is refused",
			mutate: func(m fstest.MapFS) {
				m["kontakt.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["kontakt.yaml"].Data),
					"bylaws_url: https://cloud.x.no/f/1", `bylaws_url: ""`))}
			},
			wantSubstring: "bylaws_url",
		},
		{
			name: "zero socials is refused",
			mutate: func(m fstest.MapFS) {
				m["socials.yaml"] = &fstest.MapFile{Data: []byte("socials: []\n")}
			},
			wantSubstring: "no socials defined",
		},
		{
			name: "unknown icon is refused so a typo cannot ship an invisible link",
			mutate: func(m fstest.MapFS) {
				m["socials.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["socials.yaml"].Data), "icon: discord", "icon: myspace"))}
			},
			wantSubstring: "unknown icon",
		},
		{
			name: "social without a label is refused",
			mutate: func(m fstest.MapFS) {
				m["socials.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["socials.yaml"].Data), "label: Itemize NTNU", `label: ""`))}
			},
			wantSubstring: "has no label",
		},
		{
			name: "social with a bad URL is refused",
			mutate: func(m fstest.MapFS) {
				m["socials.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["socials.yaml"].Data),
					"url: https://discord.com/invite/x", "url: discord.com/invite/x"))}
			},
			wantSubstring: "socials.yaml: entry 1: must be an absolute https URL",
		},
		{
			name: "empty board is refused",
			mutate: func(m fstest.MapFS) {
				m["styret.yaml"] = &fstest.MapFile{Data: []byte("board: []\n")}
			},
			wantSubstring: "no board members defined",
		},
		{
			name: "board entry without a name is refused",
			mutate: func(m fstest.MapFS) {
				m["styret.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["styret.yaml"].Data), "  - name: Kari Nordmann", `  - name: ""`))}
			},
			wantSubstring: "needs both name and position",
		},
		{
			name: "board entry without a position is refused",
			mutate: func(m fstest.MapFS) {
				m["styret.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["styret.yaml"].Data), "    position: Leder", `    position: ""`))}
			},
			wantSubstring: "needs both name and position",
		},
		{
			name: "board mail without @ is refused",
			mutate: func(m fstest.MapFS) {
				m["styret.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["styret.yaml"].Data), "    mail: kari@x.no", "    mail: kari.x.no"))}
			},
			wantSubstring: "has an invalid mail",
		},
		{
			// The board page shows members without a public address; an empty
			// mail is a choice, not a mistake.
			name: "board entry with an empty mail is allowed",
			mutate: func(m fstest.MapFS) {
				m["styret.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["styret.yaml"].Data), "    mail: kari@x.no", `    mail: ""`))}
			},
			wantSubstring: "",
		},
		{
			name: "zero categories is refused",
			mutate: func(m fstest.MapFS) {
				m["ressurser.yaml"] = &fstest.MapFile{Data: []byte("categories: []\n")}
			},
			wantSubstring: "no categories defined",
		},
		{
			name: "category without an id is refused",
			mutate: func(m fstest.MapFS) {
				m["ressurser.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["ressurser.yaml"].Data), "  - id: ctfer", `  - id: ""`))}
			},
			wantSubstring: "a category has no id",
		},
		{
			name: "duplicate category ids are refused",
			mutate: func(m fstest.MapFS) {
				m["ressurser.yaml"] = &fstest.MapFile{Data: []byte(`categories:
  - id: ctfer
    heading: "CTFer:"
    nav_label: CTFer
    links: []
  - id: ctfer
    heading: Andre
    nav_label: Andre
    links: []
`)}
			},
			wantSubstring: `duplicate category id "ctfer"`,
		},
		{
			name: "category without a heading is refused",
			mutate: func(m fstest.MapFS) {
				m["ressurser.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["ressurser.yaml"].Data), `    heading: "CTFer:"`, `    heading: ""`))}
			},
			wantSubstring: "needs both heading and nav_label",
		},
		{
			name: "category without a nav_label is refused",
			mutate: func(m fstest.MapFS) {
				m["ressurser.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["ressurser.yaml"].Data), "    nav_label: CTFer", `    nav_label: ""`))}
			},
			wantSubstring: "needs both heading and nav_label",
		},
		{
			name: "link without a title is refused",
			mutate: func(m fstest.MapFS) {
				m["ressurser.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["ressurser.yaml"].Data), "      - title: PicoCTF", `      - title: ""`))}
			},
			wantSubstring: "has no title",
		},
		{
			name: "link with a bad URL is refused",
			mutate: func(m fstest.MapFS) {
				m["ressurser.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["ressurser.yaml"].Data), "        url: https://x.no", "        url: x.no"))}
			},
			wantSubstring: `"ctfer" → "PicoCTF"`,
		},
		{
			name: "ref that never appears as a placeholder is refused",
			mutate: func(m fstest.MapFS) {
				m["ressurser.yaml"] = &fstest.MapFile{Data: []byte(replaceLine(
					string(m["ressurser.yaml"].Data), "              ctf: https://x.no",
					"              ubrukt: https://x.no"))}
			},
			wantSubstring: "is never used in the text",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fsys := validFiles()
			tc.mutate(fsys)
			_, err := load(fsys)
			if tc.wantSubstring == "" {
				if err != nil {
					t.Fatalf("a legal content variation was refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("a broken content file loaded without error — the whole point of validation is failing the deploy instead of shipping the breakage")
			}
			if !strings.Contains(err.Error(), tc.wantSubstring) {
				t.Errorf("the error does not name the actual problem, so whoever edits the YAML cannot find it:\n  got:  %v\n  want substring: %s", err, tc.wantSubstring)
			}
		})
	}
}

// replaceLine swaps one exact substring in a fixture and refuses to miss:
// a silent no-op replacement would turn a validation test into a test of
// the untouched baseline.
func replaceLine(doc, old, new string) string {
	if !strings.Contains(doc, old) {
		panic("fixture drift: " + old + " not found in test YAML")
	}
	return strings.Replace(doc, old, new, 1)
}

// A bad edit to the shipped YAML fails CI here instead of killing the deploy.
func TestEmbeddedContentIsValid(t *testing.T) {
	site, err := load(embedded)
	if err != nil {
		t.Fatalf("the embedded content files do not validate — this exact error would otherwise appear at deploy time: %v", err)
	}
	if len(site.Board) == 0 {
		t.Error("the shipped board is empty")
	}
	if len(site.Categories) == 0 {
		t.Error("the shipped resource directory is empty")
	}
	if len(site.Socials) == 0 {
		t.Error("the shipped social links are empty")
	}
}
