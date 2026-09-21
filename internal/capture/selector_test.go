package capture

import (
	"strings"
	"testing"

	"github.com/devicelab-dev/DeviceDeck/internal/runner"
)

func ptr(i int) *int { return &i }

// A list whose rows share one testID is the case that matters: an
// unqualified selector matches every row, and the runner takes the
// first — which is usually not the row the human touched.
func listTree() []runner.Node {
	return []runner.Node{
		{Index: 0, Type: "Application", Depth: 0, Frame: runner.Rect{Width: 100, Height: 200}},
		{
			Index: 1, Type: "Other", Identifier: "row-alice", Depth: 1, ParentIndex: ptr(0),
			Frame: runner.Rect{Y: 0, Width: 100, Height: 50},
		},
		{
			Index: 2, Type: "Button", Identifier: "row-cta", Depth: 2, ParentIndex: ptr(1),
			Frame: runner.Rect{Y: 10, Width: 40, Height: 20},
		},
		{
			Index: 3, Type: "Other", Identifier: "row-bob", Depth: 1, ParentIndex: ptr(0),
			Frame: runner.Rect{Y: 60, Width: 100, Height: 50},
		},
		{
			Index: 4, Type: "Button", Identifier: "row-cta", Depth: 2, ParentIndex: ptr(3),
			Frame: runner.Rect{Y: 70, Width: 40, Height: 20},
		},
	}
}

func TestQualifyLeavesUniqueSelectorsAlone(t *testing.T) {
	tree := listTree()
	step := qualify(tree, &tree[1], Step{Kind: "tapOn", ID: "row-alice"})
	if step.ChildOfID != "" || step.Index != 0 {
		t.Errorf("a unique identifier needs no qualifier: %+v", step)
	}
}

func TestQualifyPrefersAnIdentifiedAncestor(t *testing.T) {
	tree := listTree()
	// Bob's CTA: the identifier alone matches both rows' buttons.
	step := qualify(tree, &tree[4], Step{Kind: "tapOn", ID: "row-cta"})
	if step.ChildOfID != "row-bob" {
		t.Fatalf("expected childOf row-bob, got %+v", step)
	}
	if step.Index != 0 {
		t.Errorf("an ancestor already disambiguates; index should stay unset: %+v", step)
	}
	yaml := ExportMaestro("com.example", []Step{step})
	if !strings.Contains(yaml, "childOf:\n      id: \"row-bob\"") {
		t.Errorf("qualifier missing from flow:\n%s", yaml)
	}
	if _, err := runner.ValidateFlow([]byte(yaml)); err != nil {
		t.Fatalf("qualified flow does not parse under the runner: %v\n%s", err, yaml)
	}
}

// When no ancestor narrows it — siblings under one anonymous parent —
// position is the remaining option, and it is the weaker one because a
// reorder breaks it.
func TestQualifyFallsBackToIndex(t *testing.T) {
	tree := []runner.Node{
		{Index: 0, Type: "Application", Depth: 0, Frame: runner.Rect{Width: 100, Height: 200}},
		{Index: 1, Type: "Other", Depth: 1, ParentIndex: ptr(0), Frame: runner.Rect{Width: 100, Height: 200}},
		{
			Index: 2, Type: "Button", Identifier: "tab", Depth: 2, ParentIndex: ptr(1),
			Frame: runner.Rect{Width: 30, Height: 20},
		},
		{
			Index: 3, Type: "Button", Identifier: "tab", Depth: 2, ParentIndex: ptr(1),
			Frame: runner.Rect{X: 40, Width: 30, Height: 20},
		},
	}
	step := qualify(tree, &tree[3], Step{Kind: "tapOn", ID: "tab"})
	if step.ChildOfID != "" {
		t.Errorf("no identified ancestor exists; childOf should be empty: %+v", step)
	}
	if step.Index != 1 {
		t.Fatalf("expected index 1, got %+v", step)
	}
	yaml := ExportMaestro("com.example", []Step{step})
	if !strings.Contains(yaml, "index: 1") {
		t.Errorf("index missing from flow:\n%s", yaml)
	}
	if _, err := runner.ValidateFlow([]byte(yaml)); err != nil {
		t.Fatalf("indexed flow does not parse: %v\n%s", err, yaml)
	}
}

// Text-addressed elements take the same ladder.
func TestQualifyAppliesToTextSelectors(t *testing.T) {
	tree := []runner.Node{
		{Index: 0, Type: "Application", Depth: 0, Frame: runner.Rect{Width: 100, Height: 200}},
		{
			Index: 1, Type: "Other", Identifier: "card-two", Depth: 1, ParentIndex: ptr(0),
			Frame: runner.Rect{Width: 100, Height: 100},
		},
		{
			Index: 2, Type: "Button", Label: "Buy", Depth: 2, ParentIndex: ptr(1),
			Frame: runner.Rect{Width: 30, Height: 20},
		},
		{
			Index: 3, Type: "Button", Label: "Buy", Depth: 1, ParentIndex: ptr(0),
			Frame: runner.Rect{Y: 120, Width: 30, Height: 20},
		},
	}
	step := qualify(tree, &tree[2], Step{Kind: "tapOn", Text: "Buy"})
	if step.ChildOfID != "card-two" {
		t.Fatalf("text selector should take the ancestor too: %+v", step)
	}
}

// An ancestor that contains every match narrows nothing, so it must not
// be emitted as though it did.
func TestQualifySkipsAncestorThatDoesNotNarrow(t *testing.T) {
	tree := []runner.Node{
		{Index: 0, Type: "Application", Depth: 0, Frame: runner.Rect{Width: 100, Height: 200}},
		{
			Index: 1, Type: "Other", Identifier: "list", Depth: 1, ParentIndex: ptr(0),
			Frame: runner.Rect{Width: 100, Height: 200},
		},
		{
			Index: 2, Type: "Button", Identifier: "cta", Depth: 2, ParentIndex: ptr(1),
			Frame: runner.Rect{Width: 30, Height: 20},
		},
		{
			Index: 3, Type: "Button", Identifier: "cta", Depth: 2, ParentIndex: ptr(1),
			Frame: runner.Rect{Y: 40, Width: 30, Height: 20},
		},
	}
	step := qualify(tree, &tree[3], Step{Kind: "tapOn", ID: "cta"})
	if step.ChildOfID != "" {
		t.Errorf("ancestor holds both matches and narrows nothing: %+v", step)
	}
	if step.Index != 1 {
		t.Errorf("expected position fallback, got %+v", step)
	}
}

// A tree whose parent links point at nodes that are not there — a
// truncated or malformed snapshot — must not send the ancestor walk
// looking for something that never arrives.
func TestSelectorWalksTolerateBrokenParentLinks(t *testing.T) {
	tree := []runner.Node{
		{Index: 0, Type: "Application", Depth: 0, Frame: runner.Rect{Width: 100, Height: 200}},
		{
			Index: 2, Type: "Button", Identifier: "cta", Depth: 2, ParentIndex: ptr(99),
			Frame: runner.Rect{Width: 30, Height: 20},
		},
		{
			Index: 3, Type: "Button", Identifier: "cta", Depth: 2, ParentIndex: ptr(99),
			Frame: runner.Rect{Y: 40, Width: 30, Height: 20},
		},
	}
	if anc := identifiedAncestor(tree, &tree[1]); anc != nil {
		t.Errorf("a dangling parent link has no ancestor: %+v", anc)
	}
	if isDescendant(tree, &tree[1], &tree[0]) {
		t.Error("a dangling link cannot establish descent")
	}
	// It still resolves, falling through to position.
	step := qualify(tree, &tree[2], Step{Kind: "tapOn", ID: "cta"})
	if step.Index != 1 {
		t.Errorf("expected the position fallback, got %+v", step)
	}
}

// indexOf is asked only about nodes drawn from the match set; a node
// that is not among them yields the first position rather than a
// negative index that would render as nonsense in a flow.
func TestIndexOfUnknownNode(t *testing.T) {
	tree := listTree()
	matches := matching(tree, &tree[2])
	if got := indexOf(matches, &tree[1]); got != 0 {
		t.Errorf("unknown node should fall back to 0, got %d", got)
	}
}
