// Package printer renders a kbkb field as colored ASCII art for terminals.
package printer

import (
	"fmt"
	"io"
	"strings"

	"github.com/omakeno/kbkb/v2/pkg/field"
)

var colorCodes = map[string]string{
	"red":    "31",
	"green":  "32",
	"yellow": "33",
	"blue":   "34",
	"purple": "35",
}

// CharSet defines the glyphs used to draw the field.
type CharSet struct {
	Wall         string
	Floor        string
	LeftCorner   string
	RightCorner  string
	StableIcon   string
	UnstableIcon string
	Blank        string
}

// Default is the compact single-width character set.
func Default() CharSet {
	return CharSet{
		Wall:         "|",
		Floor:        "-",
		LeftCorner:   "+",
		RightCorner:  "+",
		StableIcon:   "@",
		UnstableIcon: "o",
		Blank:        " ",
	}
}

// Wide is the double-width character set (requires a monospaced font).
func Wide() CharSet {
	return CharSet{
		Wall:         "|",
		Floor:        "--",
		LeftCorner:   "+",
		RightCorner:  "+",
		StableIcon:   "●",
		UnstableIcon: "○",
		Blank:        "  ",
	}
}

func (cs CharSet) podIcon(p *field.Pod) string {
	icon := cs.UnstableIcon
	if p.Stable() {
		icon = cs.StableIcon
	}
	code, ok := colorCodes[p.Color]
	if !ok {
		code = "0"
	}
	return "\033[0;" + code + "m" + icon + "\033[0m"
}

// Render draws the field, bottom row last, enclosed by walls and a floor.
func (cs CharSet) Render(f *field.Field) string {
	rows := f.MaxHeight()
	var b strings.Builder
	for y := rows - 1; y >= 0; y-- {
		b.WriteString(cs.Wall)
		for x := range f.Columns {
			if p := f.At(x, y); p != nil {
				b.WriteString(cs.podIcon(p))
			} else {
				b.WriteString(cs.Blank)
			}
		}
		b.WriteString(cs.Wall + "\n")
	}
	b.WriteString(cs.LeftCorner + strings.Repeat(cs.Floor, len(f.Columns)) + cs.RightCorner + "\n")
	return b.String()
}

// Overwriter prints successive frames in place by erasing the previous frame
// with ANSI escape sequences (replaces github.com/omakeno/bashoverwriter).
type Overwriter struct {
	W         io.Writer
	lastLines int
}

// Print erases the previously printed frame and writes the new one.
func (o *Overwriter) Print(frame string) {
	if o.lastLines > 0 {
		fmt.Fprintf(o.W, "\033[%dA\033[J", o.lastLines)
	}
	fmt.Fprint(o.W, frame)
	o.lastLines = strings.Count(frame, "\n")
}
