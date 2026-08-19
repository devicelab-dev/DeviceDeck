package runner

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"strconv"
)

// geometryBucketPt quantises frames before hashing. Sub-bucket movement —
// a shadow redrawing, a sub-pixel layout rounding — is not the screen
// changing, and treating it as change means a screen never looks quiet.
// A bucket this size still registers any real animation frame.
const geometryBucketPt = 8

// statusBarType is excluded from the hash wholesale: its clock ticks every
// minute, so including it means every screen stops looking quiet once a
// minute for reasons that have nothing to do with the app.
const statusBarType = "StatusBar"

// InteractionHash fingerprints a screen for the purpose of judging
// whether an action did anything.
//
// It is ScreenHash plus which element holds focus. Focus is deliberately
// absent from ScreenHash, because a keyboard opening flips it without
// the screen having changed and a settle signal must not be held open by
// that. But an action's effect is a different question: moving from one
// field to the next changes nothing on screen and is still exactly what
// the user meant to do, so judging it by ScreenHash alone reports the
// most common interaction in any form as having done nothing.
func InteractionHash(nodes []Node) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(ScreenHash(nodes)))
	_, _ = h.Write([]byte{0})
	for _, n := range nodes {
		if !n.Focused {
			continue
		}
		// Identity, not position: a focused field that moves because the
		// keyboard opened is still the same focus.
		for _, s := range []string{n.Type, n.Identifier, n.Label, n.Placeholder} {
			_, _ = h.Write([]byte(s))
			_, _ = h.Write([]byte{0})
		}
	}
	return strconv.FormatUint(h.Sum64(), 16)
}

// ScreenHash fingerprints what is on screen and what state it is in, so a
// caller can tell "the screen changed" from "the snapshot differs".
//
// Two consumers, both needing the same distinction: the device page
// publishes settledness from it (a screen whose hash holds still has
// finished animating), and Flow Capture records it per step so replay can
// tell whether an action actually landed.
//
// Deliberately excluded: exact geometry (bucketed — see above), the status
// bar subtree, and focus. Focus flips when a keyboard opens without the
// screen having changed, and it is state the caller can assert directly.
func ScreenHash(nodes []Node) string {
	h := fnv.New64a()
	skip := statusBarSubtree(nodes)
	for _, n := range nodes {
		if skip[n.Index] {
			continue
		}
		writeNode(h, n)
	}
	return strconv.FormatUint(h.Sum64(), 16)
}

// writeNode folds one node's identity and state into the digest. Field
// order and the separator matter only in that they must never change
// without the hash being understood to change.
func writeNode(h interface{ Write([]byte) (int, error) }, n Node) {
	for _, s := range []string{n.Type, n.Identifier, n.Label, n.Value, n.Placeholder} {
		_, _ = h.Write([]byte(s))
		_, _ = h.Write([]byte{0})
	}
	var flags byte
	if n.Enabled {
		flags |= 1
	}
	if n.Selected {
		flags |= 2
	}
	if n.Hittable {
		flags |= 4
	}
	_, _ = h.Write([]byte{flags})
	for _, v := range []float64{n.Frame.X, n.Frame.Y, n.Frame.Width, n.Frame.Height} {
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(bucket(v)))
		_, _ = h.Write(buf[:])
	}
	// Depth, not ParentIndex: an inserted wrapper layer renumbers every
	// later index without the screen having changed, whereas nesting
	// depth tracks the shape a viewer would actually see.
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(n.Depth))
	_, _ = h.Write(buf[:])
}

// bucket quantises a coordinate, floor-style so a value sitting exactly on
// a boundary lands in one bucket rather than oscillating between two.
func bucket(v float64) int64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return int64(math.Floor(v / geometryBucketPt))
}

// statusBarSubtree marks the status bar and everything under it. The
// subtree is found by walking parents rather than by depth, since the bar
// sits at different depths on the two platforms.
func statusBarSubtree(nodes []Node) map[int]bool {
	skip := make(map[int]bool)
	for _, n := range nodes {
		if n.Type == statusBarType {
			skip[n.Index] = true
			continue
		}
		if n.ParentIndex != nil && skip[*n.ParentIndex] {
			skip[n.Index] = true
		}
	}
	return skip
}
