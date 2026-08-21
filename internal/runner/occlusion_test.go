package runner

import "testing"

// fullFrame is the frame every full-screen view in these tests claims.
var fullFrame = Rect{Width: 402, Height: 874}

func at(i int) *int { return &i }

// view builds a child of parent; a nil parent means the root.
func view(i int, typ, label string, frame Rect, parent *int) Node {
	return Node{Index: i, Type: typ, Label: label, Frame: frame, ParentIndex: parent}
}

func labelsOf(nodes []Node) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Label)
	}
	return out
}

func sameLabels(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestOnScreen(t *testing.T) {
	root := view(0, "Application", "app", fullFrame, nil)
	half := Rect{Y: 400, Width: 402, Height: 474}

	tests := []struct {
		name  string
		nodes []Node
		want  []string
	}{
		{
			name: "empty tree is returned untouched",
		},
		{
			name:  "a lone screen is never culled",
			nodes: []Node{root, view(1, "Other", "only", fullFrame, at(0))},
			want:  []string{"app", "only"},
		},
		{
			// The case this exists for: the app navigated, and iOS kept
			// the fullFrame it left behind.
			name: "the screen navigated away from is dropped",
			nodes: []Node{
				root,
				view(1, "Other", "dead", fullFrame, at(0)),
				view(2, "Other", "live", fullFrame, at(0)),
			},
			want: []string{"app", "live"},
		},
		{
			name: "a dropped screen takes its whole subtree",
			nodes: []Node{
				root,
				view(1, "Other", "dead", fullFrame, at(0)),
				view(2, "Button", "ghost", half, at(1)),
				view(3, "Other", "deeper", half, at(2)),
				view(4, "Other", "live", fullFrame, at(0)),
			},
			want: []string{"app", "live"},
		},
		{
			// Children can be reported before their parents, so the walk
			// must not assume slice order.
			name: "descendants listed before their parent are still dropped",
			nodes: []Node{
				root,
				view(3, "Other", "deeper", half, at(2)),
				view(2, "Button", "ghost", half, at(1)),
				view(1, "Other", "dead", fullFrame, at(0)),
				view(4, "Other", "live", fullFrame, at(0)),
			},
			want: []string{"app", "live"},
		},
		{
			// The keyboard lives in the Window. Culling it would take a
			// keyboard that is plainly on screen.
			name: "a Window is never culled, whatever follows it",
			nodes: []Node{
				root,
				view(1, "Window", "window", fullFrame, at(0)),
				view(2, "Other", "dead", fullFrame, at(0)),
				view(3, "Other", "live", fullFrame, at(0)),
			},
			want: []string{"app", "window", "live"},
		},
		{
			name: "a partial overlay leaves the screen beneath it alone",
			nodes: []Node{
				root,
				view(1, "Other", "screen", fullFrame, at(0)),
				view(2, "Other", "sheet", half, at(0)),
			},
			want: []string{"app", "screen", "sheet"},
		},
		{
			name: "sub-point rounding still counts as full screen",
			nodes: []Node{
				root,
				view(1, "Other", "dead", Rect{X: 0.4, Width: 401.7, Height: 873.8}, at(0)),
				view(2, "Other", "live", fullFrame, at(0)),
			},
			want: []string{"app", "live"},
		},
		{
			// Only the root's own children are screens; a full-fullFrame view
			// nested deeper is content.
			name: "a full-screen node deeper in the tree is not a screen",
			nodes: []Node{
				root,
				view(1, "Other", "screen", fullFrame, at(0)),
				view(2, "Other", "inner", fullFrame, at(1)),
			},
			want: []string{"app", "screen", "inner"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := labelsOf(OnScreen(tt.nodes))
			if tt.want == nil {
				if len(got) != 0 {
					t.Fatalf("expected nothing, got %v", got)
				}
				return
			}
			if !sameLabels(got, tt.want) {
				t.Errorf("OnScreen() = %v, want %v", got, tt.want)
			}
		})
	}
}
