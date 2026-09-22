package doctor

import (
	"fmt"
	"io"
	"strings"

	"github.com/devicelab-dev/DeviceDeck/internal/brand"
)

// mark is the one-character status shown before each check.
func mark(s Status) string {
	switch s {
	case OK:
		return "✓"
	case Missing:
		return "✗"
	default:
		return "-"
	}
}

// Line renders one result, indented to sit under a section heading: status,
// name, what was found, what it is for (dimmed), and on a problem the fix on
// its own line beneath.
func Line(r Result, fancy bool) string {
	found := r.Found
	if found == "" {
		found = "not found"
	}
	missing := fancy && r.Status == Missing // what a platform needs stands out
	s := fmt.Sprintf("    %s %s %-26s %s\n", brand.Bold(mark(r.Status), missing),
		brand.Bold(fmt.Sprintf("%-17s", r.Name), missing), found, brand.Dim(r.For, fancy))
	if r.Status != OK {
		s += "      " + brand.Dim("→ ", fancy) + r.Fix + "\n"
	}
	return s
}

// Print writes every result under a heading, for `devicedeck doctor`.
func Print(w io.Writer, results []Result, fancy bool) {
	var b strings.Builder
	b.WriteString("  " + brand.Bold("TOOLS", fancy) + "\n")
	for _, r := range results {
		b.WriteString(Line(r, fancy))
	}
	_, _ = io.WriteString(w, b.String())
}
