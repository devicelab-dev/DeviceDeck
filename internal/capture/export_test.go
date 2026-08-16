package capture

import (
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
