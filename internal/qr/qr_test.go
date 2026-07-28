package qr

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"rsc.io/qr"
)

func TestSVGStructure(t *testing.T) {
	svg, err := SVG("https://itemize.no/innsjekk-qr/3f8a1c2e-0000-4aaa-bbbb-ccccddddeeee")
	if err != nil {
		t.Fatal(err)
	}
	s := string(svg)

	for _, want := range []string{
		`xmlns="http://www.w3.org/2000/svg"`,
		`shape-rendering="crispEdges"`, // keeps module edges sharp when scaled
		`role="img"`,
		`aria-label=`,
		`fill="#fff"`, // white background, not the page's dark theme
		`fill="#000"`,
		"viewBox=",
		"</svg>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("SVG is missing %q", want)
		}
	}
}

// Scanners rely on the four-module margin to locate the symbol; without it a
// code reads on one phone and not another.
func TestSVGIncludesQuietZone(t *testing.T) {
	svg, err := SVG("test")
	if err != nil {
		t.Fatal(err)
	}
	// No path command may start inside the margin.
	for _, bad := range []string{"M0 0h", "M1 1h", "M3 3h"} {
		if strings.Contains(string(svg), bad) {
			t.Errorf("a module was drawn inside the quiet zone: %q", bad)
		}
	}
}

// A code per module would be thousands of path commands; merged runs keep it
// small enough to inline without thinking about it.
func TestSVGMergesRuns(t *testing.T) {
	svg, err := SVG("https://itemize.no/innsjekk-qr/3f8a1c2e-0000-4aaa-bbbb-ccccddddeeee")
	if err != nil {
		t.Fatal(err)
	}
	if size := len(svg); size > 8000 {
		t.Errorf("SVG is %d bytes; runs are probably not being merged", size)
	}
}

func TestSVGRejectsOversizedInput(t *testing.T) {
	if _, err := SVG(strings.Repeat("x", 10000)); err == nil {
		t.Error("expected an error for input too large to encode")
	}
}

// The run-merging above rewrites the module grid into path commands. If that
// rewrite is subtly wrong the SVG still looks like a QR code and still fails
// to scan, which is the worst possible failure at a door. This reconstructs
// the grid from the emitted path and compares it against the encoder's own.
func TestSVGPathReproducesTheCodeExactly(t *testing.T) {
	const text = "https://itemize.no/innsjekk-qr/3f8a1c2e-0000-4aaa-bbbb-ccccddddeeee"

	code, err := qr.Encode(text, qr.M)
	if err != nil {
		t.Fatal(err)
	}
	svg, err := SVG(text)
	if err != nil {
		t.Fatal(err)
	}

	// Every command looks like M<x> <y>h<run>v1h-<run>z
	re := regexp.MustCompile(`M(\d+) (\d+)h(\d+)v1h-\d+z`)
	matches := re.FindAllStringSubmatch(string(svg), -1)
	if len(matches) == 0 {
		t.Fatal("no path commands found")
	}

	painted := map[[2]int]bool{}
	for _, m := range matches {
		x, _ := strconv.Atoi(m[1])
		y, _ := strconv.Atoi(m[2])
		run, _ := strconv.Atoi(m[3])
		for i := 0; i < run; i++ {
			painted[[2]int{x + i, y}] = true
		}
	}

	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			want := code.Black(x, y)
			got := painted[[2]int{x + quietZone, y + quietZone}]
			if want != got {
				t.Fatalf("module (%d,%d): painted=%v, want %v — the code would not scan",
					x, y, got, want)
			}
		}
	}

	if len(painted) == 0 {
		t.Fatal("nothing was painted")
	}
}
