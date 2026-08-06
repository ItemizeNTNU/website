package web

// Package-internal tests for the Renderer: construction errors, the
// buffer-first property that keeps a half-rendered page off the wire, and the
// dev-mode per-request re-parse.

import (
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// discardLog is the logger every renderer test uses; the tests assert on the
// HTTP response, not the log.
func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// tinyTemplates is the smallest tree NewRenderer's globs accept: one layout,
// one partial (the partials glob must match at least one file or ParseFS
// errors), one working page and one page that fails at execution time.
func tinyTemplates() fstest.MapFS {
	return fstest.MapFS{
		"templates/layout/base.html": &fstest.MapFile{
			Data: []byte(`<!doctype html>{{template "content" .}}`),
		},
		"templates/partials/noop.html": &fstest.MapFile{
			Data: []byte(`{{define "noop"}}{{end}}`),
		},
		"templates/pages/home.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}Hei {{.Name}}{{end}}`),
		},
		"templates/pages/boom.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}{{index .Items 5}}{{end}}`),
		},
	}
}

type renderData struct {
	Name  string
	Items []string
}

func newTestRenderer(t *testing.T, fsys fstest.MapFS, dev bool) *Renderer {
	t.Helper()
	r, err := NewRenderer(fsys, template.FuncMap{}, dev, discardLog())
	if err != nil {
		t.Fatalf("NewRenderer failed on a valid template tree: %v", err)
	}
	return r
}

func TestNewRendererErrors(t *testing.T) {
	t.Run("no pages", func(t *testing.T) {
		fsys := tinyTemplates()
		delete(fsys, "templates/pages/home.html")
		delete(fsys, "templates/pages/boom.html")

		_, err := NewRenderer(fsys, template.FuncMap{}, false, discardLog())
		if err == nil || !strings.Contains(err.Error(), "no page templates found") {
			t.Errorf("NewRenderer with no pages returned %v, want an error containing \"no page templates found\" — a misassembled embed would otherwise start a server with nothing to serve", err)
		}
	})

	t.Run("parse error names the file", func(t *testing.T) {
		fsys := tinyTemplates()
		fsys["templates/pages/broken.html"] = &fstest.MapFile{
			Data: []byte(`{{define "content"}}{{if}}{{end}}`),
		}

		_, err := NewRenderer(fsys, template.FuncMap{}, false, discardLog())
		if err == nil {
			t.Fatal("NewRenderer accepted a page with a syntax error — the failure would surface later, on a visitor's request, instead of at startup")
		}
		if !strings.Contains(err.Error(), "templates/pages/broken.html") {
			t.Errorf("parse error %q does not name the broken file — whoever reads the startup log would have to bisect the templates by hand", err)
		}
	})
}

func TestRenderUnknownPage(t *testing.T) {
	r := newTestRenderer(t, tinyTemplates(), false)
	rec := httptest.NewRecorder()
	r.Render(rec, httptest.NewRequest("GET", "/", nil), http.StatusOK, "no-such-page", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("rendering an unknown page returned %d, want 500 — a handler typo would otherwise pass silently", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Noe gikk galt") {
		t.Errorf("body %q lacks the generic error text — the visitor would see something rawer than the Norwegian apology", rec.Body.String())
	}
}

// TestRenderBuffersBeforeWriting is the property the Renderer exists for: a
// template that fails halfway through execution must produce a clean 500, not
// a 200 whose body stops mid-sentence.
func TestRenderBuffersBeforeWriting(t *testing.T) {
	r := newTestRenderer(t, tinyTemplates(), false)
	rec := httptest.NewRecorder()
	// boom.html indexes past the end of an empty slice, so the layout's
	// doctype has already executed into the buffer by the time it fails.
	r.Render(rec, httptest.NewRequest("GET", "/", nil), http.StatusOK, "boom", renderData{})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("a mid-template failure returned status %d, want 500 — the browser would treat a truncated page as a success", rec.Code)
	}
	if strings.HasPrefix(rec.Body.String(), "<!doctype") {
		t.Errorf("body %q begins with the successfully-rendered prefix — the half-written page reached the wire, which is exactly what rendering into a buffer first is meant to prevent", rec.Body.String())
	}
}

func TestRenderDevReparsesPerRequest(t *testing.T) {
	fsys := tinyTemplates()
	r := newTestRenderer(t, fsys, true)
	req := httptest.NewRequest("GET", "/", nil)

	good := fsys["templates/pages/home.html"].Data

	// Break the file on disk after construction. In dev the very next request
	// must see the breakage.
	fsys["templates/pages/home.html"].Data = []byte(`{{define "content"}}{{if}}{{end}}`)
	rec := httptest.NewRecorder()
	r.Render(rec, req, http.StatusOK, "home", renderData{Name: "Kari"})
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("dev render of a freshly broken template returned %d, want 500 — the developer would keep seeing the stale working copy and not know the file is broken", rec.Code)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "Templatefeil: ") {
		t.Errorf("dev error body %q lacks the \"Templatefeil: \" prefix that tells the developer this is a template problem", body)
	}
	if !strings.Contains(body, "templates/pages/home.html") {
		t.Errorf("dev error body %q does not carry the parse error naming the file — the developer would have to guess which template they just broke", body)
	}

	// Fix it back: the next request must render again, proving the re-parse
	// happens per request rather than once.
	fsys["templates/pages/home.html"].Data = good
	rec = httptest.NewRecorder()
	r.Render(rec, req, http.StatusOK, "home", renderData{Name: "Kari"})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Hei Kari") {
		t.Errorf("after fixing the template, dev render returned %d with body %q — an edit-refresh cycle would need a server restart, defeating dev mode", rec.Code, rec.Body.String())
	}
}

func TestRenderProdIgnoresDiskChanges(t *testing.T) {
	fsys := tinyTemplates()
	r := newTestRenderer(t, fsys, false)

	// In production the templates were parsed once at construction; mutating
	// the source afterwards must change nothing.
	fsys["templates/pages/home.html"].Data = []byte(`{{define "content"}}{{if}}{{end}}`)
	rec := httptest.NewRecorder()
	r.Render(rec, httptest.NewRequest("GET", "/", nil), http.StatusOK, "home", renderData{Name: "Kari"})

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Hei Kari") {
		t.Errorf("prod render after a disk mutation returned %d with body %q — production must serve the templates it started with, not re-read the filesystem on every request", rec.Code, rec.Body.String())
	}
}

func TestRenderHead(t *testing.T) {
	r := newTestRenderer(t, tinyTemplates(), false)
	rec := httptest.NewRecorder()
	r.Render(rec, httptest.NewRequest("HEAD", "/", nil), http.StatusOK, "home", renderData{Name: "Kari"})

	if rec.Code != http.StatusOK {
		t.Errorf("HEAD returned status %d, want 200 — a HEAD probe would report the page as broken", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("HEAD Content-Type = %q, want \"text/html; charset=utf-8\" — HEAD must carry the same headers a GET would", got)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("HEAD response carried a %d-byte body — HEAD promises headers only, and a body here means the server misreads the method", rec.Body.Len())
	}
}

func TestRenderStatusPassthrough(t *testing.T) {
	r := newTestRenderer(t, tinyTemplates(), false)
	rec := httptest.NewRecorder()
	r.Render(rec, httptest.NewRequest("GET", "/", nil), http.StatusUnprocessableEntity, "home", renderData{Name: "Kari"})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("Render wrote status %d, want 422 — a rejected form would come back as a success, and the browser and any tooling would treat it as one", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want \"text/html; charset=utf-8\" — without the charset the browser guesses the encoding of Norwegian text", got)
	}
}
