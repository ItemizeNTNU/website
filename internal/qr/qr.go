// Package qr renders QR codes as inline SVG.
//
// Server-side rather than in the browser, which the previous site did with a
// canvas element. Three reasons, in order of how much they matter here:
//
//   - The code ends up on a poster or projected at a door. A canvas fixed at
//     300 pixels blurs when printed or enlarged; SVG does not.
//   - It works with scripting disabled, and it is present in the page source
//     rather than appearing a moment later.
//   - No client-side dependency, and nothing for the Content-Security-Policy
//     to make an exception for.
package qr

import (
	"fmt"
	"html/template"
	"strings"

	"rsc.io/qr"
)

// quietZone is the mandatory margin around a code, in modules. The
// specification requires four; scanners rely on it to find the symbol, and
// omitting it is a common reason a code reads on one phone and not another.
const quietZone = 4

// SVG renders text as a QR code.
//
// Black on white, deliberately. A themed QR code is a scanner-reliability
// problem in a crowded room with poor lighting, which is exactly where this one
// gets used — the design expresses itself in the plate around the code instead.
//
// Error-correction level M tolerates a phone held at an angle without inflating
// the module count the way higher levels would.
func SVG(text string) (template.HTML, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", fmt.Errorf("encoding QR code: %w", err)
	}

	size := code.Size + quietZone*2

	var b strings.Builder
	fmt.Fprintf(&b,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" `+
			`shape-rendering="crispEdges" role="img" aria-label="QR-kode for innsjekk">`,
		size, size)
	b.WriteString(`<rect width="100%" height="100%" fill="#fff"/>`)
	b.WriteString(`<path fill="#000" d="`)

	// Horizontal runs are merged into single path commands. A code holding a
	// UUID URL is about 41 modules square; emitting one rectangle per module
	// would be several thousand path commands, where merged runs are a couple
	// of kilobytes.
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if !code.Black(x, y) {
				continue
			}
			run := 1
			for x+run < code.Size && code.Black(x+run, y) {
				run++
			}
			fmt.Fprintf(&b, "M%d %dh%dv1h-%dz", x+quietZone, y+quietZone, run, run)
			x += run - 1
		}
	}

	b.WriteString(`"/></svg>`)
	// The markup is built here from a fixed template and integers only — the
	// caller's text never reaches the output, it is encoded into modules.
	return template.HTML(b.String()), nil
}
