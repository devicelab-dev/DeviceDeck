package capture

import (
	"strings"
	"testing"

	"github.com/devicelab-dev/DeviceDeck/internal/runner"
)

// node is a terse tree-node builder for the lint tests.
func node(typ, id, class string) runner.Node {
	return runner.Node{
		Type: typ, Identifier: id, ClassName: class, Depth: 1,
		Frame: runner.Rect{Width: 100, Height: 40},
	}
}

func frameworks(f []DesertFinding) []string {
	out := make([]string, len(f))
	for i, d := range f {
		out[i] = d.Framework
	}
	return out
}

func TestDesertLint(t *testing.T) {
	tests := []struct {
		name       string
		tree       []runner.Node
		wantFrames []string
		wantSubstr string // must appear in the (single) finding's message
	}{
		{
			name: "fully addressed native screen is not a desert",
			tree: []runner.Node{
				node("Button", "login-button", "android.widget.Button"),
				node("TextField", "username-input", "android.widget.EditText"),
			},
			wantFrames: nil,
		},
		{
			name: "flutter screen with no ids",
			tree: []runner.Node{
				node("Other", "", "io.flutter.embedding.android.FlutterView"),
				node("Button", "", "android.view.View"),
			},
			wantFrames: []string{"Flutter"},
			wantSubstr: "Semantics(identifier",
		},
		{
			name: "compose screen with no ids",
			tree: []runner.Node{
				node("Other", "", "androidx.compose.ui.platform.ComposeView"),
				node("Button", "", "android.view.View"),
			},
			wantFrames: []string{"Jetpack Compose"},
			wantSubstr: "testTagsAsResourceId",
		},
		{
			name: "react native without testID",
			tree: []runner.Node{
				node("Button", "", "com.facebook.react.views.view.ReactViewGroup"),
			},
			wantFrames: []string{"React Native"},
			wantSubstr: "testID",
		},
		{
			name: "webview with idless controls",
			tree: []runner.Node{
				node("WebView", "web", "android.webkit.WebView"),
				node("Button", "", "android.view.View"),
			},
			wantFrames: []string{"WebView"},
			wantSubstr: "native bridge",
		},
		{
			name: "unity single opaque surface",
			tree: []runner.Node{
				node("Other", "", "com.unity3d.player.UnityPlayer"),
			},
			wantFrames: []string{"Unity"},
			wantSubstr: "AccessibilityHierarchy",
		},
		{
			name: "unaddressed with no framework marker is generic",
			tree: []runner.Node{
				node("Button", "", "android.widget.Button"),
			},
			wantFrames: []string{""},
			wantSubstr: "add a test identifier",
		},
		{
			name: "a screen with no actionable elements is not a desert",
			tree: []runner.Node{
				node("StaticText", "", "android.widget.TextView"),
			},
			wantFrames: nil,
		},
		{
			name: "unity plus an idless control reports both",
			tree: []runner.Node{
				node("Other", "", "com.unity3d.player.UnityPlayer"),
				node("Button", "", "android.view.View"),
			},
			wantFrames: []string{"Unity", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DesertLint(tt.tree)
			if strings.Join(frameworks(got), "|") != strings.Join(tt.wantFrames, "|") {
				t.Fatalf("frameworks = %v, want %v", frameworks(got), tt.wantFrames)
			}
			if tt.wantSubstr != "" {
				if len(got) == 0 || !strings.Contains(got[len(got)-1].Message, tt.wantSubstr) {
					t.Errorf("message missing %q: %+v", tt.wantSubstr, got)
				}
				// Unity's finding is unconditional and carries no count; the
				// addressability-based ones all report how much they cover.
				last := got[len(got)-1]
				if last.Framework != "Unity" &&
					!strings.Contains(last.Message, "actionable elements have no identifier") {
					t.Errorf("message missing the count suffix: %+v", got)
				}
			}
		})
	}
}

// The findings ride into the flow as comments after the header, which
// Maestro ignores, so the flow still validates and replays unchanged.
func TestExportWithLintComments(t *testing.T) {
	steps := []Step{{Kind: "tapOnPoint", StartX: 0.5, StartY: 0.5}}
	findings := []DesertFinding{
		{"Flutter", "add Semantics"},
		{"", "generic reminder"},
	}
	out := ExportMaestroWithLint("com.example", steps, findings)
	if !strings.Contains(out, "# desert (Flutter): add Semantics") {
		t.Errorf("framework comment missing:\n%s", out)
	}
	if !strings.Contains(out, "# desert: generic reminder") {
		t.Errorf("generic comment missing:\n%s", out)
	}
	if _, err := runner.ValidateFlow([]byte(out)); err != nil {
		t.Fatalf("flow with desert comments invalid: %v\n%s", err, out)
	}
	// No findings → no desert comments, identical to the plain export.
	if got := ExportMaestroWithLint("com.example", steps, nil); strings.Contains(got, "# desert") {
		t.Errorf("empty findings should add no comments:\n%s", got)
	}
}
