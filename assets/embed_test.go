package assets

import (
	"io/fs"
	"slices"
	"testing"
)

// pageTemplates pins the full set of page templates that must be present in
// the embedded filesystem. A page missing from the embed still builds fine
// and only fails at runtime with a 500, so additions and removals under
// assets/templates/pages must be mirrored here.
var pageTemplates = []string{
	"templates/pages/arrangementer.html",
	"templates/pages/error.html",
	"templates/pages/for-bedrifter.html",
	"templates/pages/historie.html",
	"templates/pages/index.html",
	"templates/pages/innsjekk-qr.html",
	"templates/pages/innsjekk.html",
	"templates/pages/om-itemize.html",
	"templates/pages/profil.html",
	"templates/pages/registrer.html",
	"templates/pages/registrert.html",
	"templates/pages/ressurser.html",
	"templates/pages/slett-arrangement.html",
	"templates/pages/utmelding.html",
}

func TestEmbeddedTemplatesComplete(t *testing.T) {
	fsys := FS(false)

	t.Run("pages match the pinned set", func(t *testing.T) {
		got, err := fs.Glob(fsys, "templates/pages/*.html")
		if err != nil {
			t.Fatalf("globbing embedded page templates failed: %v", err)
		}
		slices.Sort(got)

		want := slices.Clone(pageTemplates)
		slices.Sort(want)

		if !slices.Equal(got, want) {
			t.Errorf("embedded page templates do not match the pinned set: "+
				"got %v, want %v. A page dropped from the embed still builds "+
				"but 500s at runtime when the server tries to render it; if a "+
				"page was intentionally added or removed, update the pinned "+
				"list in this test.", got, want)
		}
	})

	t.Run("layout templates present", func(t *testing.T) {
		got, err := fs.Glob(fsys, "templates/layout/*.html")
		if err != nil {
			t.Fatalf("globbing embedded layout templates failed: %v", err)
		}
		if len(got) == 0 {
			t.Error("no layout templates are embedded; every page render " +
				"depends on templates/layout")
		}
	})

	t.Run("partial templates present", func(t *testing.T) {
		got, err := fs.Glob(fsys, "templates/partials/*.html")
		if err != nil {
			t.Fatalf("globbing embedded partial templates failed: %v", err)
		}
		if len(got) == 0 {
			t.Error("no partial templates are embedded; pages that include " +
				"partials would fail to parse")
		}
	})

	t.Run("base layout readable", func(t *testing.T) {
		body, err := fs.ReadFile(fsys, "templates/layout/base.html")
		if err != nil {
			t.Fatalf("reading templates/layout/base.html from the embed "+
				"failed: %v", err)
		}
		if len(body) == 0 {
			t.Error("templates/layout/base.html is embedded but empty")
		}
	})
}

func TestEmbeddedStaticPresent(t *testing.T) {
	fsys := FS(false)

	t.Run("stylesheets embedded", func(t *testing.T) {
		got, err := fs.Glob(fsys, "static/css/*.css")
		if err != nil {
			t.Fatalf("globbing embedded stylesheets failed: %v", err)
		}
		if len(got) == 0 {
			t.Error("no stylesheets are embedded under static/css; the site " +
				"would render unstyled")
		}
	})

	t.Run("scripts embedded", func(t *testing.T) {
		got, err := fs.Glob(fsys, "static/js/*.js")
		if err != nil {
			t.Fatalf("globbing embedded scripts failed: %v", err)
		}
		if len(got) == 0 {
			t.Error("no scripts are embedded under static/js; client-side " +
				"behavior would be missing")
		}
	})

	// Files referenced directly by templates or handlers: base.html links
	// icon.svg and logo-192.png, the front page and footer use logo.png, and
	// the profile page falls back to logo-512.png as the avatar.
	pinned := []struct {
		name string
		path string
	}{
		{"site icon", "static/img/icon.svg"},
		{"footer and front-page logo", "static/img/logo.png"},
		{"apple touch icon", "static/img/logo-192.png"},
		{"profile avatar fallback", "static/img/logo-512.png"},
		{"web app manifest", "static/manifest.json"},
		{"robots.txt", "static/robots.txt"},
	}
	for _, tt := range pinned {
		t.Run(tt.name, func(t *testing.T) {
			body, err := fs.ReadFile(fsys, tt.path)
			if err != nil {
				t.Fatalf("reading %s from the embed failed: %v. This file is "+
					"referenced by a template or handler, so a request for it "+
					"would 404 at runtime.", tt.path, err)
			}
			if len(body) == 0 {
				t.Errorf("%s is embedded but empty", tt.path)
			}
		})
	}
}
