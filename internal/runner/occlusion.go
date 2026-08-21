package runner

// screenEpsilon absorbs the sub-point rounding in reported frames, so a
// view one hundredth of a point short of the screen still counts as
// covering it.
const screenEpsilon = 1.0

// OnScreen drops the app views that a later view has replaced.
//
// iOS does not tear down a screen when the app navigates away from it:
// the old view stays in the hierarchy and XCUITest keeps reporting it. A
// mirror faithful to that tree therefore offers a login form — with the
// credentials still in its fields — on every screen that follows,
// indistinguishable from what is actually in front of the user. A driver
// asked to sign in when already signed in finds the dead Sign In button,
// clicks it, and nothing happens; a captured flow records a selector that
// resolves to a screen nobody can see. Neither failure announces itself.
//
// Nothing in the node tells the two apart: hittable reads false for the
// live screen and the dead one alike, and both claim the full screen.
// Sibling order is the only signal — the view the app moved to is
// reported last — so among the full-screen siblings only the last
// survives.
//
// Windows are exempt, and that exemption is the whole subtlety. The
// keyboard lives inside the app's Window, which is itself full-screen and
// reported first; culling by geometry alone would take a keyboard that is
// plainly on screen along with the dead views. Excluding Windows also
// makes the rule fail safe: an app whose screens really are Windows keeps
// every node it had before.
func OnScreen(nodes []Node) []Node {
	if len(nodes) == 0 {
		return nodes
	}
	replaced := supersededViews(nodes)
	if len(replaced) == 0 {
		return nodes
	}
	doomed := withDescendants(nodes, replaced)
	kept := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		if !doomed[n.Index] {
			kept = append(kept, n)
		}
	}
	return kept
}

// supersededViews returns the indices of every full-screen view the app
// has navigated away from — all but the last of them.
func supersededViews(nodes []Node) map[int]bool {
	root := nodes[0]
	var fullScreen []int
	for _, n := range nodes[1:] {
		if n.Type == "Window" || n.ParentIndex == nil || *n.ParentIndex != root.Index {
			continue
		}
		if covers(n.Frame, root.Frame) {
			fullScreen = append(fullScreen, n.Index)
		}
	}
	if len(fullScreen) < 2 {
		return nil
	}
	out := make(map[int]bool, len(fullScreen)-1)
	for _, idx := range fullScreen[:len(fullScreen)-1] {
		out[idx] = true
	}
	return out
}

// covers reports whether inner spans the whole of outer.
func covers(inner, outer Rect) bool {
	return inner.X <= outer.X+screenEpsilon &&
		inner.Y <= outer.Y+screenEpsilon &&
		inner.X+inner.Width >= outer.X+outer.Width-screenEpsilon &&
		inner.Y+inner.Height >= outer.Y+outer.Height-screenEpsilon
}

// withDescendants grows a set of node indices to include everything
// beneath them. Children may appear before their parents in the slice, so
// the walk repeats until it stops growing rather than assuming an order.
func withDescendants(nodes []Node, seed map[int]bool) map[int]bool {
	out := make(map[int]bool, len(seed))
	for idx := range seed {
		out[idx] = true
	}
	for grew := true; grew; {
		grew = false
		for _, n := range nodes {
			if n.ParentIndex == nil || out[n.Index] || !out[*n.ParentIndex] {
				continue
			}
			out[n.Index] = true
			grew = true
		}
	}
	return out
}
