package capture

import (
	"gopkg.in/yaml.v3"
	"strings"
	"testing"

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
	if got != label {
		t.Errorf("round trip changed the selector:\n got %q\nwant %q", got, label)
	}
}
