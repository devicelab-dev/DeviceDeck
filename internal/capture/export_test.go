package capture

import (
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/devicelab-dev/DeviceDeck/internal/runner"
)

func TestExportMaestroParsesUnderRealRunner(t *testing.T) {
	steps := []Step{
		{Kind: "tapOn", ID: "loginButton"},
		{Kind: "inputText", Input: `user "quoted" \ backslash`},
		{Kind: "pressKey", Input: "Enter"},
		{Kind: "tapOn", Text: "Welcome back"},
		{Kind: "longPressOn", ID: "cardCell"},
		{Kind: "swipe", StartX: 0.5, StartY: 0.8, EndX: 0.5, EndY: 0.2},
		{Kind: "tapOnPoint", StartX: 0.25, StartY: 0.75},
		{Kind: "pressKey", Input: "Home"},
	}
	yaml := ExportMaestro("dev.devicelab.testhive", steps)

	// The fidelity contract: maestro-runner's own parser must accept the
	// export and see every step (+1 for launchApp).
	count, err := runner.ValidateFlow([]byte(yaml))
	if err != nil {
		t.Fatalf("maestro-runner rejected export:\n%s\nerror: %v", yaml, err)
	}
	if count != len(steps)+1 {
		t.Fatalf("parsed %d steps, want %d\n%s", count, len(steps)+1, yaml)
	}

	for _, want := range []string{
		"appId: dev.devicelab.testhive",
		"- launchApp",
		`id: "loginButton"`,
		`text: "Welcome back"`,
		`- swipe`,
		`start: "50%, 80%"`,
		`point: "25%,75%"`,
	} {
		if !strings.Contains(yaml, want) {
			t.Errorf("export missing %q:\n%s", want, yaml)
		}
	}
}

func TestExportEmptySession(t *testing.T) {
	yaml := ExportMaestro("com.example.app", nil)
	count, err := runner.ValidateFlow([]byte(yaml))
	if err != nil || count != 1 {
		t.Fatalf("empty session export invalid (%d steps, %v):\n%s", count, err, yaml)
	}
}

func TestValidateFlowRejectsGarbage(t *testing.T) {
	if _, err := runner.ValidateFlow([]byte("appId: x\n---\n- notARealCommand: 1\n")); err == nil {
		t.Fatal("expected parser rejection")
	}
}

// Flutter merges a widget's child semantics into one label, so labels
// routinely arrive with embedded newlines. A raw newline in a
// double-quoted scalar folds to a space, which parses cleanly and then
// matches nothing — so the escape has to survive a round trip.
func TestQuoteEscapesControlCharacters(t *testing.T) {
	label := "#182604 — OverlayPortal\nSemantics\tregression\r\\ \"quoted\""
	yamlText := ExportMaestro("com.example", []Step{{Kind: "tapOn", Text: label}})
	if strings.Contains(yamlText, "\n    text: \"#182604 — OverlayPortal\nSemantics") {
		t.Error("newline emitted raw into the scalar")
	}
	if _, err := runner.ValidateFlow([]byte(yamlText)); err != nil {
		t.Fatalf("exported flow invalid: %v\n%s", err, yamlText)
	}
	// launchApp is a bare scalar step, so decode loosely and dig.
	var steps []any
	parts := strings.SplitN(yamlText, "---\n", 2)
	if err := yaml.Unmarshal([]byte(parts[1]), &steps); err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	var got string
	for _, step := range steps {
		m, ok := step.(map[string]any)
		if !ok {
			continue
		}
		if tap, ok := m["tapOn"].(map[string]any); ok {
			got, _ = tap["text"].(string)
		}
	}
	// text is now emitted as a regex (id/text are regexes in Maestro), so
	// the decoded scalar is the regex-escaped label, and that regex must
	// still match the original label exactly.
	want := reEscape(label)
	if got != want {
		t.Errorf("round trip changed the selector:\n got %q\nwant %q", got, want)
	}
	re, err := regexp.Compile(got)
	if err != nil {
		t.Fatalf("emitted text is not a valid regex: %v", err)
	}
	if !re.MatchString(label) {
		t.Errorf("escaped regex %q does not match the label it came from", got)
	}
}

// An element with no identifier is asserted by its text, the same
// fallback a tap uses.
func TestExportAssertVisibleByText(t *testing.T) {
	yaml := ExportMaestro("com.example", []Step{{Kind: "assertVisible", Text: "Welcome Back"}})
	if !strings.Contains(yaml, "- assertVisible:\n    text: \"Welcome Back\"") {
		t.Errorf("text assertion missing:\n%s", yaml)
	}
	if _, err := runner.ValidateFlow([]byte(yaml)); err != nil {
		t.Fatalf("assertion flow invalid: %v\n%s", err, yaml)
	}
}

// The emitted flow carries the reviewer-facing scaffolding a captured
// artifact needs: a name, tags, a clean-state launch, and a provenance
// comment grading every step's selector — none of which changes what the
// runner replays, all of which a person reads.
func TestExportHeaderAndProvenance(t *testing.T) {
	steps := []Step{
		{Kind: "tapOn", ID: "loginButton"},
		{Kind: "tapOn", Text: "Continue"},
		{Kind: "tapOn", ID: "row-cta", ChildOfID: "alice-row"},
		{Kind: "tapOn", ID: "cell", Index: 2},
		{Kind: "tapOnPoint", StartX: 0.25, StartY: 0.75},
	}
	out := ExportMaestro("dev.devicelab.testhive", steps)

	for _, want := range []string{
		"name: testhive",
		"tags:\n  - devicedeck\n  - capture",
		"- launchApp:\n    clearState: true",
		"# devicedeck: tapOn selector=id confidence=high",
		"# devicedeck: tapOn selector=text confidence=medium",
		"# devicedeck: tapOn selector=id+childOf confidence=medium",
		"# devicedeck: tapOn selector=id+index confidence=low",
		"# devicedeck: tapOn selector=point confidence=low FALLBACK(coordinate)",
		`    label: "loginButton"`,
		`    label: "Continue"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("export missing %q:\n%s", want, out)
		}
	}
	if _, err := runner.ValidateFlow([]byte(out)); err != nil {
		t.Fatalf("annotated export invalid: %v\n%s", err, out)
	}
}

// A dotted bundle id names the flow by its last segment; a bare id or an
// empty one still yields a usable name.
func TestFlowName(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"dev.devicelab.testhive", "testhive"},
		{"single", "single"},
		{"", "capture"},
		{"trailing.", "trailing."},
	} {
		if got := flowName(c.in); got != c.want {
			t.Errorf("flowName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// id and text are regexes in Maestro, so a metacharacter in a captured
// identifier is escaped to match the literal element and nothing else.
func TestReEscape(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"star.fill", `star\.fill`},
		{"add-to-cart-1", "add-to-cart-1"},
		{"price($)", `price\(\$\)`},
		{"a[0]", `a\[0\]`},
		{"plain", "plain"},
	} {
		if got := reEscape(c.in); got != c.want {
			t.Errorf("reEscape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	out := ExportMaestro("com.example", []Step{{Kind: "tapOn", ID: "star.fill"}})
	if !strings.Contains(out, `id: "star\\.fill"`) {
		t.Errorf("id not regex-escaped in export:\n%s", out)
	}
	if _, err := runner.ValidateFlow([]byte(out)); err != nil {
		t.Fatalf("escaped-id flow invalid: %v\n%s", err, out)
	}
}

// The dual-platform check is the emitter's guardrail: everything capture
// emits must be replayable on both iOS and Android simulators, since a
// captured flow has to run unchanged on either. A field one platform's
// driver ignores (iOS has no css) makes a flow that mismatches on replay.
func TestExportUsesNoPlatformSpecificFields(t *testing.T) {
	steps := []Step{
		{Kind: "tapOn", ID: "loginButton"},
		{Kind: "tapOn", Text: "Continue", ChildOfID: "form"},
		{Kind: "inputText", Input: "devicelab"},
		{Kind: "assertVisible", Text: "Hello"},
		{Kind: "tapOnPoint", StartX: 0.5, StartY: 0.5},
	}
	out := []byte(ExportMaestro("dev.devicelab.testhive", steps))
	for _, platform := range []string{"ios", "android"} {
		bad, err := runner.UnsupportedFields(out, platform)
		if err != nil {
			t.Fatalf("%s: %v", platform, err)
		}
		if len(bad) != 0 {
			t.Errorf("%s: emitted unsupported fields %v\n%s", platform, bad, out)
		}
	}
}

// A flow that reaches for a field iOS does not support must be caught, so
// the guardrail is proven to reject and not merely to pass — css is
// Android/web only.
func TestUnsupportedFieldsRejectsCSSOnIOS(t *testing.T) {
	yaml := "appId: x\n---\n- tapOn:\n    css: \".btn\"\n"
	bad, err := runner.UnsupportedFields([]byte(yaml), "ios")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	found := false
	for _, f := range bad {
		if f == "css" {
			found = true
		}
	}
	if !found {
		t.Errorf("css not reported unsupported on iOS: %v", bad)
	}
	// The same field is fine on Android, so the check is platform-aware.
	if bad, _ := runner.UnsupportedFields([]byte(yaml), "android"); len(bad) != 0 {
		t.Errorf("css wrongly rejected on Android: %v", bad)
	}
}

// An unknown platform name warns about nothing rather than guessing.
func TestUnsupportedFieldsUnknownPlatform(t *testing.T) {
	yaml := "appId: x\n---\n- tapOn:\n    css: \".btn\"\n"
	if bad, err := runner.UnsupportedFields([]byte(yaml), "toaster"); err != nil || bad != nil {
		t.Errorf("unknown platform: bad=%v err=%v", bad, err)
	}
}

// Garbage in reports a parse error, not a nil field list.
func TestUnsupportedFieldsParseError(t *testing.T) {
	if _, err := runner.UnsupportedFields([]byte("appId: x\n---\n- notACommand: 1\n"), "ios"); err == nil {
		t.Fatal("expected a parse error")
	}
}
