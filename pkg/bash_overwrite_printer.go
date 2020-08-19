package kbkb

import (
	"fmt"
	"strings"
)

type BashOverwritePrinter struct {
	Row int
}

func (p *BashOverwritePrinter) Print(out string) {
	Row := strings.Count(out, "\n")
	if p.Row > 0 {
		out = "\033[" + fmt.Sprint(p.Row) + "A\033[0;K" + out
	}
	if p.Row > Row {
		out = strings.Repeat("\033[\n", p.Row-Row) + out
	} else {
		p.Row = Row
	}
	fmt.Print(out)
}
