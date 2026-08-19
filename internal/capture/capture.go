// Package capture turns a manual device session into a replayable test:
// it observes the input events flowing to a device, resolves each
// interaction to a durable selector against the UI tree, and exports the
// session as a Maestro YAML flow. This is DeviceDeck's differentiator —
// `tapOn: id` survives layout changes; `tap(x, y)` survives nothing.
package capture

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/input"
	"github.com/devicelab-dev/DeviceDeck/internal/runner"
)

// Interaction classification thresholds.
const (
	// swipeDistance is normalized movement beyond which a touch is a
	// swipe, not a tap (2% of screen ≈ jitter allowance).
	swipeDistance = 0.02
	// longPressAfter turns a stationary hold into longPressOn.
	longPressAfter = 800 * time.Millisecond
	// settleDelay is how long after an interaction the tree is
	// re-snapshotted — animations need to land before the next
	// interaction resolves against the new screen.
	settleDelay = 600 * time.Millisecond
	// staleAfter bounds how old the cached tree may be when a tap
	// resolves. The screen can change without any recorded interaction
	// (launch animations, async loads) — resolving against a snapshot
	// older than this fetches fresh first. Without it, the first tap
	// after an app launch resolves against the splash screen.
	staleAfter = 2 * time.Second
)

// Step is one recorded flow step, exportable to Maestro YAML.
type Step struct {
	Kind string `json:"kind"` // tapOn | longPressOn | inputText | swipe | pressKey | tapOnPoint
	// Selector for tapOn/longPressOn: exactly one of ID/Text set.
	ID   string `json:"id,omitempty"`
	Text string `json:"text,omitempty"`
	// Qualifiers, set only when the selector would otherwise match more
	// than one element — see qualify. ChildOfID names an ancestor;
	// Index is a position among matches and is the weaker of the two,
	// so at most one is ever set.
	ChildOfID string `json:"childOfId,omitempty"`
	Index     int    `json:"index,omitempty"`
	// Input for inputText / key name for pressKey.
	Input string `json:"input,omitempty"`
	// Normalized coordinates for swipe / tapOnPoint fallback.
	StartX float64 `json:"startX,omitempty"`
	StartY float64 `json:"startY,omitempty"`
	EndX   float64 `json:"endX,omitempty"`
	EndY   float64 `json:"endY,omitempty"`
	// Bounds of the resolved element, normalized 0-1 — the console draws
	// this over the video so the user sees what each tap resolved to.
	Bounds *NormRect `json:"bounds,omitempty"`
	// Screen fingerprints either side of the action: Pre is the screen it
	// was performed against, Post the screen once it settled. They never
	// reach the Maestro flow — a captured flow has to stay byte-identical
	// to replay unchanged on real devices — and travel in a sidecar
	// instead, where replay can use them to say which step diverged
	// rather than only that a tap failed.
	Pre  string `json:"pre,omitempty"`
	Post string `json:"post,omitempty"`
}

// NoEffect reports that the action left the screen exactly as it found
// it. Such a step is the likeliest to flake on replay — the tap may have
// landed on nothing — so the recorder surfaces it rather than emitting a
// step that quietly does nothing. Unknown until the post-action snapshot
// lands, so a step still settling reports false.
func (s Step) NoEffect() bool { return s.Post != "" && s.Post == s.Pre }

// NormRect is an element frame normalized to the app's bounds.
type NormRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// SnapshotFunc fetches the current UI tree for the recorder's device.
type SnapshotFunc func(ctx context.Context) ([]runner.Node, error)

// Recorder assembles one device's input events into steps. All methods
// are safe for concurrent use — events arrive from HTTP handlers and the
// input WebSocket goroutine.
type Recorder struct {
	appID    string
	snapshot SnapshotFunc
	now      func() time.Time
	// settle sleeps before a tree refresh; injected for tests.
	settle func()

	mu      sync.Mutex
	steps   []Step
	tree    []runner.Node
	treeAt  time.Time
	pending *pendingTouch
	text    []rune
	refresh int // refresh generation; stale async refreshes are dropped
}

type pendingTouch struct {
	startX, startY float64
	lastX, lastY   float64
	startedAt      time.Time
}

// NewRecorder starts recording for appID, taking the initial tree
// snapshot synchronously so the first tap has something to resolve
// against.
func NewRecorder(ctx context.Context, appID string, snapshot SnapshotFunc) (*Recorder, error) {
	tree, err := snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("initial tree snapshot: %w", err)
	}
	r := &Recorder{
		appID:    appID,
		snapshot: snapshot,
		now:      time.Now,
		settle:   func() { time.Sleep(settleDelay) },
		tree:     tree,
	}
	r.treeAt = r.now()
	return r, nil
}

// OnEvent feeds one decoded input event into the recording.
func (r *Recorder) OnEvent(ev input.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch ev.Kind {
	case input.EventTouch:
		r.onTouch(ev)
	case input.EventKey:
		r.onKey(ev)
	case input.EventGesture:
		r.flushTextLocked()
		if ev.Gesture == input.GestureSwipeToHome {
			r.appendStep(Step{Kind: "pressKey", Input: "Home"})
		}
		// Other system gestures have no durable Maestro equivalent; they
		// are intentionally not recorded rather than recorded as lies.
	case input.EventLegacyButton:
		r.flushTextLocked()
		if ev.Code == 0 {
			r.appendStep(Step{Kind: "pressKey", Input: "Home"})
		} else if ev.Code == 1 {
			r.appendStep(Step{Kind: "pressKey", Input: "Lock"})
		}
	default:
		// Two-finger and raw HID buttons are not yet exportable.
	}
}

// appendStep records a step against the screen it was performed on. Every
// step goes through here so no path can forget the fingerprint.
func (r *Recorder) appendStep(s Step) {
	hash := runner.InteractionHash(r.tree)
	// Close out the previous step if nothing has yet. Only touches
	// schedule a settle refresh, so typing and key presses would
	// otherwise never learn what they produced — and the screen we are
	// about to act on is exactly the screen the previous step left.
	if n := len(r.steps); n > 0 && r.steps[n-1].Post == "" {
		r.steps[n-1].Post = hash
	}
	s.Pre = hash
	r.steps = append(r.steps, s)
}

// Assert records an assertion that the element at a normalized point is
// visible, without touching the device.
//
// A captured flow made only of actions proves that its steps executed,
// not that the journey worked — the 50x replay gate can pass while the
// app quietly fails, because tapping a button that does nothing is
// indistinguishable from tapping one that does. An assertion is what
// turns the recording into a test. It resolves through the same
// hit-test as a tap, so it inherits the same durable selector.
//
// Returns false when the point resolves to nothing selectable: a
// coordinate assertion would be a lie, since it asserts only that the
// screen has pixels there.
func (r *Recorder) Assert(x, y float64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushTextLocked()
	step := r.resolveTap("assertVisible", x, y)
	if step.Kind != "assertVisible" {
		return false
	}
	r.appendStep(step)
	return true
}

// Steps returns a copy of what has been recorded so far.
func (r *Recorder) Steps() []Step {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushTextLocked()
	return append([]Step(nil), r.steps...)
}

// Finish flushes pending text and returns the completed step list.
func (r *Recorder) Finish() []Step {
	return r.Steps()
}

// AppID returns the bundle id the recording targets.
func (r *Recorder) AppID() string { return r.appID }

// ---------- touch assembly ----------

func (r *Recorder) onTouch(ev input.Event) {
	switch ev.Phase {
	case input.TouchDown:
		r.flushTextLocked()
		r.pending = &pendingTouch{
			startX: ev.X, startY: ev.Y,
			lastX: ev.X, lastY: ev.Y,
			startedAt: r.now(),
		}
	case input.TouchMove:
		if r.pending != nil {
			r.pending.lastX, r.pending.lastY = ev.X, ev.Y
		}
	case input.TouchUp:
		if r.pending == nil {
			return
		}
		p := r.pending
		r.pending = nil
		r.finalizeTouch(p)
		r.scheduleRefreshLocked()
	}
}

func (r *Recorder) finalizeTouch(p *pendingTouch) {
	distance := math.Hypot(p.lastX-p.startX, p.lastY-p.startY)
	if distance >= swipeDistance {
		r.appendStep(Step{
			Kind:   "swipe",
			StartX: p.startX, StartY: p.startY,
			EndX: p.lastX, EndY: p.lastY,
		})
		return
	}
	kind := "tapOn"
	if r.now().Sub(p.startedAt) >= longPressAfter {
		kind = "longPressOn"
	}
	r.appendStep(r.resolveTap(kind, p.startX, p.startY))
}

// resolveTap turns a normalized point into a selector-based step, falling
// back to a percent point tap when the tree offers nothing durable. A
// cached tree older than staleAfter is replaced with a fresh synchronous
// snapshot first — the screen may have changed without any recorded
// interaction (launch animation, async content).
func (r *Recorder) resolveTap(kind string, x, y float64) Step {
	if r.now().Sub(r.treeAt) > staleAfter {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if tree, err := r.snapshot(ctx); err == nil {
			r.tree = tree
			r.treeAt = r.now()
		}
		cancel()
	}
	if node := hitTest(r.tree, x, y); node != nil {
		bounds := normalizedBounds(r.tree, node)
		if node.Identifier != "" {
			return qualify(r.tree, node, Step{Kind: kind, ID: node.Identifier, Bounds: bounds})
		}
		if node.Label != "" {
			return qualify(r.tree, node, Step{Kind: kind, Text: node.Label, Bounds: bounds})
		}
	}
	return Step{Kind: "tapOnPoint", StartX: x, StartY: y}
}

// normalizedBounds converts a node's frame from points into the 0-1 space
// the console overlays on the video.
func normalizedBounds(tree []runner.Node, node *runner.Node) *NormRect {
	app := tree[0].Frame
	if app.Width <= 0 || app.Height <= 0 {
		return nil
	}
	return &NormRect{
		X:      node.Frame.X / app.Width,
		Y:      node.Frame.Y / app.Height,
		Width:  node.Frame.Width / app.Width,
		Height: node.Frame.Height / app.Height,
	}
}

// hitTest finds the most specific durable node containing the point:
// smallest-area node with an identifier wins, then smallest with a label.
// The application node (depth 0) never matches — tapping "the app" is not
// a selector.
func hitTest(tree []runner.Node, x, y float64) *runner.Node {
	if len(tree) == 0 {
		return nil
	}
	app := tree[0].Frame
	if app.Width <= 0 || app.Height <= 0 {
		return nil
	}
	px, py := x*app.Width, y*app.Height
	// A node covering (nearly) the whole screen is a backdrop or splash
	// image, not a tap target — resolving to it produces a selector that
	// matches the wrong thing on every other screen.
	maxArea := app.Width * app.Height * 0.9
	var best *runner.Node
	bestScore := math.MaxFloat64
	for i := range tree {
		n := &tree[i]
		if n.Depth == 0 || n.Frame.Width <= 0 || n.Frame.Height <= 0 {
			continue
		}
		if n.Frame.Width*n.Frame.Height >= maxArea {
			continue
		}
		if n.Identifier == "" && n.Label == "" {
			continue
		}
		f := n.Frame
		if px < f.X || px > f.X+f.Width || py < f.Y || py > f.Y+f.Height {
			continue
		}
		score := f.Width * f.Height
		// An identifier beats a label at any size: halving the score space
		// keeps id-bearing containers preferred over labeled leaves only
		// when the id node is genuinely smaller than twice the leaf.
		if n.Identifier == "" {
			score *= 2
		}
		if score < bestScore {
			best = n
			bestScore = score
		}
	}
	return best
}

// scheduleRefreshLocked re-snapshots the tree after the screen settles so
// the NEXT interaction resolves against what the user now sees. Stale
// refreshes (superseded by a newer interaction) are dropped.
func (r *Recorder) scheduleRefreshLocked() {
	r.refresh++
	generation := r.refresh
	go func() {
		r.settle()
		tree, err := r.snapshot(context.Background())
		if err != nil {
			return // keep the previous tree; better stale than empty
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.refresh == generation {
			r.tree = tree
			r.treeAt = r.now()
			// The screen has settled: close out the step that caused it.
			if n := len(r.steps); n > 0 && r.steps[n-1].Post == "" {
				r.steps[n-1].Post = runner.InteractionHash(tree)
			}
		}
	}()
}

// ---------- keyboard assembly ----------

func (r *Recorder) onKey(ev input.Event) {
	if ev.Usage == 0x28 { // Enter
		r.flushTextLocked()
		r.appendStep(Step{Kind: "pressKey", Input: "Enter"})
		return
	}
	if ev.Usage == 0x2A { // Backspace: retract the last buffered rune
		if len(r.text) > 0 {
			r.text = r.text[:len(r.text)-1]
		}
		return
	}
	if ch, ok := keyRune(ev.Usage, ev.Mod&0x22 != 0); ok {
		r.text = append(r.text, ch)
	}
}

func (r *Recorder) flushTextLocked() {
	if len(r.text) == 0 {
		return
	}
	r.appendStep(Step{Kind: "inputText", Input: string(r.text)})
	r.text = nil
}

// keyRune maps a USB HID keyboard usage (page 0x07) to the character it
// types, honoring shift. Mirrors the console's KEY_USAGE map.
func keyRune(usage uint32, shift bool) (rune, bool) {
	switch {
	case usage >= 0x04 && usage <= 0x1D: // a–z
		ch := rune('a' + usage - 0x04)
		if shift {
			ch = ch - 'a' + 'A'
		}
		return ch, true
	case usage >= 0x1E && usage <= 0x27: // 1234567890
		plain := []rune("1234567890")
		shifted := []rune("!@#$%^&*()")
		if shift {
			return shifted[usage-0x1E], true
		}
		return plain[usage-0x1E], true
	}
	pairs := map[uint32][2]rune{
		0x2C: {' ', ' '},
		0x2D: {'-', '_'},
		0x2E: {'=', '+'},
		0x2F: {'[', '{'},
		0x30: {']', '}'},
		0x31: {'\\', '|'},
		0x33: {';', ':'},
		0x34: {'\'', '"'},
		0x35: {'`', '~'},
		0x36: {',', '<'},
		0x37: {'.', '>'},
		0x38: {'/', '?'},
	}
	if pair, ok := pairs[usage]; ok {
		if shift {
			return pair[1], true
		}
		return pair[0], true
	}
	return 0, false
}
