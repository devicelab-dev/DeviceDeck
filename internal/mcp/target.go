package mcp

import (
	"errors"
	"fmt"
	"strings"

	"github.com/devicelab-dev/DeviceDeck/internal/uisem"
)

// errNoTarget is returned when an act tool is given nothing to find.
var errNoTarget = errors.New("name the element: ref (from snapshot), testid, text, or role and name")

// resolveTarget reads the current screen and finds the one element an act
// tool names. It returns that element and the whole tree, which the caller
// needs for the screen size.
func (c *Client) resolveTarget(a deviceArgs) (treeNode, []treeNode, error) {
	nodes, err := c.fetchTree(a.UDID, a.App)
	if err != nil {
		return treeNode{}, nil, err
	}
	i, err := c.targetIndex(nodes, a)
	if err != nil {
		return treeNode{}, nil, err
	}
	return nodes[i], nodes, nil
}

// targetIndex picks the element by the first selector given, in the order an
// agent is told to prefer them: a snapshot ref, a testid, visible text, then
// role and name. A testid keeps its old meaning — the first element carrying
// it. Text and role/name must match exactly one element, so an ambiguous
// name fails with a hint instead of acting on a guess.
func (c *Client) targetIndex(nodes []treeNode, a deviceArgs) (int, error) {
	switch {
	case a.Ref != "":
		return c.refIndex(nodes, a.UDID, a.Ref)
	case a.Testid != "":
		for i, n := range nodes {
			if n.Identifier == a.Testid {
				return i, nil
			}
		}
		return -1, fmt.Errorf("no element with testid %q on screen — snapshot or find_element, or swipe to bring it into view", a.Testid)
	case a.Text != "":
		return uniqueMatch(nodes, fmt.Sprintf("text %q", a.Text), func(n treeNode) bool { return nodeText(n, a.Text) })
	case a.Role != "" || a.Name != "":
		return uniqueMatch(nodes, fmt.Sprintf("role %q name %q", a.Role, a.Name), func(n treeNode) bool { return roleAndName(n, a.Role, a.Name) })
	}
	return -1, errNoTarget
}

// nodeText reports whether a node's visible label, value or placeholder
// contains text, ignoring case.
func nodeText(n treeNode, text string) bool {
	return containsFold(n.Label, text) || containsFold(n.Value, text) || containsFold(n.Placeholder, text)
}

// roleAndName reports whether a node has the role (when given) and a name
// containing name (when given).
func roleAndName(n treeNode, role, name string) bool {
	if role != "" && uisem.Role(n.Type) != role {
		return false
	}
	return name == "" || containsFold(n.Label, name) || containsFold(n.Value, name)
}

// uniqueMatch returns the one node match accepts. An exact-text tie-break is
// not attempted: two matches means the agent should name the element more
// precisely, which the error says.
func uniqueMatch(nodes []treeNode, what string, match func(treeNode) bool) (int, error) {
	found, count := -1, 0
	for i, n := range nodes {
		if match(n) {
			if found < 0 {
				found = i
			}
			count++
		}
	}
	switch count {
	case 0:
		return -1, fmt.Errorf("no element with %s on screen — snapshot or find_element, or swipe to bring it into view", what)
	case 1:
		return found, nil
	}
	return -1, fmt.Errorf("%d elements match %s — use a ref from snapshot or a testid", count, what)
}

// refIndex finds the element a snapshot ref names on the current screen. It
// recomputes each node's identity the way the snapshot minted refs and looks
// the ref up, so a ref still names the same element after the screen changed
// elsewhere, and a ref for an element that has gone fails instead of acting
// on whatever took its place.
func (c *Client) refIndex(nodes []treeNode, udid, ref string) (int, error) {
	reg := c.snaps.registry(udid)
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if !reg.issued(ref) {
		return -1, fmt.Errorf("unknown ref %q — take a snapshot first and use its refs", ref)
	}
	counts := map[string]int{}
	snaps := make([]snapNode, len(nodes))
	for i := range nodes {
		snaps[i] = nodes[i].snap()
	}
	nameFromChildren(snaps)
	for i := range snaps {
		n := snaps[i]
		role, name := uisem.Role(n.Type), snapName(&n)
		if !snapIncluded(&n, role, name, true) {
			continue
		}
		if reg.ids[snapIdentity(&n, role, name, counts)] == ref {
			return i, nil
		}
	}
	return -1, fmt.Errorf("ref %q is not on the current screen — snapshot again", ref)
}

// snap copies the fields a snapshot keys identity on, so refs resolve with
// the same rules that minted them.
func (n treeNode) snap() snapNode {
	s := snapNode{Index: n.Index, ParentIndex: n.ParentIndex, Identifier: n.Identifier, Type: n.Type, Label: n.Label, Value: n.Value, Placeholder: n.Placeholder}
	s.Frame.X, s.Frame.Y, s.Frame.Width, s.Frame.Height = n.Frame.X, n.Frame.Y, n.Frame.Width, n.Frame.Height
	return s
}

// centre is a node's centre as a fraction of the screen, whose size the
// tree's root node carries.
func centre(nodes []treeNode, n treeNode) (float64, float64, error) {
	if len(nodes) == 0 || nodes[0].Frame.Width <= 0 || nodes[0].Frame.Height <= 0 {
		return 0, 0, fmt.Errorf("no screen dimensions in tree")
	}
	w, h := nodes[0].Frame.Width, nodes[0].Frame.Height
	return clamp01((n.Frame.X + n.Frame.Width/2) / w), clamp01((n.Frame.Y + n.Frame.Height/2) / h), nil
}

// targetName is how a result names the element acted on: the selector the
// agent gave, so the reply reads back what it asked for.
func targetName(a deviceArgs) string {
	for _, s := range []string{a.Ref, a.Testid, a.Text} {
		if s != "" {
			return s
		}
	}
	return strings.TrimSpace(a.Role + " " + a.Name)
}

// isDisabled reports whether the device marks the node not enabled. A nil
// Enabled means the field was absent; only an explicit false is disabled.
func isDisabled(n treeNode) bool {
	return n.Enabled != nil && !*n.Enabled
}

// errDisabled explains a present-but-disabled target: usually the screen is
// still settling (a Sign In button that enables once both fields fill),
// which is a retryable state distinct from an element that is not there.
func errDisabled(a deviceArgs) error {
	return fmt.Errorf("element %q is on screen but disabled; the screen may be settling — re-snapshot and retry", targetName(a))
}

// reachable refuses a target an action would miss. A tap goes to the
// element's centre; when that point is off the screen, under the status bar,
// or under the on-screen keyboard, the device receives the touch somewhere
// else — at the screen's edge, in the system's bar, or on a key — and reports
// success. iOS lists its keyboard as nodes; Android reports both through the
// tree's chrome. Saying so lets the agent scroll or
// dismiss the keyboard first. Keys themselves are reachable, of course.
func reachable(nodes []treeNode, n treeNode, a deviceArgs) error {
	if len(nodes) == 0 || nodes[0].Frame.Width <= 0 || nodes[0].Frame.Height <= 0 {
		return nil // no screen size to judge by; centre reports that
	}
	cx, cy := n.Frame.X+n.Frame.Width/2, n.Frame.Y+n.Frame.Height/2
	if cx < 0 || cy < 0 || cx > nodes[0].Frame.Width || cy > nodes[0].Frame.Height {
		return fmt.Errorf("element %q is off screen — swipe to bring it into view, then snapshot again", targetName(a))
	}
	sys := nodes[0].chrome
	if sys.StatusBar > 0 && cy < sys.StatusBar {
		return fmt.Errorf("element %q is under the status bar, where the system takes every tap — the app draws it "+
			"behind the bar without insetting its layout (an edge-to-edge bug in the app); no tap can reach it", targetName(a))
	}
	kb, ok := keyboard(nodes)
	underKeys := ok && inside(kb, cx, cy) && !descends(nodes, n, kb)
	if underKeys || sys.KeyboardTop > 0 && cy > sys.KeyboardTop {
		return fmt.Errorf("element %q is under the on-screen keyboard — press enter or swipe the content up, then snapshot again", targetName(a))
	}
	return nil
}

// keyboard is the on-screen keyboard's node (iOS reports one; Android keeps
// its keyboard in another window, which the tree leaves out).
func keyboard(nodes []treeNode) (treeNode, bool) {
	for _, n := range nodes {
		if n.Type == "Keyboard" && n.Frame.Width > 0 && n.Frame.Height > 0 {
			return n, true
		}
	}
	return treeNode{}, false
}

// inside reports whether a point falls within a node's frame.
func inside(n treeNode, x, y float64) bool {
	return x >= n.Frame.X && x <= n.Frame.X+n.Frame.Width && y >= n.Frame.Y && y <= n.Frame.Y+n.Frame.Height
}

// descends reports whether n is ancestor itself or sits under it.
func descends(nodes []treeNode, n, ancestor treeNode) bool {
	byIndex := make(map[int]treeNode, len(nodes))
	for _, node := range nodes {
		byIndex[node.Index] = node
	}
	for hops := 0; hops <= len(nodes); hops++ {
		if n.Index == ancestor.Index {
			return true
		}
		if n.ParentIndex == nil {
			return false
		}
		n = byIndex[*n.ParentIndex]
	}
	return false // a cycle; treat as unrelated
}
