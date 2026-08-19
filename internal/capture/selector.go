package capture

import "github.com/devicelab-dev/DeviceDeck/internal/runner"

// qualify decides how to address node so the selector matches it and
// nothing else on the screen.
//
// An identifier is the durable way to name an element, but nothing
// guarantees it is unique: a list whose rows share one testID, a tab bar
// of repeated labels, and a flow that says `tapOn: id: row-cta` matches
// several elements. The runner then takes the first, which is often not
// the one the human touched — and no fingerprint catches it, because
// tapping the wrong row still changes the screen. The failure surfaces
// later, somewhere else, as an unexplained divergence.
//
// So the selector escalates only as far as it must:
//
//  1. the identifier alone, when it is unique;
//  2. qualified by the nearest ancestor that has an identifier of its
//     own — the node a person would name ("the row for Alice");
//  3. qualified by position among the matches.
//
// Position is the last rung deliberately: an ordinal breaks the moment
// the list reorders, whereas an ancestor keeps meaning what it meant.
// Label-addressed elements take the same ladder.
func qualify(tree []runner.Node, node *runner.Node, step Step) Step {
	matches := matching(tree, node)
	if len(matches) <= 1 {
		return step
	}
	if anc := identifiedAncestor(tree, node); anc != nil && countUnder(tree, anc, node) == 1 {
		step.ChildOfID = anc.Identifier
		return step
	}
	step.Index = indexOf(matches, node)
	return step
}

// matching returns every node the step's selector would address, in tree
// order — which is the order the runner's index counts in.
func matching(tree []runner.Node, node *runner.Node) []*runner.Node {
	var out []*runner.Node
	for i := range tree {
		n := &tree[i]
		if node.Identifier != "" {
			if n.Identifier == node.Identifier {
				out = append(out, n)
			}
			continue
		}
		if node.Identifier == "" && n.Identifier == "" && n.Label == node.Label && n.Label != "" {
			out = append(out, n)
		}
	}
	return out
}

// identifiedAncestor walks up from node to the nearest ancestor bearing
// an identifier. The immediate parent is usually an anonymous container
// — in React Native it nearly always is — so the useful anchor is the
// first one someone actually named.
func identifiedAncestor(tree []runner.Node, node *runner.Node) *runner.Node {
	byIndex := make(map[int]*runner.Node, len(tree))
	for i := range tree {
		byIndex[tree[i].Index] = &tree[i]
	}
	for cur := node; cur.ParentIndex != nil; {
		parent, ok := byIndex[*cur.ParentIndex]
		if !ok {
			return nil
		}
		if parent.Identifier != "" {
			return parent
		}
		cur = parent
	}
	return nil
}

// countUnder reports how many nodes matching the target's selector sit
// beneath anc — the anchor only disambiguates if exactly one does.
func countUnder(tree []runner.Node, anc, node *runner.Node) int {
	count := 0
	for _, m := range matching(tree, node) {
		if isDescendant(tree, m, anc) {
			count++
		}
	}
	return count
}

func isDescendant(tree []runner.Node, node, anc *runner.Node) bool {
	byIndex := make(map[int]*runner.Node, len(tree))
	for i := range tree {
		byIndex[tree[i].Index] = &tree[i]
	}
	for cur := node; cur.ParentIndex != nil; {
		parent, ok := byIndex[*cur.ParentIndex]
		if !ok {
			return false
		}
		if parent == anc {
			return true
		}
		cur = parent
	}
	return false
}

// indexOf gives node's position among the matches, which is what the
// runner's index: means.
func indexOf(matches []*runner.Node, node *runner.Node) int {
	for i, m := range matches {
		if m.Index == node.Index {
			return i
		}
	}
	return 0
}
