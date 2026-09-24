package mmc

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Table is a table printed with box-drawing characters
type Table struct {
	Title  string
	Header []string
	Rows   [][]string
	// Footer is printed below the rows, separated by a line, e.g. for totals
	Footer []string
	// RightAligned marks the columns aligned to the right, e.g. numbers
	RightAligned map[int]bool
}

// Print prints the table to stdout
func (t Table) Print() {
	all := append([][]string{t.Header}, t.Rows...)
	if t.Footer != nil {
		all = append(all, t.Footer)
	}

	widths := make([]int, len(t.Header))
	for _, r := range all {
		for i, cell := range r {
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
		}
	}

	line := func(left, middle, right string) {
		parts := make([]string, len(widths))
		for i, w := range widths {
			parts[i] = strings.Repeat("─", w+2)
		}
		fmt.Println(left + strings.Join(parts, middle) + right)
	}
	row := func(r []string) {
		cells := make([]string, len(widths))
		for i := range widths {
			cell := ""
			if i < len(r) {
				cell = r[i]
			}
			if t.RightAligned[i] {
				cells[i] = fmt.Sprintf(" %*s ", widths[i], cell)
			} else {
				cells[i] = fmt.Sprintf(" %-*s ", widths[i], cell)
			}
		}
		fmt.Println("│" + strings.Join(cells, "│") + "│")
	}

	if t.Title != "" {
		fmt.Printf("\n%s\n", t.Title)
	}
	line("┌", "┬", "┐")
	row(t.Header)
	line("├", "┼", "┤")
	for _, r := range t.Rows {
		row(r)
	}
	if t.Footer != nil {
		line("├", "┼", "┤")
		row(t.Footer)
	}
	line("└", "┴", "┘")
}
