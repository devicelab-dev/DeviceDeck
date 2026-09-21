package runner

import (
	"math"
	"testing"
)

func node(i int, typ string, x, y float64) Node {
	return Node{
		Index:   i,
		Type:    typ,
		Enabled: true,
		Frame:   Rect{X: x, Y: y, Width: 100, Height: 40},
		Depth:   1,
	}
}

func TestScreenHashStableAcrossIdenticalSnapshots(t *testing.T) {
	nodes := []Node{node(0, "Button", 10, 20)}
	first, second := ScreenHash(nodes), ScreenHash(nodes)
	if first != second {
		t.Fatal("hash not deterministic")
	}
}

func TestScreenHashSensitivity(t *testing.T) {
	base := []Node{{
		Index: 0, Type: "Button", Label: "Log in", Identifier: "login",
		Value: "v", Placeholder: "p", Enabled: true, Selected: true,
		Hittable: true, Frame: Rect{X: 0, Y: 0, Width: 100, Height: 40}, Depth: 2,
	}}
	want := ScreenHash(base)

	cases := []struct {
		name    string
		mutate  func(*Node)
		differs bool
	}{
		{"type", func(n *Node) { n.Type = "Image" }, true},
		{"identifier", func(n *Node) { n.Identifier = "other" }, true},
		{"label", func(n *Node) { n.Label = "Log out" }, true},
		{"value", func(n *Node) { n.Value = "w" }, true},
		{"placeholder", func(n *Node) { n.Placeholder = "q" }, true},
		{"enabled", func(n *Node) { n.Enabled = false }, true},
		{"selected", func(n *Node) { n.Selected = false }, true},
		{"hittable", func(n *Node) { n.Hittable = false }, true},
		{"depth", func(n *Node) { n.Depth = 3 }, true},
		{"big move", func(n *Node) { n.Frame.Y = 400 }, true},
		{"resize", func(n *Node) { n.Frame.Width = 300 }, true},
		// Sub-bucket movement is not the screen changing.
		{"sub-pixel drift", func(n *Node) { n.Frame.X = 0.4 }, false},
		// Focus flips when a keyboard opens; the screen has not changed.
		{"focus", func(n *Node) { n.Focused = !n.Focused }, false},
		// ParentIndex renumbers when a wrapper layer is inserted.
		{"parent index", func(n *Node) { p := 7; n.ParentIndex = &p }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mutated := base[0]
			tc.mutate(&mutated)
			got := ScreenHash([]Node{mutated})
			if (got != want) != tc.differs {
				t.Fatalf("differs=%v, want %v (%s vs %s)", got != want, tc.differs, got, want)
			}
		})
	}
}

func TestScreenHashIgnoresStatusBarSubtree(t *testing.T) {
	zero := 0
	one := 1
	withClock := func(clock string) []Node {
		return []Node{
			{Index: 0, Type: "StatusBar", Depth: 1},
			{Index: 1, Type: "Other", Depth: 2, ParentIndex: &zero},
			{Index: 2, Type: "StaticText", Label: clock, Depth: 3, ParentIndex: &one},
			{Index: 3, Type: "Button", Label: "Log in", Depth: 1},
		}
	}
	if ScreenHash(withClock("7:12")) != ScreenHash(withClock("7:13")) {
		t.Fatal("status bar clock changed the hash")
	}
	// The rest of the screen must still register.
	other := withClock("7:12")
	other[3].Label = "Log out"
	if ScreenHash(withClock("7:12")) == ScreenHash(other) {
		t.Fatal("change outside the status bar was ignored")
	}
}

func TestScreenHashOrderMatters(t *testing.T) {
	a := []Node{node(0, "Button", 0, 0), node(1, "Image", 0, 100)}
	b := []Node{node(0, "Image", 0, 100), node(1, "Button", 0, 0)}
	if ScreenHash(a) == ScreenHash(b) {
		t.Fatal("reordering the tree left the hash unchanged")
	}
}

func TestScreenHashEmpty(t *testing.T) {
	if ScreenHash(nil) == "" {
		t.Fatal("empty tree should still hash")
	}
	if ScreenHash(nil) != ScreenHash([]Node{}) {
		t.Fatal("nil and empty disagree")
	}
}

func TestBucketNonFinite(t *testing.T) {
	// iOS reports CGRect.infinite for some off-screen elements; it must
	// not poison the digest with a garbage magnitude.
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := bucket(v); got != 0 {
			t.Fatalf("bucket(%v) = %d, want 0", v, got)
		}
	}
	if bucket(15.9) != 1 || bucket(16) != 2 || bucket(-1) != -1 {
		t.Fatalf("bucket boundaries wrong: %d %d %d", bucket(15.9), bucket(16), bucket(-1))
	}
}

// Moving focus between fields changes nothing on screen, so ScreenHash
// cannot see it — which is right for a settle signal and wrong for
// judging whether a tap did anything.
func TestInteractionHashSeesFocusMoves(t *testing.T) {
	field := func(id string, focused bool) Node {
		return Node{
			Index: 1, Type: "TextField", Identifier: id, Depth: 1, Enabled: true,
			Frame: Rect{X: 0, Y: 0, Width: 100, Height: 40}, Focused: focused,
		}
	}
	app := Node{Index: 0, Type: "Application", Depth: 0, Frame: Rect{Width: 100, Height: 200}}
	before := []Node{app, field("user", true), field("pass", false)}
	after := []Node{app, field("user", false), field("pass", true)}

	if ScreenHash(before) != ScreenHash(after) {
		t.Fatal("precondition: focus must be invisible to ScreenHash")
	}
	if InteractionHash(before) == InteractionHash(after) {
		t.Error("a focus move is an effect and must change InteractionHash")
	}
}

func TestInteractionHashFollowsTheScreen(t *testing.T) {
	app := Node{Index: 0, Type: "Application", Depth: 0, Frame: Rect{Width: 100, Height: 200}}
	one := []Node{app, {
		Index: 1, Type: "Button", Label: "Log in", Depth: 1, Enabled: true,
		Frame: Rect{Width: 50, Height: 20},
	}}
	two := []Node{app, {
		Index: 1, Type: "Button", Label: "Log out", Depth: 1, Enabled: true,
		Frame: Rect{Width: 50, Height: 20},
	}}
	if InteractionHash(one) == InteractionHash(two) {
		t.Error("a screen change must still change InteractionHash")
	}
	a, b := InteractionHash(one), InteractionHash(one)
	if a != b {
		t.Error("InteractionHash must be deterministic")
	}
	// A focused element that only moved is the same focus.
	moved := []Node{app, {
		Index: 1, Type: "TextField", Identifier: "user", Depth: 1, Enabled: true,
		Frame: Rect{Y: 400, Width: 50, Height: 20}, Focused: true,
	}}
	same := []Node{app, {
		Index: 1, Type: "TextField", Identifier: "user", Depth: 1, Enabled: true,
		Frame: Rect{Y: 400, Width: 50, Height: 20}, Focused: true,
	}}
	if InteractionHash(moved) != InteractionHash(same) {
		t.Error("identical trees must hash identically")
	}
}
