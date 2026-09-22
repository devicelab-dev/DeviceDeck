package capture

import (
	"fmt"
	"sort"
	"strings"

	"github.com/devicelab-dev/DeviceDeck/internal/brand"
)

// ExportMaestro renders recorded steps as a Maestro YAML flow. The output
// must parse under maestro-runner's own parser and use no field a target
// platform's driver would silently ignore — both contracts are enforced
// by tests via the runner seam (ValidateFlow, UnsupportedFields).
//
// Every step is preceded by a `# devicedeck:` provenance comment naming
// how it was addressed and how confident that selector is. Maestro ignores
// comments, so the flow still replays byte-for-byte on real devices; the
// comment is there for the human reviewing the capture, which is the whole
// point of a reviewable artifact.
func ExportMaestro(appID string, steps []Step) string {
	return exportMaestro(appID, steps, nil)
}

// ExportMaestroWithLint renders the flow and, when the capture hit a
// selector desert, prepends the findings as `# desert:` comments after
// the header. Maestro ignores them, so replay is unchanged; the reviewer
// reading the flow sees which screens have no durable selector and how to
// fix them, right where they would otherwise wonder why a step is a
// coordinate.
func ExportMaestroWithLint(appID string, steps []Step, findings []DesertFinding) string {
	return exportMaestro(appID, steps, findings)
}

func exportMaestro(appID string, steps []Step, findings []DesertFinding) string {
	var b strings.Builder
	b.WriteString(brand.FlowHeader())
	fmt.Fprintf(&b, "appId: %s\n", appID)
	fmt.Fprintf(&b, "name: %s\n", flowName(appID))
	fmt.Fprintf(&b, "tags:\n  - devicedeck\n  - capture\n")
	fmt.Fprintf(&b, "---\n")
	writeSecrets(&b, steps)
	for _, d := range findings {
		if d.Framework != "" {
			fmt.Fprintf(&b, "# desert (%s): %s\n", d.Framework, d.Message)
		} else {
			fmt.Fprintf(&b, "# desert: %s\n", d.Message)
		}
	}
	// A capture is recorded against a freshly launched app, so replay must
	// start from the same clean state or the first selector resolves
	// against whatever the previous session left on screen.
	fmt.Fprintf(&b, "- launchApp:\n    clearState: true\n")
	for _, s := range steps {
		fmt.Fprintf(&b, "# devicedeck: %s\n", provenance(s))
		writeStep(&b, s)
	}
	return b.String()
}

// writeSecrets emits a comment naming the env parameters a masked flow
// needs at replay, with the exact -e flags to supply them.
//
// It is deliberately a comment, not a real `env:` block: a flow-level
// `env: VAR: ""` sets a default that the runner honours OVER a command
// line `-e VAR=...`, so a declared empty default makes the secret
// un-overridable and every replay types nothing. A comment documents the
// requirement without shadowing `-e`; the value is supplied at run time
// and never lives in the artifact. Sorted and de-duplicated so a field
// typed into twice is named once. Nothing is written without secrets.
func writeSecrets(b *strings.Builder, steps []Step) {
	seen := map[string]bool{}
	var vars []string
	for _, s := range steps {
		if s.Secure && s.SecureVar != "" && !seen[s.SecureVar] {
			seen[s.SecureVar] = true
			vars = append(vars, s.SecureVar)
		}
	}
	if len(vars) == 0 {
		return
	}
	sort.Strings(vars)
	var flags strings.Builder
	for _, v := range vars {
		fmt.Fprintf(&flags, " -e %s=…", v)
	}
	fmt.Fprintf(b, "# devicedeck secrets: supply at replay with%s\n", flags.String())
}

// flowName derives a short human name from the bundle id — the last
// dotted segment ("dev.devicelab.testhive" -> "testhive") — so the flow
// reads as a named artifact rather than a full reverse-DNS string.
func flowName(appID string) string {
	if i := strings.LastIndex(appID, "."); i >= 0 && i < len(appID)-1 {
		return appID[i+1:]
	}
	if appID == "" {
		return "capture"
	}
	return appID
}

// provenance is the one-line `# devicedeck:` summary of how a step was
// addressed. It carries the selector kind and a confidence grade a
// reviewer can scan, mirroring the recorder's colour: an identifier is
// high, a text match medium, a relational or positional qualifier weaker,
// and a raw coordinate the last resort.
func provenance(s Step) string {
	switch s.Kind {
	case "tapOn", "longPressOn", "assertVisible":
		return fmt.Sprintf("%s selector=%s confidence=%s", s.Kind, selectorKind(s), confidence(s))
	case "tapOnPoint":
		return "tapOn selector=point confidence=low FALLBACK(coordinate)"
	case "inputText":
		if s.Secure {
			return "inputText secret=${" + s.SecureVar + "}"
		}
		return "inputText"
	default:
		return s.Kind
	}
}

// selectorKind names the addressing scheme a step settled on.
func selectorKind(s Step) string {
	base := "text"
	if s.ID != "" {
		base = "id"
	}
	switch {
	case s.ChildOfID != "":
		return base + "+childOf"
	case s.Index > 0:
		return base + "+index"
	default:
		return base
	}
}

// confidence grades a selector for the reviewer. An unqualified identifier
// is the durable case; a positional index is the brittle one, breaking the
// moment the screen reorders.
func confidence(s Step) string {
	switch {
	case s.Index > 0:
		return "low"
	case s.ChildOfID != "":
		return "medium"
	case s.ID != "":
		return "high"
	default:
		return "medium"
	}
}

func writeStep(b *strings.Builder, s Step) {
	switch s.Kind {
	case "assertVisible":
		// Same selector shape as a tap: whatever the recorder could
		// resolve durably, it asserts on.
		writeSelector(b, "assertVisible", s)
	case "tapOn", "longPressOn":
		writeSelector(b, s.Kind, s)
	case "tapOnPoint":
		fmt.Fprintf(b, "- tapOn:\n    point: \"%s,%s\"\n    label: %s\n",
			percent(s.StartX), percent(s.StartY), quote("tap at "+percent(s.StartX)+","+percent(s.StartY)))
	case "inputText":
		if s.Secure {
			// The secret lives in an env parameter, not the flow. ${VAR}
			// is a bare scalar, never quoted, or Maestro takes it as a
			// literal string instead of an interpolation.
			fmt.Fprintf(b, "- inputText: ${%s}\n", s.SecureVar)
		} else {
			fmt.Fprintf(b, "- inputText: %s\n", quote(s.Input))
		}
	case "pressKey":
		fmt.Fprintf(b, "- pressKey: %s\n", s.Input)
	case "swipe":
		fmt.Fprintf(b, "- swipe:\n    start: \"%s, %s\"\n    end: \"%s, %s\"\n",
			percent(s.StartX), percent(s.StartY), percent(s.EndX), percent(s.EndY))
	}
}

// writeSelector emits a selector-bearing command (tapOn/longPressOn/
// assertVisible) with its regex-escaped id or text, any narrowing
// qualifier, and a human-readable label.
func writeSelector(b *strings.Builder, cmd string, s Step) {
	if s.ID != "" {
		fmt.Fprintf(b, "- %s:\n    id: %s\n", cmd, quote(reEscape(s.ID)))
	} else {
		fmt.Fprintf(b, "- %s:\n    text: %s\n", cmd, quote(reEscape(s.Text)))
	}
	writeQualifiers(b, s)
	fmt.Fprintf(b, "    label: %s\n", quote(selectorLabel(s)))
}

// selectorLabel is the human intent Maestro shows for a step: the element
// as a person would name it, not the escaped regex the runner matches on.
func selectorLabel(s Step) string {
	if s.ID != "" {
		return s.ID
	}
	return s.Text
}

// writeQualifiers narrows a selector that would otherwise match more
// than one element. Only one is ever emitted: qualify prefers an
// ancestor and falls back to position.
func writeQualifiers(b *strings.Builder, s Step) {
	switch {
	case s.ChildOfID != "":
		fmt.Fprintf(b, "    childOf:\n      id: %s\n", quote(reEscape(s.ChildOfID)))
	case s.Index > 0:
		fmt.Fprintf(b, "    index: %d\n", s.Index)
	}
}

// percent renders a normalized coordinate as an integer percentage.
func percent(v float64) string {
	return fmt.Sprintf("%d%%", int(v*100+0.5))
}

// reEscape backslash-escapes the regex metacharacters in a selector value.
//
// Maestro treats `id:` and `text:` as regular expressions, so a captured
// identifier like "star.fill" or "user.name" would match "starXfill" as
// well as the literal, and a flow that says `tapOn: id: star.fill` could
// resolve to a different element on a screen the capture never saw. An
// exact literal still matches via Maestro's pattern-equals-value fallback,
// but escaping removes the over-match without changing what the intended
// element resolves to.
func reEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`\.+*?()|[]{}^$`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
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
