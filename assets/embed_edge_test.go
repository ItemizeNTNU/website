package assets

// What the two filesystems contain, and what happens at their edges.
//
// The embedded tree is fixed at build time and nothing about it can be checked
// by the compiler: a file that go:embed quietly skipped, a stylesheet that was
// renamed out from under a template, or a stray editor backup that got shipped
// inside the binary all build perfectly well and only show up in production.
// Everything here is about catching that at test time instead.

import (
	"html/template"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// walkFiles lists every file (not directory) under the given roots, sorted.
func walkFiles(t *testing.T, fsys fs.FS, roots ...string) []string {
	t.Helper()

	var out []string
	for _, root := range roots {
		err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				out = append(out, p)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %q failed: %v", root, err)
		}
	}
	slices.Sort(out)
	return out
}

// The embedded tree must be exactly the tree on disk.
//
// go:embed silently skips files whose names begin with a dot or an underscore.
// A contributor adding _draft.html or .htaccess under assets/ sees it work in
// -dev mode, where the files are read from disk, and never learn that it is
// missing from the binary everyone else runs. Comparing the two filesystems is
// the only way that difference ever becomes visible.
func TestEmbeddedTreeMatchesTheRepository(t *testing.T) {
	// The test binary runs in its own package directory, so the repository
	// root — which is what the -dev filesystem is relative to — is one level
	// up. t.Chdir restores the previous directory when the test ends.
	t.Chdir("..")

	embeddedFiles := walkFiles(t, FS(false), "templates", "static")
	diskFiles := walkFiles(t, os.DirFS("assets"), "templates", "static")

	if slices.Equal(embeddedFiles, diskFiles) {
		return
	}

	for _, name := range diskFiles {
		if !slices.Contains(embeddedFiles, name) {
			t.Errorf("%s exists on disk but is not in the binary. go:embed skips "+
				"names beginning with a dot or an underscore, so this file works "+
				"in -dev mode and is missing everywhere else.", name)
		}
	}
	for _, name := range embeddedFiles {
		if !slices.Contains(diskFiles, name) {
			t.Errorf("%s is embedded but no longer exists on disk; the binary is "+
				"serving something no one can edit", name)
		}
	}
}

// The embed patterns name two directories, and nothing else may come along.
// A widened pattern would pull this test file, embed.go and anything else in
// the package into the shipped binary.
func TestEmbeddedRootHoldsOnlyTheAssetDirectories(t *testing.T) {
	entries, err := fs.ReadDir(FS(false), ".")
	if err != nil {
		t.Fatalf("reading the embed root failed: %v", err)
	}

	var got []string
	for _, e := range entries {
		if !e.IsDir() {
			t.Errorf("%s is embedded at the top level; only the templates and "+
				"static directories belong in the binary", e.Name())
		}
		got = append(got, e.Name())
	}
	slices.Sort(got)

	if want := []string{"static", "templates"}; !slices.Equal(got, want) {
		t.Errorf("embed root = %v, want %v", got, want)
	}
}

// Every directory the server reaches for by name. An absent one is not a build
// error — fs.Glob simply returns nothing and ReadDir is treated as optional by
// the asset builder — so the site comes up unstyled or without images instead
// of failing loudly.
func TestExpectedDirectoriesArePresent(t *testing.T) {
	dirs := map[string]string{
		"templates/layout":   "every page render starts from the layout",
		"templates/pages":    "the renderer refuses to start without page templates",
		"templates/partials": "pages that include partials would fail to parse",
		"static/css":         "the site would render unstyled",
		"static/js":          "client-side behaviour would be missing",
		"static/img":         "the logo and favicon would 404",
		"static/fonts":       "the page would fall back to system fonts",
	}

	for dir, why := range dirs {
		t.Run(dir, func(t *testing.T) {
			entries, err := fs.ReadDir(FS(false), dir)
			if err != nil {
				t.Fatalf("%s is not embedded: %v. %s.", dir, err, why)
			}
			if len(entries) == 0 {
				t.Errorf("%s is embedded but empty. %s.", dir, why)
			}
		})
	}
}

// Nothing that is not an asset may be shipped inside the binary. Editor
// backups, macOS metadata and source maps are all things that arrive by
// accident, and once embedded they are served to anyone who guesses the path.
func TestNoJunkIsEmbedded(t *testing.T) {
	// One mebibyte. The largest asset today is a 320 KB SVG; anything past
	// this is a file that was committed by mistake, and every byte of it is
	// carried by every deployment.
	const maxFileSize = 1 << 20

	badNames := []string{".DS_Store", "Thumbs.db", "desktop.ini"}
	badSuffixes := []string{
		".map",                                       // a source map exposes the unminified original
		".bak", ".orig", ".rej", ".swp", ".swo", "~", // editor and merge leftovers
		".go",                    // no source belongs in the asset tree
		".psd", ".ai", ".sketch", // design sources; large and useless at runtime
	}

	fsys := FS(false)
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		name := path.Base(p)
		if slices.Contains(badNames, name) {
			t.Errorf("%s is embedded; it is metadata, not an asset", p)
		}
		for _, suffix := range badSuffixes {
			if strings.HasSuffix(name, suffix) {
				t.Errorf("%s is embedded and ends in %q, which is not something "+
					"the site serves on purpose", p, suffix)
			}
		}
		if strings.HasPrefix(name, "#") {
			t.Errorf("%s looks like an Emacs autosave file", p)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() == 0 {
			t.Errorf("%s is embedded but empty; a truncated asset is served as a "+
				"blank page rather than a 404, which is far harder to notice", p)
		}
		if info.Size() > maxFileSize {
			t.Errorf("%s is %d bytes, over the %d byte limit this test pins. Every "+
				"deployment carries it; if it really belongs in the binary, raise "+
				"the limit deliberately.", p, info.Size(), maxFileSize)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the embedded filesystem failed: %v", err)
	}
}

// The production filesystem must not depend on where the process was started
// from, and the development one must. That difference is the entire point of
// the flag: a deployment that suddenly needed a working directory would fail
// only once it was running somewhere other than the build machine.
func TestFSSelection(t *testing.T) {
	const probe = "templates/layout/base.html"

	t.Run("embedded is independent of the working directory", func(t *testing.T) {
		t.Chdir(t.TempDir()) // nothing resembling the repository is here

		if _, err := fs.ReadFile(FS(false), probe); err != nil {
			t.Errorf("the embedded filesystem could not read %s from an unrelated "+
				"directory: %v. Production would depend on where it was started.",
				probe, err)
		}
		if _, err := fs.ReadFile(FS(true), probe); err == nil {
			t.Error("the -dev filesystem found the templates in a directory that " +
				"has none, so it is not reading from disk at all")
		}
	})

	t.Run("dev reads the working copy", func(t *testing.T) {
		t.Chdir("..") // the repository root, which is where -dev must be run

		disk, err := fs.ReadFile(FS(true), probe)
		if err != nil {
			t.Fatalf("the -dev filesystem could not read %s from the repository "+
				"root: %v. Template edits would not show up on reload.", probe, err)
		}
		embedded, err := fs.ReadFile(FS(false), probe)
		if err != nil {
			t.Fatalf("reading %s from the embed failed: %v", probe, err)
		}
		if string(disk) != string(embedded) {
			t.Error("the -dev and embedded copies of the layout differ, so the " +
				"two modes are reading different trees")
		}
	})
}

// Paths that are not in the form io/fs defines must be refused rather than
// quietly normalised. The asset server builds lookups from request paths, and
// an fs that resolved "../" or a leading slash would widen what a crafted
// request can reach beyond the two embedded directories.
func TestEmbeddedRefusesUnnormalisedPaths(t *testing.T) {
	const real = "templates/layout/base.html"
	fsys := FS(false)

	if _, err := fs.ReadFile(fsys, real); err != nil {
		t.Fatalf("the control case failed: %v", err)
	}

	for _, p := range []string{
		"/templates/layout/base.html",             // absolute
		"./templates/layout/base.html",            // a dot segment
		"templates//layout/base.html",             // an empty segment
		"templates/../templates/layout/base.html", // a traversal that resolves inside
		"static/../../assets/embed.go",            // one that resolves outside
		"templates/layout/../layout/base.html",    // and one in the middle
		"templates/layout/base.html/",             // a trailing slash
	} {
		t.Run(p, func(t *testing.T) {
			if _, err := fs.ReadFile(fsys, p); err == nil {
				t.Errorf("%q resolved to a file; io/fs paths are unrooted and "+
					"already clean, and an fs that normalises them makes every "+
					"caller responsible for doing so first", p)
			}
		})
	}

	// A directory is not a file, however it is asked for.
	for _, p := range []string{"templates", "static/css", "."} {
		if _, err := fs.ReadFile(fsys, p); err == nil {
			t.Errorf("reading the directory %q as a file succeeded", p)
		}
	}
}

// fs.Sub is how a caller narrows the filesystem to one subtree. embed.FS does
// not implement it natively, so the wrapper is lazy: an invalid path fails at
// once, a merely absent one fails only when read. Both have to be errors and
// neither may panic.
func TestSubFilesystems(t *testing.T) {
	fsys := FS(false)

	t.Run("a subtree serves its own paths", func(t *testing.T) {
		static, err := fs.Sub(fsys, "static")
		if err != nil {
			t.Fatalf("fs.Sub(static) failed: %v", err)
		}
		if _, err := fs.ReadFile(static, "img/icon.svg"); err != nil {
			t.Errorf("img/icon.svg is not readable under the static subtree: %v", err)
		}
		// The prefix is gone, not optional.
		if _, err := fs.ReadFile(static, "static/img/icon.svg"); err == nil {
			t.Error("the full path still resolves under the subtree, so fs.Sub " +
				"did not narrow anything")
		}
		// And the subtree cannot see its siblings.
		if _, err := fs.ReadFile(static, "templates/layout/base.html"); err == nil {
			t.Error("the templates are reachable from the static subtree")
		}
	})

	t.Run("the root subtree is the whole filesystem", func(t *testing.T) {
		same, err := fs.Sub(fsys, ".")
		if err != nil {
			t.Fatalf("fs.Sub(.) failed: %v", err)
		}
		if _, err := fs.ReadFile(same, "templates/layout/base.html"); err != nil {
			t.Errorf("the layout is unreadable through fs.Sub(.): %v", err)
		}
	})

	t.Run("invalid subtree paths are refused", func(t *testing.T) {
		for _, p := range []string{"/static", "static/", "./static", "../assets", ""} {
			if _, err := fs.Sub(fsys, p); err == nil {
				t.Errorf("fs.Sub(%q) was accepted; only unrooted, clean paths are "+
					"filesystem paths", p)
			}
		}
	})

	t.Run("an absent subtree fails on read, not on Sub", func(t *testing.T) {
		missing, err := fs.Sub(fsys, "static/does-not-exist")
		if err != nil {
			t.Skipf("fs.Sub now validates existence (%v), which is stricter than "+
				"this test assumed", err)
		}
		if _, err := fs.ReadFile(missing, "anything.css"); err == nil {
			t.Error("reading through a subtree that does not exist succeeded")
		}
	})
}

// templateFuncs pins the helper names the templates are allowed to call. The
// real map is built in internal/web; naming them here keeps this package from
// depending on the server, and a template calling something new fails to parse
// with a message pointing straight at this list.
var templateFuncs = []string{
	"asset", "hasAsset", "eml", "emlfallback", "csrf",
	"smartTime", "dict", "list", "hasRole",
}

func stubFuncs() template.FuncMap {
	funcs := template.FuncMap{}
	for _, name := range templateFuncs {
		funcs[name] = func(...any) any { return nil }
	}
	return funcs
}

// Every page must parse together with the layout and all the partials, the
// same combination the renderer builds. A template that does not parse is a
// 500 on that page and nothing else — the rest of the site keeps working, so
// nobody notices until someone visits it.
func TestEveryPageParses(t *testing.T) {
	fsys := FS(false)

	pages, err := fs.Glob(fsys, "templates/pages/*.html")
	if err != nil {
		t.Fatalf("globbing pages failed: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("no page templates are embedded; the renderer refuses to start")
	}

	for _, page := range pages {
		t.Run(path.Base(page), func(t *testing.T) {
			_, err := template.New("base.html").
				Funcs(stubFuncs()).
				ParseFS(fsys, "templates/layout/*.html", "templates/partials/*.html", page)
			if err != nil {
				t.Errorf("%s does not parse: %v. If the failure names an undefined "+
					"function, add it to templateFuncs here and to web.Funcs.", page, err)
			}
		})
	}
}

var templateCall = regexp.MustCompile(`\{\{-?\s*template\s+"([^"]+)"`)

// Every {{ template }} invocation has to name something that exists. Missing
// ones are only found at execution time, and only on the branch that reaches
// them — so a partial dropped from a conditional survives every render until
// the day the condition is true.
func TestEveryTemplateInvocationIsDefined(t *testing.T) {
	fsys := FS(false)

	set, err := template.New("base.html").
		Funcs(stubFuncs()).
		ParseFS(fsys,
			"templates/layout/*.html",
			"templates/partials/*.html",
			"templates/pages/*.html")
	if err != nil {
		t.Fatalf("parsing the whole template tree failed: %v", err)
	}

	var checked int
	for _, name := range walkFiles(t, fsys, "templates") {
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			t.Fatalf("reading %s failed: %v", name, err)
		}
		for _, match := range templateCall.FindAllStringSubmatch(string(body), -1) {
			checked++
			called := match[1]
			if defined := set.Lookup(called); defined == nil || defined.Tree == nil {
				t.Errorf("%s invokes the template %q, which nothing defines; the "+
					"page renders until that branch is taken and then 500s",
					name, called)
			}
		}
	}
	// The layout alone pulls in several partials, so finding none means the
	// pattern stopped matching rather than the invocations going away.
	if checked == 0 {
		t.Error("no {{ template }} invocations were found in the whole tree, so " +
			"this test checked nothing")
	}
}

var (
	assetCall = regexp.MustCompile(`\{\{-?\s*(?:hasAsset|asset)\s+"([^"]+)"`)
	// Only literal paths: anything holding a template action is resolved at
	// render time and cannot be checked here.
	markupRef   = regexp.MustCompile(`(?:href|src)="(/[^"{]+)"`)
	manifestRef = regexp.MustCompile(`"src"\s*:\s*"([^"]+)"`)
)

// rootFileSources mirrors the table in internal/httpx: files served from /
// because their URLs are referenced from outside our own HTML.
var rootFileSources = map[string]string{
	"/icon.svg":          "static/img/icon.svg",
	"/logo.png":          "static/img/logo.png",
	"/logo-192.png":      "static/img/logo-192.png",
	"/logo-512.png":      "static/img/logo-512.png",
	"/manifest.json":     "static/manifest.json",
	"/robots.txt":        "static/robots.txt",
	"/service-worker.js": "static/service-worker.js",
}

// Every file the markup points at must exist.
//
// An unknown logical name does not fail: the asset resolver returns "/" + the
// name so the mistake becomes a 404 rather than a crash. That is the right
// runtime behaviour and a terrible way to find out, because the page still
// renders — unstyled, or without its logo, with nothing in the server log.
func TestReferencedFilesExist(t *testing.T) {
	fsys := FS(false)

	// Logical names, as passed to the asset helper. Stylesheets and scripts
	// are bundles built from a whole directory; everything else is one file
	// under static/.
	resolve := func(name string) (string, bool) {
		switch name {
		case "app.css":
			matches, _ := fs.Glob(fsys, "static/css/*.css")
			return "static/css/*.css", len(matches) > 0
		case "app.js":
			matches, _ := fs.Glob(fsys, "static/js/*.js")
			return "static/js/*.js", len(matches) > 0
		case "boot.js":
			_, err := fs.Stat(fsys, "static/boot.js")
			return "static/boot.js", err == nil
		default:
			source := "static/" + name
			_, err := fs.Stat(fsys, source)
			return source, err == nil
		}
	}

	var checked int
	for _, name := range walkFiles(t, fsys, "templates") {
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			t.Fatalf("reading %s failed: %v", name, err)
		}
		text := string(body)

		for _, match := range assetCall.FindAllStringSubmatch(text, -1) {
			checked++
			source, ok := resolve(match[1])
			if !ok {
				t.Errorf("%s asks for the asset %q, which nothing under %s "+
					"provides; the page would link a path that 404s",
					name, match[1], source)
			}
		}

		for _, match := range markupRef.FindAllStringSubmatch(text, -1) {
			ref := match[1]
			if path.Ext(ref) == "" {
				continue // a route, not a file
			}
			checked++
			source, served := rootFileSources[ref]
			if !served {
				t.Errorf("%s links %s, which is not one of the files served from "+
					"the site root; either add it to internal/httpx or fix the link",
					name, ref)
				continue
			}
			if _, err := fs.Stat(fsys, source); err != nil {
				t.Errorf("%s links %s, which is served from %s — but that file is "+
					"not embedded: %v", name, ref, source, err)
			}
		}
	}

	// The layout alone links the stylesheet, two scripts and the favicon, so
	// an empty run means the patterns stopped matching the markup.
	if checked == 0 {
		t.Error("no asset references were found in any template, so this test " +
			"checked nothing")
	}

	// The manifest is not markup, but browsers fetch what it points at and a
	// missing icon there is an install prompt with a blank square.
	manifest, err := fs.ReadFile(fsys, "static/manifest.json")
	if err != nil {
		t.Fatalf("reading the web app manifest failed: %v", err)
	}
	for _, match := range manifestRef.FindAllStringSubmatch(string(manifest), -1) {
		source, served := rootFileSources[match[1]]
		if !served {
			t.Errorf("the manifest references %s, which is not served from the "+
				"site root", match[1])
			continue
		}
		if _, err := fs.Stat(fsys, source); err != nil {
			t.Errorf("the manifest references %s, but %s is not embedded: %v",
				match[1], source, err)
		}
	}
}

// The two fonts the layout preloads. The preload is guarded by hasAsset, so a
// renamed font file does not break the page — it silently stops being
// preloaded, and the first paint waits for the font instead.
func TestPreloadedFontsExist(t *testing.T) {
	fsys := FS(false)

	layout, err := fs.ReadFile(fsys, "templates/layout/base.html")
	if err != nil {
		t.Fatalf("reading the layout failed: %v", err)
	}

	fonts := regexp.MustCompile(`"(fonts/[^"]+\.woff2)"`).
		FindAllStringSubmatch(string(layout), -1)
	if len(fonts) == 0 {
		t.Fatal("the layout preloads no fonts at all; if that is deliberate, " +
			"this test should go")
	}
	for _, match := range fonts {
		if _, err := fs.Stat(fsys, "static/"+match[1]); err != nil {
			t.Errorf("the layout preloads %s, which is not embedded: %v. The "+
				"hasAsset guard means the page still renders, so this would only "+
				"show up as a slower first paint.", match[1], err)
		}
	}
}
