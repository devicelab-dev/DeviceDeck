package capture

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/runner"
)

func TestNoEffect(t *testing.T) {
	cases := []struct {
		name string
		step Step
		want bool
	}{
		{"changed screen", Step{Pre: "a", Post: "b"}, false},
		{"unchanged screen", Step{Pre: "a", Post: "a"}, true},
		// Still settling: unknown is not the same as ineffective.
		{"no post yet", Step{Pre: "a"}, false},
		{"neither", Step{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.step.NoEffect(); got != tc.want {
				t.Fatalf("NoEffect = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExportGuard(t *testing.T) {
	steps := []Step{
		{Kind: "tapOn", ID: "login", Pre: "aaa", Post: "bbb"},
		{Kind: "inputText", Input: "hi", Pre: "bbb", Post: "bbb"},
	}
	out := ExportGuard("com.example", steps)
	if !strings.HasSuffix(out, "\n") {
		t.Error("guard should end with a newline")
	}
	var g Guard
	if err := json.Unmarshal([]byte(out), &g); err != nil {
		t.Fatalf("guard is not valid JSON: %v", err)
	}
	if g.Version != guardVersion || g.AppID != "com.example" || len(g.Steps) != 2 {
		t.Fatalf("guard header wrong: %+v", g)
	}
	if g.Steps[0].Index != 0 || g.Steps[0].Kind != "tapOn" || g.Steps[0].Pre != "aaa" || g.Steps[0].NoEffect {
		t.Errorf("step 0 wrong: %+v", g.Steps[0])
	}
	// The typing step left the screen identical — that is the signal.
	if !g.Steps[1].NoEffect {
		t.Errorf("step 1 should be flagged NoEffect: %+v", g.Steps[1])
	}
}

func TestExportGuardEmpty(t *testing.T) {
	out := ExportGuard("com.example", nil)
	var g Guard
	if err := json.Unmarshal([]byte(out), &g); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if g.Steps == nil {
		t.Error("steps should encode as [] rather than null")
	}
}

// The guard is a sidecar: nothing it carries may leak into the flow, or a
// captured flow stops replaying unchanged on a stock runner.
func TestGuardFieldsStayOutOfMaestroYAML(t *testing.T) {
	steps := []Step{{Kind: "tapOn", ID: "login", Pre: "aaa", Post: "bbb"}}
	yaml := ExportMaestro("com.example", steps)
	for _, leaked := range []string{"aaa", "bbb", "pre", "post", "noEffect"} {
		if strings.Contains(yaml, leaked) {
			t.Errorf("guard data %q leaked into the flow:\n%s", leaked, yaml)
		}
	}
}

// Only touches schedule a settle refresh, so a step that is followed by
// another action learns its outcome from that action's starting screen.
func TestPreviousStepClosedOutOnNextStep(t *testing.T) {
	r := &Recorder{now: time.Now, tree: []runner.Node{{Index: 0, Type: "Application", Label: "one"}}}
	r.appendStep(Step{Kind: "inputText", Input: "hi"})
	if r.steps[0].Post != "" {
		t.Fatal("first step should not have a post yet")
	}
	r.tree = []runner.Node{{Index: 0, Type: "Application", Label: "two"}}
	r.appendStep(Step{Kind: "tapOn", ID: "next"})
	if r.steps[0].Post == "" || r.steps[0].Post != r.steps[1].Pre {
		t.Fatalf("step 0 post %q should equal step 1 pre %q", r.steps[0].Post, r.steps[1].Pre)
	}
	if r.steps[0].NoEffect() {
		t.Error("screen changed; step should not be flagged NoEffect")
	}
	// A step that changes nothing is the one worth flagging.
	r.appendStep(Step{Kind: "tapOn", ID: "again"})
	if !r.steps[1].NoEffect() {
		t.Error("unchanged screen should flag NoEffect")
	}
}
