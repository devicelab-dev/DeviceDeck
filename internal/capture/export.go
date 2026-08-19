package capture

import (
	"fmt"
	"strings"
)

// ExportMaestro renders recorded steps as a Maestro YAML flow. The output
// must parse under maestro-runner's own parser — that contract is enforced
// by tests via the runner seam's ValidateFlow.
func ExportMaestro(appID string, steps []Step) string {
	var b strings.Builder
	fmt.Fprintf(&b, "appId: %s\n---\n", appID)
	fmt.Fprintf(&b, "- launchApp\n")
	for _, s := range steps {
		writeStep(&b, s)
	}
	return b.String()
}

func writeStep(b *strings.Builder, s Step) {
	switch s.Kind {
	case "assertVisible":
		// Same selector shape as a tap: whatever the recorder could
		// resolve durably, it asserts on.
		if s.ID != "" {
			fmt.Fprintf(b, "- assertVisible:\n    id: %s\n", quote(s.ID))
		} else {
			fmt.Fprintf(b, "- assertVisible:\n    text: %s\n", quote(s.Text))
		}
	case "tapOn", "longPressOn":
		if s.ID != "" {
			fmt.Fprintf(b, "- %s:\n    id: %s\n", s.Kind, quote(s.ID))
		} else {
			fmt.Fprintf(b, "- %s:\n    text: %s\n", s.Kind, quote(s.Text))
		}
	case "tapOnPoint":
		fmt.Fprintf(b, "- tapOn:\n    point: \"%s,%s\"\n", percent(s.StartX), percent(s.StartY))
	case "inputText":
		fmt.Fprintf(b, "- inputText: %s\n", quote(s.Input))
	case "pressKey":
		fmt.Fprintf(b, "- pressKey: %s\n", s.Input)
	case "swipe":
		fmt.Fprintf(b, "- swipe:\n    start: \"%s, %s\"\n    end: \"%s, %s\"\n",
			percent(s.StartX), percent(s.StartY), percent(s.EndX), percent(s.EndY))
	}
}

// percent renders a normalized coordinate as an integer percentage.
func percent(v float64) string {
	return fmt.Sprintf("%d%%", int(v*100+0.5))
}

// quote YAML-quotes a string defensively: captured identifiers and typed
// text are user data and can contain any character.
//
// Control characters must be escaped, not passed through. A raw newline
// inside a double-quoted scalar is legal YAML and folds to a space, so
// the flow parses and then silently fails to match an element whose
// label really does contain a newline — which Flutter produces routinely,
// because it merges a widget's child semantics into one label.
func quote(s string) string {
	return `"` + controlEscaper.Replace(s) + `"`
}

var controlEscaper = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
	"\n", `\n`,
	"\r", `\r`,
	"\t", `\t`,
)
