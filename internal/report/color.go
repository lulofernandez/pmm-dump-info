package report

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// ANSI styles of the text report.
const (
	styleTitle  = "1;33" // report title and section names: bold yellow
	styleHeader = "36"   // table column headers: cyan, like the Dump keys
	styleKey    = "36"   // keys of the Dump table: cyan
	styleWarn   = "33"   // warning alerts mark: yellow
	styleInfo   = "36"   // info alerts mark: cyan
)

// palette wraps text in ANSI styles when on.
type palette struct{ on bool }

func (c palette) paint(style, s string) string {
	if !c.on || s == "" {
		return s
	}
	return "\x1b[" + style + "m" + s + "\x1b[0m"
}

// table prints rows indented by two spaces, columns padded to the widest cell
// plus two spaces (the last column is not padded). With head the first row is
// styled as column headers; with keys the first column of the other rows is
// styled as keys. Widths are counted before the styles are added, so the
// columns line up with and without colors.
func table(w io.Writer, c palette, head, keys bool, rows [][]string) {
	var width []int
	for _, r := range rows {
		for i, cell := range r {
			if i == len(width) {
				width = append(width, 0)
			}
			width[i] = max(width[i], utf8.RuneCountInString(cell))
		}
	}
	for n, r := range rows {
		var b strings.Builder
		b.WriteString("  ")
		for i, cell := range r {
			pad := ""
			if i < len(r)-1 {
				pad = strings.Repeat(" ", width[i]-utf8.RuneCountInString(cell)+2)
			}
			switch {
			case head && n == 0:
				cell = c.paint(styleHeader, cell)
			case keys && i == 0:
				cell = c.paint(styleKey, cell)
			}
			b.WriteString(cell + pad)
		}
		fmt.Fprintln(w, b.String())
	}
}
