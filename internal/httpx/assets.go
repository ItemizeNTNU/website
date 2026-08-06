package httpx

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

// Assets serves the site's static files from an embedded filesystem.
//
// CSS and JS are authored as several ordered files and concatenated here at
// startup, then fingerprinted so they can be cached forever: this gives the
// benefits of a build step without there being one. Everything is compressed
// once up front rather than per request — the largest asset on the site is the
// favicon, and gzipping it on every hit would be pure waste.
type Assets struct {
	mu    sync.RWMutex
	files map[string]*asset // request path -> asset
	names map[string]string // logical name ("app.css") -> fingerprinted path

	fsys fs.FS
	dev  bool
}

type asset struct {
	contentType string
	body        []byte
	gzipped     []byte // nil when compression did not help
	etag        string
	immutable   bool
}

// rootFiles are served from / rather than /assets/ because their URLs are
// referenced from outside our own HTML — by the web app manifest, by browsers
// looking for a favicon, and by service workers already registered in
// returning visitors' browsers.
var rootFiles = map[string]string{
	"/icon.svg":      "static/img/icon.svg",
	"/logo.png":      "static/img/logo.png",
	"/logo-192.png":  "static/img/logo-192.png",
	"/logo-512.png":  "static/img/logo-512.png",
	"/manifest.json": "static/manifest.json",
	"/robots.txt":    "static/robots.txt",
	// Returning visitors have the old Sapper service worker registered and it
	// will keep serving them a cached copy of the previous site. The
	// replacement at this path unregisters it. Do not remove.
	"/service-worker.js": "static/service-worker.js",
}

// NewAssets builds the asset table from fsys. In dev mode the table is rebuilt
// on every request so edits to CSS and JS appear on reload.
func NewAssets(fsys fs.FS, dev bool) (*Assets, error) {
	a := &Assets{fsys: fsys, dev: dev}
	if err := a.build(); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *Assets) build() error {
	files := map[string]*asset{}
	names := map[string]string{}

	// Bundled stylesheets and scripts. Lexical order is load order, which is
	// why the source files are numbered.
	//
	// boot.js is separate because it must run before the first paint — it sets
	// the flag the stylesheet uses to decide whether the navigation collapses
	// into a hamburger, and a deferred script would let the expanded menu
	// paint and then snap shut.
	for _, b := range []struct{ name, glob, contentType string }{
		{"app.css", "static/css/*.css", "text/css; charset=utf-8"},
		{"app.js", "static/js/*.js", "text/javascript; charset=utf-8"},
		{"boot.js", "static/boot.js", "text/javascript; charset=utf-8"},
	} {
		body, err := concat(a.fsys, b.glob)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			continue
		}
		stem, ext, _ := strings.Cut(b.name, ".")
		digest := fingerprint(body)
		p := fmt.Sprintf("/assets/%s.%s.%s", stem, digest, ext)

		files[p] = newAsset(b.contentType, body, true)
		names[b.name] = p
	}

	// Fonts and images keep stable paths. Their content is tied to their
	// filename in practice — a different font is a different file — so they
	// can still be cached immutably.
	for _, dir := range []string{"static/fonts", "static/img"} {
		entries, err := fs.ReadDir(a.fsys, dir)
		if err != nil {
			continue // an empty or absent directory is not a failure
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := path.Join(dir, e.Name())
			body, err := fs.ReadFile(a.fsys, name)
			if err != nil {
				return fmt.Errorf("reading %s: %w", name, err)
			}
			p := "/assets/" + strings.TrimPrefix(name, "static/")
			files[p] = newAsset(contentTypeFor(e.Name()), body, true)
			names[strings.TrimPrefix(name, "static/")] = p
		}
	}

	for urlPath, name := range rootFiles {
		body, err := fs.ReadFile(a.fsys, name)
		if err != nil {
			continue // optional
		}
		files[urlPath] = newAsset(contentTypeFor(name), body, false)
		names[path.Base(urlPath)] = urlPath
	}

	a.mu.Lock()
	a.files, a.names = files, names
	a.mu.Unlock()
	return nil
}

func newAsset(contentType string, body []byte, immutable bool) *asset {
	sum := sha256.Sum256(body)
	res := &asset{
		contentType: contentType,
		body:        body,
		etag:        `"` + hex.EncodeToString(sum[:])[:16] + `"`,
		immutable:   immutable,
	}
	if compressible(contentType) {
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if _, err := zw.Write(body); err == nil && zw.Close() == nil {
			// Only keep the compressed copy if it is actually smaller.
			if buf.Len() < len(body) {
				res.gzipped = buf.Bytes()
			}
		}
	}
	return res
}

// URL returns the public path for a logical asset name, e.g. "app.css" ->
// "/assets/app.1a2b3c4d5e.css". An unknown name yields "/" + name so a typo
// shows up as a 404 rather than a silently broken page.
func (a *Assets) URL(name string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if p, ok := a.names[name]; ok {
		return p
	}
	return "/" + name
}

// Has reports whether an asset exists. Templates use it to avoid emitting a
// preload hint for a font that has not been vendored yet, which would cost a
// request and a console warning for nothing.
func (a *Assets) Has(name string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.names[name]
	return ok
}

// Refresh rebuilds the asset table in dev mode; in production it does nothing.
//
// This has to happen before a page is rendered, not only when an asset is
// requested. The fingerprint is baked into the HTML, so rebuilding on the
// asset request alone would mean every page after an edit points at the
// previous hash — which then 404s, and the page loads unstyled.
func (a *Assets) Refresh() error {
	if !a.dev {
		return nil
	}
	return a.build()
}

// ServeHTTP serves an asset by request path.
func (a *Assets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if a.dev {
		if err := a.build(); err != nil {
			http.Error(w, "asset build failed", http.StatusInternalServerError)
			return
		}
	}

	a.mu.RLock()
	f, ok := a.files[r.URL.Path]
	a.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}

	h := w.Header()
	h.Set("Content-Type", f.contentType)
	h.Set("ETag", f.etag)
	switch {
	case a.dev:
		h.Set("Cache-Control", "no-cache")
	case f.immutable:
		// Safe because the path changes whenever the content does.
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	default:
		h.Set("Cache-Control", "public, max-age=3600")
	}

	if match := r.Header.Get("If-None-Match"); match != "" && etagMatches(match, f.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	body := f.body
	if f.gzipped != nil && acceptsGzip(r) {
		body = f.gzipped
		h.Set("Content-Encoding", "gzip")
		h.Add("Vary", "Accept-Encoding")
	}
	h.Set("Content-Length", fmt.Sprint(len(body)))

	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	// A zero modtime keeps ServeContent from adding Last-Modified; the ETag
	// is the validator, and Range support comes along for free.
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
}

// Register wires the asset routes into mux.
func (a *Assets) Register(mux *http.ServeMux) {
	mux.Handle("GET /assets/", a)
	a.mu.RLock()
	defer a.mu.RUnlock()
	for urlPath := range rootFiles {
		if _, ok := a.files[urlPath]; ok {
			mux.Handle("GET "+urlPath, a)
		}
	}
}

// etagMatches implements the If-None-Match comparison, tolerating the weak
// prefix that the gzip middleware adds and the "*" wildcard.
func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			return true
		}
		if strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(etag, "W/") {
			return true
		}
	}
	return false
}

// concat joins every file matching glob, in lexical order, separated by a
// comment naming the source so the bundled output stays navigable.
func concat(fsys fs.FS, glob string) ([]byte, error) {
	matches, err := fs.Glob(fsys, glob)
	if err != nil {
		return nil, fmt.Errorf("globbing %s: %w", glob, err)
	}
	sort.Strings(matches)

	var buf bytes.Buffer
	for _, name := range matches {
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		fmt.Fprintf(&buf, "/* %s */\n", path.Base(name))
		buf.Write(body)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

func fingerprint(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])[:10]
}

func contentTypeFor(name string) string {
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		return ct
	}
	switch path.Ext(name) {
	case ".woff2":
		return "font/woff2"
	case ".webmanifest":
		return "application/manifest+json"
	}
	return "application/octet-stream"
}
