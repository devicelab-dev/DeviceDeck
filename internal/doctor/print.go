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

// line renders one result: status, name, what was found or what it is for,
// and on a problem the fix on its own line beneath.
func line(r Result, fancy bool) string {
	found := r.Found
	if found == "" {
		found = "not found"
	}
	missing := fancy && r.Status == Missing // what a platform needs stands out
	s := fmt.Sprintf("    %s %s %-34s %s\n", brand.Bold(mark(r.Status), missing),
		brand.Bold(fmt.Sprintf("%-18s", r.Name), missing), found, r.For)
	if r.Status != OK {
		s += "        " + r.Fix + "\n"
	}
	return s
}

// Print writes every result, for `devicedeck doctor`.
func Print(w io.Writer, results []Result, fancy bool) {
	var b strings.Builder
	b.WriteString("  Tools\n")
	for _, r := range results {
		b.WriteString(line(r, fancy))
	}
	_, _ = io.WriteString(w, b.String())
}

// PrintProblems writes only what is missing, for the server's startup
// message, or a single line when everything is in place.
func PrintProblems(w io.Writer, results []Result, fancy bool) {
	problems := Problems(results)
	if len(problems) == 0 {
		_, _ = io.WriteString(w, "  Tools            all found (devicedeck doctor for details)\n\n")
		return
	}
	var b strings.Builder
	b.WriteString("  Tools to check (devicedeck doctor for the full list):\n")
	for _, r := range problems {
		b.WriteString(line(r, fancy))
	}
	b.WriteString("\n")
	_, _ = io.WriteString(w, b.String())
}
