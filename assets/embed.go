// Package assets embeds the site's templates, stylesheets, scripts, fonts and
// images into the binary.
//
// The embed root has to live in a package that physically contains the files —
// //go:embed cannot reach into a parent directory — which is why this sits at
// the top level rather than under internal/. Keeping it here also means a
// contributor editing a template never has to go looking inside internal/.
package assets

import (
	"embed"
	"io/fs"
	"os"
)

//go:embed templates static
var embedded embed.FS

// FS returns the asset filesystem. When dev is true it reads from disk
// relative to the repository root, so template and stylesheet edits take
// effect on reload; otherwise it serves the copies compiled into the binary.
func FS(dev bool) fs.FS {
	if dev {
		return os.DirFS("assets")
	}
	return embedded
}
