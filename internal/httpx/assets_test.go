package httpx

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

// secretBody lives in the embedded filesystem but under no path the asset
// server is supposed to expose. Every traversal test asserts these bytes never
// reach a client — if they do, the same trick reaches templates and any other
// non-public file that shares the filesystem.
const secretBody = "SECRET-NOT-AN-ASSET"

// layoutCSS is padded with repetitive rules on purpose. The pre-compression
// step only keeps a gzipped copy when it comes out smaller than the original,
// and gzip framing costs more than it saves on a forty-byte stylesheet — so a
// minimal fixture would silently test the uncompressed path everywhere.
var layoutCSS = "body{display:grid}\n" + strings.Repeat(".col-span-2{grid-column:span 2}\n", 40)

// testAssetFS mirrors the real layout in assets/static: numbered CSS and JS
// that get bundled, a standalone boot script, fonts and images served under
// stable paths, and the handful of files that must live at the site root.
func testAssetFS() fstest.MapFS {
	return fstest.MapFS{
		"static/css/01-reset.css":    &fstest.MapFile{Data: []byte("html{margin:0}\n")},
		"static/css/02-layout.css":   &fstest.MapFile{Data: []byte(layoutCSS)},
		"static/js/01-nav.js":        &fstest.MapFile{Data: []byte("const nav = 1;\n")},
		"static/js/02-forms.js":      &fstest.MapFile{Data: []byte("const forms = 2;\n")},
		"static/boot.js":             &fstest.MapFile{Data: []byte("document.documentElement.dataset.js = '1';\n")},
		"static/fonts/inter.woff2":   &fstest.MapFile{Data: []byte("wOF2 pretend font payload")},
		"static/img/icon.svg":        &fstest.MapFile{Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)},
		"static/img/logo.png":        &fstest.MapFile{Data: []byte("\x89PNG\r\n\x1a\n pretend image payload")},
		"static/img/nested/deep.png": &fstest.MapFile{Data: []byte("\x89PNG nested")},
		"static/manifest.json":       &fstest.MapFile{Data: []byte(`{"name":"Itemize"}`)},
		"static/robots.txt":          &fstest.MapFile{Data: []byte("User-agent: *\nAllow: /\n")},
		"static/service-worker.js":   &fstest.MapFile{Data: []byte("self.registration.unregister();\n")},
		// Neither of these is an asset. They share the filesystem with the
		// ones that are, which is exactly what the traversal tests are about.
		"static/secret.txt":          &fstest.MapFile{Data: []byte(secretBody)},
		"templates/pages/index.html": &fstest.MapFile{Data: []byte(secretBody)},
	}
}

func newTestAssets(t *testing.T, fsys fs.FS, dev bool) *Assets {
	t.Helper()
	a, err := NewAssets(fsys, dev)
	if err != nil {
		t.Fatalf("building the asset table failed, which takes the whole site down at startup: %v", err)
	}
	return a
}

// serveAsset issues a request straight at the asset handler, bypassing
// ServeMux — the handler must be safe on its own, since it is also reachable
// through the "GET /assets/" subtree pattern that hands it arbitrary suffixes.
func serveAsset(t *testing.T, a *Assets, target string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := wireRequest(t, target)
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)
	return rec
}

var fingerprintedCSS = regexp.MustCompile(`^/assets/app\.[0-9a-f]{10}\.css$`)

// The CSS and JS are authored as several numbered files and concatenated at
// startup. Lexical order is load order: get it backwards and a later
// stylesheet's overrides are applied before the base it overrides.
func TestAssetsBundlesInLexicalOrder(t *testing.T) {
	a := newTestAssets(t, testAssetFS(), false)

	body := serveAsset(t, a, a.URL("app.css"), nil).Body.String()

	reset := strings.Index(body, "01-reset.css")
	layout := strings.Index(body, "02-layout.css")
	switch {
	case reset < 0 || layout < 0:
		t.Fatalf("bundle is missing a source marker; got %q", body)
	case reset > layout:
		t.Error("02-layout.css was bundled before 01-reset.css, so base rules would override the layout")
	}
	for _, want := range []string{"html{margin:0}", "body{display:grid}"} {
		if !strings.Contains(body, want) {
			t.Errorf("bundle is missing %q; that stylesheet never reaches the browser", want)
		}
	}

	js := serveAsset(t, a, a.URL("app.js"), nil).Body.String()
	if nav, forms := strings.Index(js, "const nav"), strings.Index(js, "const forms"); nav < 0 || forms < 0 || nav > forms {
		t.Errorf("app.js bundle is out of order or incomplete: %q", js)
	}

	// boot.js must stay its own file: it runs before first paint, and folding
	// it into the deferred bundle would let the expanded menu paint and snap.
	boot := serveAsset(t, a, a.URL("boot.js"), nil).Body.String()
	if strings.Contains(boot, "const nav") {
		t.Error("boot.js was bundled together with app.js; it has to load separately to run before first paint")
	}
}

// Templates bake the fingerprinted path into the HTML, so URL is the only way
// a page can reference an asset. A name that resolves to something other than
// the real path renders the site unstyled.
func TestAssetsURLAndHas(t *testing.T) {
	a := newTestAssets(t, testAssetFS(), false)

	tests := []struct {
		name    string
		want    string
		pattern *regexp.Regexp
		wantHas bool
	}{
		{name: "app.css", pattern: fingerprintedCSS, wantHas: true},
		{name: "app.js", pattern: regexp.MustCompile(`^/assets/app\.[0-9a-f]{10}\.js$`), wantHas: true},
		{name: "boot.js", pattern: regexp.MustCompile(`^/assets/boot\.[0-9a-f]{10}\.js$`), wantHas: true},
		{name: "img/logo.png", want: "/assets/img/logo.png", wantHas: true},
		{name: "img/icon.svg", want: "/assets/img/icon.svg", wantHas: true},
		{name: "fonts/inter.woff2", want: "/assets/fonts/inter.woff2", wantHas: true},
		{name: "robots.txt", want: "/robots.txt", wantHas: true},
		{name: "manifest.json", want: "/manifest.json", wantHas: true},
		{name: "service-worker.js", want: "/service-worker.js", wantHas: true},
		// An unknown name deliberately yields a path that 404s: a typo in a
		// template should be loud, not silently resolve to something else.
		{name: "fonts/never-vendored.woff2", want: "/fonts/never-vendored.woff2"},
		{name: "app.scss", want: "/app.scss"},
		{name: "", want: "/"},
	}

	for _, tt := range tests {
		t.Run(strconv.Quote(tt.name), func(t *testing.T) {
			got := a.URL(tt.name)
			switch {
			case tt.pattern != nil && !tt.pattern.MatchString(got):
				t.Errorf("URL(%q) = %q, want a fingerprinted path matching %s", tt.name, got, tt.pattern)
			case tt.pattern == nil && got != tt.want:
				t.Errorf("URL(%q) = %q, want %q", tt.name, got, tt.want)
			}
			if has := a.Has(tt.name); has != tt.wantHas {
				t.Errorf("Has(%q) = %v, want %v; templates use this to decide whether to emit a preload hint",
					tt.name, has, tt.wantHas)
			}
			// Whatever URL says, requesting it must agree with Has.
			code := serveAsset(t, a, got, nil).Code
			if tt.wantHas && code != http.StatusOK {
				t.Errorf("URL(%q) points at %q which returns %d", tt.name, got, code)
			}
			if !tt.wantHas && code != http.StatusNotFound {
				t.Errorf("a name no asset has resolved to %q which returns %d, want a visible 404", got, code)
			}
		})
	}
}

// The whole point of the fingerprint is that the path changes whenever the
// bytes do. If it did not, a year-long immutable cache would pin visitors to
// the stylesheet that shipped on their first visit.
func TestAssetsFingerprintTracksContent(t *testing.T) {
	base := testAssetFS()
	first := newTestAssets(t, base, false).URL("app.css")

	same := newTestAssets(t, testAssetFS(), false).URL("app.css")
	if same != first {
		t.Errorf("identical content produced two different URLs (%q and %q); every deploy would bust the cache for nothing", first, same)
	}

	changed := testAssetFS()
	changed["static/css/02-layout.css"] = &fstest.MapFile{Data: []byte("body{display:flex}\n")}
	if got := newTestAssets(t, changed, false).URL("app.css"); got == first {
		t.Errorf("edited CSS kept the URL %q, so cached browsers would never see the change", got)
	}

	// A new file appearing in the bundle must move the fingerprint too.
	added := testAssetFS()
	added["static/css/03-print.css"] = &fstest.MapFile{Data: []byte("@media print{a{color:#000}}\n")}
	if got := newTestAssets(t, added, false).URL("app.css"); got == first {
		t.Errorf("a stylesheet was added and the URL stayed %q", got)
	}
}

// Fingerprinted bundles can be cached forever because their path encodes their
// content. The root files cannot: /robots.txt keeps its path across deploys,
// so an immutable header there would freeze it in every visitor's cache.
func TestAssetsCacheControl(t *testing.T) {
	prod := newTestAssets(t, testAssetFS(), false)
	dev := newTestAssets(t, testAssetFS(), true)

	tests := []struct {
		path     string
		wantProd string
	}{
		{path: prod.URL("app.css"), wantProd: "public, max-age=31536000, immutable"},
		{path: prod.URL("app.js"), wantProd: "public, max-age=31536000, immutable"},
		{path: "/assets/img/logo.png", wantProd: "public, max-age=31536000, immutable"},
		{path: "/assets/fonts/inter.woff2", wantProd: "public, max-age=31536000, immutable"},
		{path: "/robots.txt", wantProd: "public, max-age=3600"},
		{path: "/manifest.json", wantProd: "public, max-age=3600"},
		{path: "/service-worker.js", wantProd: "public, max-age=3600"},
		{path: "/icon.svg", wantProd: "public, max-age=3600"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := serveAsset(t, prod, tt.path, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200", rec.Code)
			}
			if got := rec.Header().Get("Cache-Control"); got != tt.wantProd {
				t.Errorf("Cache-Control = %q, want %q", got, tt.wantProd)
			}
			if rec.Header().Get("ETag") == "" {
				t.Error("no ETag, so a revalidating client has to re-download the whole file")
			}

			// In dev nothing may be cached, or an edit never shows up on reload.
			if got := serveAsset(t, dev, tt.path, nil).Header().Get("Cache-Control"); got != "no-cache" {
				t.Errorf("dev mode sent Cache-Control %q, want no-cache", got)
			}
		})
	}
}

// A returning visitor sends the ETag back and expects an empty 304 rather than
// the file again. The comparison has to tolerate the weak prefix the gzip
// middleware adds, or every revalidation downloads the asset a second time.
func TestAssetsConditionalGet(t *testing.T) {
	a := newTestAssets(t, testAssetFS(), false)
	path := a.URL("app.css")
	etag := serveAsset(t, a, path, nil).Header().Get("ETag")
	if etag == "" {
		t.Fatal("the asset has no ETag to revalidate against")
	}

	tests := []struct {
		name        string
		ifNoneMatch string
		want        int
	}{
		{name: "exact match", ifNoneMatch: etag, want: http.StatusNotModified},
		{name: "weakened by a proxy", ifNoneMatch: "W/" + etag, want: http.StatusNotModified},
		{name: "wildcard", ifNoneMatch: "*", want: http.StatusNotModified},
		{name: "one of several candidates", ifNoneMatch: `"stale", ` + etag, want: http.StatusNotModified},
		{name: "candidates with untrimmed spacing", ifNoneMatch: `  "stale" ,   ` + etag + ` `, want: http.StatusNotModified},
		{name: "stale validator", ifNoneMatch: `"0000000000000000"`, want: http.StatusOK},
		{name: "empty header", ifNoneMatch: "", want: http.StatusOK},
		{name: "junk", ifNoneMatch: "not-an-etag", want: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			if tt.ifNoneMatch != "" {
				h.Set("If-None-Match", tt.ifNoneMatch)
			}
			rec := serveAsset(t, a, path, h)

			if rec.Code != tt.want {
				t.Errorf("If-None-Match %q got %d, want %d", tt.ifNoneMatch, rec.Code, tt.want)
			}
			if tt.want == http.StatusNotModified && rec.Body.Len() != 0 {
				t.Errorf("a 304 carried %d bytes of body; the point of revalidating is to send none", rec.Body.Len())
			}
			if tt.want == http.StatusOK && rec.Body.Len() == 0 {
				t.Error("a 200 carried no body, so the page loads unstyled")
			}
		})
	}
}

func TestEtagMatches(t *testing.T) {
	tests := []struct {
		header, etag string
		want         bool
	}{
		{`"abc"`, `"abc"`, true},
		{`W/"abc"`, `"abc"`, true},
		{`"abc"`, `W/"abc"`, true},
		{`W/"abc"`, `W/"abc"`, true},
		{"*", `"abc"`, true},
		{` * `, `"abc"`, true},
		{`"x", "y", "abc"`, `"abc"`, true},
		{`  "abc"  `, `"abc"`, true},
		{`"x", "y"`, `"abc"`, false},
		{"", `"abc"`, false},
		{`"ab"`, `"abc"`, false},
		{`abc`, `"abc"`, false},
	}

	for _, tt := range tests {
		t.Run(tt.header+" vs "+tt.etag, func(t *testing.T) {
			if got := etagMatches(tt.header, tt.etag); got != tt.want {
				t.Errorf("etagMatches(%q, %q) = %v, want %v", tt.header, tt.etag, got, tt.want)
			}
		})
	}
}

// The asset table is an exact-match map, which is what makes traversal
// impossible — but that is a property worth pinning, because the obvious
// "refactor" to reading straight off the filesystem would hand out templates,
// configuration and anything else sharing the embedded FS.
func TestAssetsRejectsPathTraversal(t *testing.T) {
	a := newTestAssets(t, testAssetFS(), false)

	// The fixture really does contain the secret, so a leak would be visible.
	if _, err := fs.ReadFile(testAssetFS(), "static/secret.txt"); err != nil {
		t.Fatalf("fixture is broken; the traversal tests would pass vacuously: %v", err)
	}

	hostile := []string{
		"/assets/../secret.txt",
		"/assets/../static/secret.txt",
		"/assets/../../etc/passwd",
		"/assets/%2e%2e/secret.txt",
		"/assets/%2e%2e%2fsecret.txt",
		"/assets/..%2fsecret.txt",
		"/assets/..%252fsecret.txt",
		"/assets/....//secret.txt",
		"/assets/./../secret.txt",
		"/assets/img/../../secret.txt",
		"/assets/static/secret.txt",
		"/assets/templates/pages/index.html",
		"/secret.txt",
		"/static/secret.txt",
		"//assets/img/logo.png",
		"/assets//img/logo.png",
		"/assets/img/logo.png%00.txt",
		"/assets/img/logo.png%00",
		"/assets/IMG/LOGO.PNG",
		"/assets/",
		"/assets",
	}

	for _, target := range hostile {
		t.Run(strconv.Quote(target), func(t *testing.T) {
			rec := serveAsset(t, a, target, nil)
			if rec.Code != http.StatusNotFound {
				t.Errorf("got %d, want 404 — this path is not an asset", rec.Code)
			}
			if strings.Contains(rec.Body.String(), secretBody) {
				t.Fatalf("%s leaked a file that is not an asset; anything in the embedded filesystem is reachable", target)
			}
		})
	}

	// The same paths through a ServeMux, which cleans them first: it may
	// redirect, but it must never produce the file.
	mux := http.NewServeMux()
	a.Register(mux)
	for _, target := range hostile {
		t.Run("via mux "+strconv.Quote(target), func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, wireRequest(t, target))
			if strings.Contains(rec.Body.String(), secretBody) {
				t.Fatalf("%s leaked a non-asset file through the mux (status %d)", target, rec.Code)
			}
		})
	}
}

func TestAssetsUnknownPathIsNotFound(t *testing.T) {
	a := newTestAssets(t, testAssetFS(), false)

	for _, target := range []string{
		"/assets/app.css",            // unfingerprinted
		"/assets/app.0000000000.css", // wrong fingerprint
		"/assets/img/missing.png",
		"/assets/fonts/",
		"/favicon.ico",
		"/",
	} {
		t.Run(target, func(t *testing.T) {
			if got := serveAsset(t, a, target, nil).Code; got != http.StatusNotFound {
				t.Errorf("got %d, want 404", got)
			}
		})
	}
}

// A HEAD must report the same headers as the GET it stands in for. Browsers
// and monitors use it to check whether a cached copy is still current, and a
// HEAD that carries a body breaks HTTP framing on a keep-alive connection.
func TestAssetsHeadRequest(t *testing.T) {
	a := newTestAssets(t, testAssetFS(), false)
	path := a.URL("app.css")

	get := serveAsset(t, a, path, nil)
	req := wireRequest(t, path)
	req.Method = http.MethodHead
	head := httptest.NewRecorder()
	a.ServeHTTP(head, req)

	if head.Code != http.StatusOK {
		t.Fatalf("HEAD got %d, want 200", head.Code)
	}
	if head.Body.Len() != 0 {
		t.Errorf("HEAD returned %d bytes of body, which desynchronises a keep-alive connection", head.Body.Len())
	}
	for _, h := range []string{"Content-Type", "ETag", "Cache-Control", "Content-Length"} {
		if got, want := head.Header().Get(h), get.Header().Get(h); got != want {
			t.Errorf("HEAD %s = %q but GET says %q", h, got, want)
		}
	}
	if got := head.Header().Get("Content-Length"); got != strconv.Itoa(get.Body.Len()) {
		t.Errorf("HEAD Content-Length = %q but the body is %d bytes", got, get.Body.Len())
	}
}

// Range support comes from http.ServeContent. Fonts in particular are fetched
// with ranges by some browsers, so a handler that ignored Range and returned
// 200 with the whole file would still work but waste the transfer.
func TestAssetsRangeRequests(t *testing.T) {
	a := newTestAssets(t, testAssetFS(), false)
	full := serveAsset(t, a, "/assets/img/logo.png", nil).Body.String()

	t.Run("a satisfiable range returns just that slice", func(t *testing.T) {
		rec := serveAsset(t, a, "/assets/img/logo.png", http.Header{"Range": {"bytes=0-4"}})
		if rec.Code != http.StatusPartialContent {
			t.Fatalf("got %d, want 206", rec.Code)
		}
		if got := rec.Body.String(); got != full[:5] {
			t.Errorf("range body = %q, want %q", got, full[:5])
		}
		if cr := rec.Header().Get("Content-Range"); !strings.HasPrefix(cr, "bytes 0-4/") {
			t.Errorf("Content-Range = %q, want it to describe bytes 0-4 of the file", cr)
		}
	})

	t.Run("a range past the end is refused", func(t *testing.T) {
		rec := serveAsset(t, a, "/assets/img/logo.png", http.Header{"Range": {"bytes=100000-"}})
		if rec.Code != http.StatusRequestedRangeNotSatisfiable {
			t.Errorf("got %d, want 416 — silently returning the whole file confuses a resuming client", rec.Code)
		}
	})

	t.Run("an empty range header is ignored", func(t *testing.T) {
		rec := serveAsset(t, a, "/assets/img/logo.png", http.Header{"Range": {"bytes="}})
		if rec.Code != http.StatusOK || rec.Body.String() != full {
			t.Errorf("got %d with %d bytes, want the whole file with 200", rec.Code, rec.Body.Len())
		}
	})

	t.Run("an unknown range unit is refused rather than served whole", func(t *testing.T) {
		rec := serveAsset(t, a, "/assets/img/logo.png", http.Header{"Range": {"pages=1-2"}})
		if rec.Code != http.StatusRequestedRangeNotSatisfiable {
			t.Errorf("got %d, want 416", rec.Code)
		}
	})
}

// Assets are compressed once at startup rather than on every request. The
// stored copy is only used when the client asks for it, and only when it is
// actually smaller than the original.
func TestAssetsPreCompression(t *testing.T) {
	a := newTestAssets(t, testAssetFS(), false)
	cssPath := a.URL("app.css")
	identity := serveAsset(t, a, cssPath, nil)

	t.Run("identity request is never encoded", func(t *testing.T) {
		if enc := identity.Header().Get("Content-Encoding"); enc != "" {
			t.Errorf("Content-Encoding = %q for a client that did not ask for it", enc)
		}
		if got, want := identity.Header().Get("Content-Length"), strconv.Itoa(identity.Body.Len()); got != want {
			t.Errorf("Content-Length = %q but the body is %s bytes", got, want)
		}
	})

	t.Run("a gzip client gets the stored copy", func(t *testing.T) {
		rec := serveAsset(t, a, cssPath, http.Header{"Accept-Encoding": {"gzip, deflate"}})
		if enc := rec.Header().Get("Content-Encoding"); enc != "gzip" {
			t.Fatalf("Content-Encoding = %q, want gzip", enc)
		}
		if !slicesContainsFold(rec.Header().Values("Vary"), "Accept-Encoding") {
			t.Error("no Vary: Accept-Encoding, so a shared cache would hand this gzipped body to an identity client")
		}
		if got := gunzip(t, rec.Body.Bytes()); got != identity.Body.String() {
			t.Error("the compressed copy does not decode to the same bytes as the identity copy")
		}
		if got, want := rec.Header().Get("Content-Length"), strconv.Itoa(rec.Body.Len()); got != want {
			t.Errorf("Content-Length = %q describes the identity body, not the %s compressed bytes on the wire", got, want)
		}
	})

	t.Run("images are not compressed at all", func(t *testing.T) {
		rec := serveAsset(t, a, "/assets/img/logo.png", http.Header{"Accept-Encoding": {"gzip"}})
		if enc := rec.Header().Get("Content-Encoding"); enc != "" {
			t.Errorf("Content-Encoding = %q on a PNG; gzipping it costs CPU and usually grows it", enc)
		}
	})

	t.Run("svg is text and is compressed", func(t *testing.T) {
		big := testAssetFS()
		big["static/img/icon.svg"] = &fstest.MapFile{
			Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg">` + strings.Repeat(`<path d="M0 0h10v10H0z"/>`, 200) + `</svg>`),
		}
		rec := serveAsset(t, newTestAssets(t, big, false), "/assets/img/icon.svg", http.Header{"Accept-Encoding": {"gzip"}})
		if enc := rec.Header().Get("Content-Encoding"); enc != "gzip" {
			t.Errorf("Content-Encoding = %q on an SVG, which is markup and compresses well", enc)
		}
	})

	t.Run("an incompressible payload is served as-is", func(t *testing.T) {
		// gzip framing costs more than it saves on a few bytes, so no
		// compressed copy is kept and the client gets the original.
		tiny := testAssetFS()
		tiny["static/robots.txt"] = &fstest.MapFile{Data: []byte("a")}
		rec := serveAsset(t, newTestAssets(t, tiny, false), "/robots.txt", http.Header{"Accept-Encoding": {"gzip"}})
		if enc := rec.Header().Get("Content-Encoding"); enc != "" {
			t.Errorf("Content-Encoding = %q on a body gzip made larger", enc)
		}
		if rec.Body.String() != "a" {
			t.Errorf("body = %q, want the original byte", rec.Body.String())
		}
	})
}

// A stylesheet served as text/plain is not applied by the browser, and a
// script served with the wrong type is refused outright under nosniff.
func TestAssetsContentTypes(t *testing.T) {
	a := newTestAssets(t, testAssetFS(), false)

	tests := []struct{ path, wantSubstring string }{
		{a.URL("app.css"), "text/css"},
		{a.URL("app.js"), "javascript"},
		{a.URL("boot.js"), "javascript"},
		{"/assets/img/logo.png", "image/png"},
		{"/assets/img/icon.svg", "image/svg"},
		{"/assets/fonts/inter.woff2", "woff2"},
		{"/robots.txt", "text/plain"},
		{"/manifest.json", "json"},
		{"/service-worker.js", "javascript"},
		{"/icon.svg", "image/svg"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := serveAsset(t, a, tt.path, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200", rec.Code)
			}
			if got := rec.Header().Get("Content-Type"); !strings.Contains(got, tt.wantSubstring) {
				t.Errorf("Content-Type = %q, want it to contain %q", got, tt.wantSubstring)
			}
		})
	}
}

func TestContentTypeFor(t *testing.T) {
	tests := []struct{ name, wantSubstring string }{
		{"style.css", "text/css"},
		{"app.js", "javascript"},
		{"icon.svg", "image/svg"},
		{"logo.png", "image/png"},
		{"robots.txt", "text/plain"},
		{"manifest.json", "json"},
		{"inter.woff2", "woff2"},
		{"site.webmanifest", "manifest"},
		// Anything unrecognised must fall back to a type the browser will not
		// execute or render, rather than to an empty header it would sniff.
		{"archive.zzz", "application/octet-stream"},
		{"LICENSE", "application/octet-stream"},
		{"", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(strconv.Quote(tt.name), func(t *testing.T) {
			if got := contentTypeFor(tt.name); !strings.Contains(got, tt.wantSubstring) {
				t.Errorf("contentTypeFor(%q) = %q, want it to contain %q", tt.name, got, tt.wantSubstring)
			}
		})
	}
}

// In dev the table is rebuilt so an edit shows up on reload. The fingerprint
// has to move with the content and the old path has to stop resolving,
// otherwise a stale tab keeps loading a file that no longer exists.
func TestAssetsDevModeRebuilds(t *testing.T) {
	fsys := testAssetFS()
	a := newTestAssets(t, fsys, true)
	before := a.URL("app.css")

	fsys["static/css/02-layout.css"] = &fstest.MapFile{Data: []byte("body{display:flex}\n")}

	if err := a.Refresh(); err != nil {
		t.Fatalf("Refresh failed, so every page after an edit would point at a dead hash: %v", err)
	}
	after := a.URL("app.css")
	if after == before {
		t.Fatal("the fingerprint did not move after an edit; the page would render with the previous stylesheet")
	}

	rec := serveAsset(t, a, after, nil)
	if !strings.Contains(rec.Body.String(), "display:flex") {
		t.Error("the rebuilt bundle does not contain the edit")
	}
	if got := serveAsset(t, a, before, nil).Code; got != http.StatusNotFound {
		t.Errorf("the superseded path still returns %d; a stale reference would look fine and silently serve old CSS", got)
	}
}

// A request in dev rebuilds on its own, so an asset added while the server is
// running is served without a restart.
func TestAssetsDevModeRebuildsOnRequest(t *testing.T) {
	fsys := testAssetFS()
	a := newTestAssets(t, fsys, true)

	fsys["static/img/new-banner.png"] = &fstest.MapFile{Data: []byte("\x89PNG added at runtime")}

	if got := serveAsset(t, a, "/assets/img/new-banner.png", nil).Code; got != http.StatusOK {
		t.Errorf("a newly added image returned %d in dev mode, want 200 without a restart", got)
	}
}

// In production the table is built once. Rebuilding per request would cost a
// full re-hash and re-gzip of every asset on every hit.
func TestAssetsProductionIgnoresChanges(t *testing.T) {
	fsys := testAssetFS()
	a := newTestAssets(t, fsys, false)
	before := a.URL("app.css")

	fsys["static/css/02-layout.css"] = &fstest.MapFile{Data: []byte("body{display:flex}\n")}
	if err := a.Refresh(); err != nil {
		t.Fatalf("Refresh should be a no-op in production, got %v", err)
	}

	if got := a.URL("app.css"); got != before {
		t.Errorf("the production table was rebuilt: URL moved from %q to %q", before, got)
	}
	if body := serveAsset(t, a, before, nil).Body.String(); strings.Contains(body, "display:flex") {
		t.Error("the production table picked up a filesystem change; it is meant to be frozen at startup")
	}
	fsys["static/img/late.png"] = &fstest.MapFile{Data: []byte("\x89PNG")}
	if got := serveAsset(t, a, "/assets/img/late.png", nil).Code; got != http.StatusNotFound {
		t.Errorf("a file added after startup returned %d in production, want 404", got)
	}
}

// failingFS makes one path unreadable. Embedding the interface rather than the
// concrete MapFS keeps ReadDir and Glob on their fallback paths, which is what
// the asset builder actually uses.
type failingFS struct {
	fs.FS
	fail string
}

func (f failingFS) Open(name string) (fs.File, error) {
	if name == f.fail {
		return nil, fmt.Errorf("simulated I/O failure reading %s", name)
	}
	return f.FS.Open(name)
}

// A file the directory listing promises but that cannot be read is a broken
// build, not something to paper over: starting up with half an asset table
// would serve a site with missing images and no indication why.
func TestAssetsBuildFailureIsReported(t *testing.T) {
	tests := []struct{ name, fail string }{
		{"an image that cannot be read", "static/img/logo.png"},
		{"a font that cannot be read", "static/fonts/inter.woff2"},
		{"a stylesheet in the bundle", "static/css/01-reset.css"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAssets(failingFS{FS: testAssetFS(), fail: tt.fail}, false)
			if err == nil {
				t.Fatalf("NewAssets succeeded although %s is unreadable", tt.fail)
			}
			if !strings.Contains(err.Error(), tt.fail) {
				t.Errorf("error %q does not name the file that failed, which makes the startup failure hard to diagnose", err)
			}
		})
	}

	// Root files are optional by design — a deployment without a robots.txt is
	// fine — so an unreadable one must not take the whole build down.
	if _, err := NewAssets(failingFS{FS: testAssetFS(), fail: "static/robots.txt"}, false); err != nil {
		t.Errorf("an unreadable optional root file failed the build: %v", err)
	}
}

// In dev the rebuild happens inside the request, so a filesystem that breaks
// mid-session has to produce a 500 rather than a panic or a blank 200.
func TestAssetsDevBuildFailureReturns500(t *testing.T) {
	a := newTestAssets(t, testAssetFS(), true)
	a.fsys = failingFS{FS: testAssetFS(), fail: "static/img/logo.png"}

	rec := serveAsset(t, a, "/assets/img/logo.png", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500 when the dev rebuild fails", rec.Code)
	}
}

// A deployment that has not vendored a font, or an embed that never included
// the images, must still start. The site renders unstyled, which is bad, but a
// server that refuses to boot is worse.
func TestAssetsEmptyFilesystem(t *testing.T) {
	a := newTestAssets(t, fstest.MapFS{}, false)

	if a.Has("app.css") {
		t.Error("Has reports a bundle that was never built; templates would emit a preload for nothing")
	}
	if got := a.URL("app.css"); got != "/app.css" {
		t.Errorf("URL(%q) = %q, want the visible-404 fallback", "app.css", got)
	}
	if got := serveAsset(t, a, "/assets/app.css", nil).Code; got != http.StatusNotFound {
		t.Errorf("got %d, want 404", got)
	}

	mux := http.NewServeMux()
	a.Register(mux) // must not panic on an empty table

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, wireRequest(t, "/robots.txt"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("a root file that does not exist returned %d, want 404", rec.Code)
	}
}

// The root paths are referenced from outside our own HTML — by the manifest,
// by browsers hunting for a favicon, and by service workers already registered
// in returning visitors' browsers — so they have to be routed individually
// rather than living under /assets/.
func TestAssetsRegister(t *testing.T) {
	a := newTestAssets(t, testAssetFS(), false)
	mux := http.NewServeMux()
	a.Register(mux)

	t.Run("fingerprinted bundles are reachable", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, wireRequest(t, a.URL("app.css")))
		if rec.Code != http.StatusOK {
			t.Errorf("got %d, want 200", rec.Code)
		}
	})

	for _, path := range []string{"/robots.txt", "/manifest.json", "/icon.svg", "/logo.png", "/service-worker.js"} {
		t.Run("root file "+path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, wireRequest(t, path))
			if rec.Code != http.StatusOK {
				t.Errorf("got %d, want 200 — this URL is referenced from outside our HTML", rec.Code)
			}
		})
	}

	// The fixture has no logo-192.png, so no route may have been registered.
	t.Run("an absent root file is not routed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, wireRequest(t, "/logo-192.png"))
		if rec.Code != http.StatusNotFound {
			t.Errorf("got %d, want 404", rec.Code)
		}
	})

	// Assets are read-only; only GET is registered, so anything else must be
	// rejected by the mux rather than reaching the handler.
	t.Run("only GET and HEAD are routed", func(t *testing.T) {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			rec := httptest.NewRecorder()
			req := wireRequest(t, "/robots.txt")
			req.Method = method
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s /robots.txt got %d, want 405", method, rec.Code)
			}
		}
	})
}

// Assets sets Content-Encoding itself for the copies it pre-compressed. The
// gzip middleware must notice and leave them alone: compressing them a second
// time produces a body no browser can decode.
func TestAssetsAreNotDoubleCompressed(t *testing.T) {
	a := newTestAssets(t, testAssetFS(), false)
	h := Gzip(a)

	rec := httptest.NewRecorder()
	req := wireRequest(t, a.URL("app.css"))
	req.Header.Set("Accept-Encoding", "gzip")
	h.ServeHTTP(rec, req)

	if enc := rec.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", enc)
	}
	body := gunzip(t, rec.Body.Bytes())
	if !strings.Contains(body, "html{margin:0}") {
		t.Errorf("one gzip layer did not reveal the stylesheet; the response is compressed twice: %.40q", body)
	}
}

// A compressible asset small enough that gzip made it larger has no stored
// compressed copy, so Assets serves it as identity — including for a range
// request, where ServeContent computes Content-Range over those identity
// bytes. The gzip middleware wraps the mux in production, so it sees that 206
// on its way out; if it compressed the body, the offsets it is labelled with
// would no longer describe what the client receives and a resumed download
// would be stitched back together wrongly.
func TestAssetsRangeIsNotCompressedDownstream(t *testing.T) {
	tiny := testAssetFS()
	tiny["static/robots.txt"] = &fstest.MapFile{Data: []byte("User-agent: *\nAllow: /\n")}
	h := Gzip(newTestAssets(t, tiny, false))

	rec := httptest.NewRecorder()
	req := wireRequest(t, "/robots.txt")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=0-4")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("got %d, want 206 — the fixture no longer exercises the range path", rec.Code)
	}
	if enc := rec.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("Content-Encoding = %q on a 206; the range offsets no longer describe the bytes on the wire", enc)
	}
	if got := rec.Body.String(); got != "User-" {
		t.Errorf("body = %q, want the first five bytes the Content-Range promises", got)
	}
}

// Dev mode rebuilds the table inside ServeHTTP while templates concurrently
// call URL for the same table. Without the lock this is a straight data race,
// and the symptom in production would be an intermittent panic under load.
func TestAssetsConcurrentDevRebuild(t *testing.T) {
	a := newTestAssets(t, testAssetFS(), true)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 25 {
				path := a.URL("app.css")
				a.Has("boot.js")
				if code := serveAsset(t, a, path, nil).Code; code != http.StatusOK {
					t.Errorf("a concurrent request for %s got %d", path, code)
					return
				}
				if err := a.Refresh(); err != nil {
					t.Errorf("concurrent Refresh failed: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
