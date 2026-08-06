// Package web renders the site's HTML.
package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"
)

// Renderer holds the parsed templates.
//
// Each page gets its own fully resolved template set rather than sharing one:
// every page file defines "content", so a single set would keep only whichever
// happened to be parsed last.
type Renderer struct {
	mu    sync.RWMutex
	pages map[string]*template.Template

	fsys  fs.FS
	funcs template.FuncMap
	dev   bool
	log   *slog.Logger
}

// NewRenderer parses every page under templates/pages, each combined with the
// layout and all partials. In dev mode templates are re-parsed per request.
func NewRenderer(fsys fs.FS, funcs template.FuncMap, dev bool, log *slog.Logger) (*Renderer, error) {
	r := &Renderer{fsys: fsys, funcs: funcs, dev: dev, log: log}
	if err := r.parse(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Renderer) parse() error {
	names, err := fs.Glob(r.fsys, "templates/pages/*.html")
	if err != nil {
		return fmt.Errorf("globbing pages: %w", err)
	}
	if len(names) == 0 {
		return fmt.Errorf("no page templates found under templates/pages")
	}

	pages := make(map[string]*template.Template, len(names))
	for _, page := range names {
		t, err := template.New("base.html").
			Funcs(r.funcs).
			ParseFS(r.fsys,
				"templates/layout/*.html",
				"templates/partials/*.html",
				page,
			)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", page, err)
		}
		pages[strings.TrimSuffix(path.Base(page), ".html")] = t
	}

	r.mu.Lock()
	r.pages = pages
	r.mu.Unlock()
	return nil
}

// Render writes a page. The template is executed into a buffer first: a
// template error halfway through would otherwise leave a truncated page
// already committed to a 200 response.
func (r *Renderer) Render(w http.ResponseWriter, req *http.Request, status int, page string, data any) {
	if r.dev {
		if err := r.parse(); err != nil {
			r.log.Error("template parse failed", "err", err)
			http.Error(w, "Templatefeil: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	r.mu.RLock()
	t, ok := r.pages[page]
	r.mu.RUnlock()
	if !ok {
		r.log.Error("unknown page template", "page", page)
		http.Error(w, "Noe gikk galt", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	// Always by name: Execute would run whichever template the set happens to
	// be associated with, which is not necessarily the layout.
	if err := t.ExecuteTemplate(&buf, "base.html", data); err != nil {
		r.log.Error("template execution failed", "page", page, "err", err)
		http.Error(w, "Noe gikk galt", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if req.Method != http.MethodHead {
		_, _ = buf.WriteTo(w)
	}
}
