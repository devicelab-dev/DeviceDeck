package capture

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/input"
	"github.com/devicelab-dev/DeviceDeck/internal/runner"
)

// testTree: an app (402x874 points) with a login button carrying an
// identifier, a labeled-only banner, and an anonymous wrapper covering
// everything.
func testTree() []runner.Node {
	parent := 0
	return []runner.Node{
		{Index: 0, Type: "Application", Depth: 0,
			Frame: runner.Rect{X: 0, Y: 0, Width: 402, Height: 874}},
		{Index: 1, Type: "Other", Depth: 1, ParentIndex: &parent,
			Frame: runner.Rect{X: 0, Y: 0, Width: 402, Height: 874}},
		{Index: 2, Type: "Button", Identifier: "loginButton", Label: "Log in", Depth: 2,
			Hittable: true, ParentIndex: &parent,
			Frame: runner.Rect{X: 100, Y: 400, Width: 200, Height: 50}},
		{Index: 3, Type: "StaticText", Label: "Welcome back", Depth: 2,
			ParentIndex: &parent,
			Frame:       runner.Rect{X: 50, Y: 100, Width: 300, Height: 40}},
	}
}

// newTestRecorder builds a Recorder with instant settle and a controllable
// clock.
func newTestRecorder(t *testing.T, tree []runner.Node) (*Recorder, *time.Time) {
	t.Helper()
	rec, err := NewRecorder(context.Background(), "com.example.app",
		func(context.Context) ([]runner.Node, error) { return tree, nil })
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Unix(1000, 0)
	rec.now = func() time.Time { return clock }
	rec.settle = func() {}
	return rec, &clock
}

func touch(phase input.TouchPhase, x, y float64) input.Event {
	return input.Event{Kind: input.EventTouch, Phase: phase, X: x, Y: y}
}

func key(mod byte, usage uint32) input.Event {
	return input.Event{Kind: input.EventKey, Mod: mod, Usage: usage}
}

func TestTapResolvesToIdentifier(t *testing.T) {
	rec, _ := newTestRecorder(t, testTree())
	// Button center: (200/402, 425/874).
	rec.OnEvent(touch(input.TouchDown, 0.5, 0.486))
	rec.OnEvent(touch(input.TouchUp, 0.5, 0.486))
	steps := rec.Steps()
	if len(steps) != 1 || steps[0].Kind != "tapOn" || steps[0].ID != "loginButton" {
		t.Fatalf("steps = %+v", steps)
	}
	// Bounds are the button frame (100,400 200x50 in a 402x874 app),
	// normalized — the console draws this over the video.
	b := steps[0].Bounds
	if b == nil {
		t.Fatal("resolved step missing bounds")
	}
	if b.X < 0.24 || b.X > 0.26 || b.Width < 0.49 || b.Width > 0.51 {
		t.Errorf("bounds = %+v", b)
	}
}

func TestTapResolvesToLabelWhenNoIdentifier(t *testing.T) {
	rec, _ := newTestRecorder(t, testTree())
	// Banner center: (200/402, 120/874).
	rec.OnEvent(touch(input.TouchDown, 0.5, 0.137))
	rec.OnEvent(touch(input.TouchUp, 0.5, 0.137))
	steps := rec.Steps()
	if len(steps) != 1 || steps[0].Kind != "tapOn" || steps[0].Text != "Welcome back" {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestTapFallsBackToPoint(t *testing.T) {
	rec, _ := newTestRecorder(t, testTree())
	// Empty area: nothing durable under (0.9, 0.9).
	rec.OnEvent(touch(input.TouchDown, 0.9, 0.9))
	rec.OnEvent(touch(input.TouchUp, 0.9, 0.9))
	steps := rec.Steps()
	if len(steps) != 1 || steps[0].Kind != "tapOnPoint" {
		t.Fatalf("steps = %+v", steps)
	}
	if steps[0].StartX != 0.9 || steps[0].StartY != 0.9 {
		t.Errorf("point = %+v", steps[0])
	}
}

func TestSwipeClassification(t *testing.T) {
	rec, _ := newTestRecorder(t, testTree())
	rec.OnEvent(touch(input.TouchDown, 0.5, 0.8))
	rec.OnEvent(touch(input.TouchMove, 0.5, 0.5))
	rec.OnEvent(touch(input.TouchMove, 0.5, 0.2))
	rec.OnEvent(touch(input.TouchUp, 0.5, 0.2))
	steps := rec.Steps()
	if len(steps) != 1 || steps[0].Kind != "swipe" {
		t.Fatalf("steps = %+v", steps)
	}
	s := steps[0]
	if s.StartY != 0.8 || s.EndY != 0.2 {
		t.Errorf("swipe coords = %+v", s)
	}
}

func TestLongPress(t *testing.T) {
	rec, clock := newTestRecorder(t, testTree())
	rec.OnEvent(touch(input.TouchDown, 0.5, 0.486))
	*clock = clock.Add(time.Second)
	rec.OnEvent(touch(input.TouchUp, 0.5, 0.486))
	steps := rec.Steps()
	if len(steps) != 1 || steps[0].Kind != "longPressOn" || steps[0].ID != "loginButton" {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestTypingAssemblesText(t *testing.T) {
	rec, _ := newTestRecorder(t, testTree())
	// "Hi 5!" → shift-h, i, space, 5, shift-1; then a backspaced typo.
	rec.OnEvent(key(0x02, 0x0B)) // H
	rec.OnEvent(key(0, 0x0C))    // i
	rec.OnEvent(key(0, 0x2C))    // space
	rec.OnEvent(key(0, 0x22))    // 5
	rec.OnEvent(key(0x02, 0x1E)) // !
	rec.OnEvent(key(0, 0x1D))    // z (typo)
	rec.OnEvent(key(0, 0x2A))    // backspace removes it
	rec.OnEvent(key(0, 0x28))    // Enter flushes + pressKey
	steps := rec.Steps()
	if len(steps) != 2 {
		t.Fatalf("steps = %+v", steps)
	}
	if steps[0].Kind != "inputText" || steps[0].Input != "Hi 5!" {
		t.Errorf("text step = %+v", steps[0])
	}
	if steps[1].Kind != "pressKey" || steps[1].Input != "Enter" {
		t.Errorf("enter step = %+v", steps[1])
	}
}

func TestTextFlushedBeforeNextTap(t *testing.T) {
	rec, _ := newTestRecorder(t, testTree())
	rec.OnEvent(key(0, 0x04)) // a
	rec.OnEvent(touch(input.TouchDown, 0.5, 0.486))
	rec.OnEvent(touch(input.TouchUp, 0.5, 0.486))
	steps := rec.Steps()
	if len(steps) != 2 || steps[0].Kind != "inputText" || steps[1].Kind != "tapOn" {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestHomeGestureAndLegacyButtons(t *testing.T) {
	rec, _ := newTestRecorder(t, testTree())
	rec.OnEvent(input.Event{Kind: input.EventGesture, Gesture: input.GestureSwipeToHome})
	rec.OnEvent(input.Event{Kind: input.EventGesture, Gesture: input.GestureAppSwitcher}) // dropped
	rec.OnEvent(input.Event{Kind: input.EventLegacyButton, Code: 1})
	steps := rec.Steps()
	if len(steps) != 2 {
		t.Fatalf("steps = %+v", steps)
	}
	if steps[0].Input != "Home" || steps[1].Input != "Lock" {
		t.Errorf("steps = %+v", steps)
	}
}

func TestTreeRefreshAfterInteraction(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	rec, err := NewRecorder(context.Background(), "com.example.app",
		func(context.Context) ([]runner.Node, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			return testTree(), nil
		})
	if err != nil {
		t.Fatal(err)
	}
	rec.settle = func() {}
	rec.OnEvent(touch(input.TouchDown, 0.5, 0.486))
	rec.OnEvent(touch(input.TouchUp, 0.5, 0.486))

	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		n := calls
		mu.Unlock()
		if n >= 2 { // initial + post-interaction refresh
			return
		}
		select {
		case <-deadline:
			t.Fatalf("refresh never ran (calls=%d)", n)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestRefreshFailureKeepsOldTree(t *testing.T) {
	first := true
	rec, err := NewRecorder(context.Background(), "com.example.app",
		func(context.Context) ([]runner.Node, error) {
			if first {
				first = false
				return testTree(), nil
			}
			return nil, errors.New("runner died")
		})
	if err != nil {
		t.Fatal(err)
	}
	rec.settle = func() {}
	rec.OnEvent(touch(input.TouchDown, 0.5, 0.486))
	rec.OnEvent(touch(input.TouchUp, 0.5, 0.486))
	time.Sleep(50 * time.Millisecond)
	// Old tree still resolves the next tap.
	rec.OnEvent(touch(input.TouchDown, 0.5, 0.486))
	rec.OnEvent(touch(input.TouchUp, 0.5, 0.486))
	steps := rec.Steps()
	if len(steps) != 2 || steps[1].ID != "loginButton" {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestStaleTreeRefreshedAtResolveTime(t *testing.T) {
	// Initial snapshot: splash screen (only a logo). Second snapshot: the
	// real login UI. A tap after staleAfter must resolve against the
	// refreshed tree, not the splash.
	splash := []runner.Node{
		{Depth: 0, Frame: runner.Rect{Width: 402, Height: 874}},
		{Depth: 1, Type: "Image", Label: "RobusTest",
			Frame: runner.Rect{X: 0, Y: 0, Width: 402, Height: 874}},
	}
	calls := 0
	rec, err := NewRecorder(context.Background(), "app",
		func(context.Context) ([]runner.Node, error) {
			calls++
			if calls == 1 {
				return splash, nil
			}
			return testTree(), nil
		})
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Unix(1000, 0)
	rec.now = func() time.Time { return clock }
	rec.treeAt = clock
	rec.settle = func() {}

	clock = clock.Add(5 * time.Second) // cached splash is now stale
	rec.OnEvent(touch(input.TouchDown, 0.5, 0.486))
	rec.OnEvent(touch(input.TouchUp, 0.5, 0.486))
	steps := rec.Steps()
	if len(steps) != 1 || steps[0].ID != "loginButton" {
		t.Fatalf("stale tree not refreshed; steps = %+v", steps)
	}
}

func TestFreshTreeNotRefetchedAtResolveTime(t *testing.T) {
	calls := 0
	rec, err := NewRecorder(context.Background(), "app",
		func(context.Context) ([]runner.Node, error) {
			calls++
			return testTree(), nil
		})
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Unix(1000, 0)
	rec.now = func() time.Time { return clock }
	rec.treeAt = clock
	rec.settle = func() { time.Sleep(time.Hour) } // block async refresh

	clock = clock.Add(time.Second) // within staleAfter
	rec.OnEvent(touch(input.TouchDown, 0.5, 0.486))
	rec.OnEvent(touch(input.TouchUp, 0.5, 0.486))
	if calls != 1 {
		t.Errorf("fresh tree refetched (%d snapshot calls)", calls)
	}
}

func TestNewRecorderSnapshotFailure(t *testing.T) {
	_, err := NewRecorder(context.Background(), "app",
		func(context.Context) ([]runner.Node, error) { return nil, errors.New("no runner") })
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHitTestEdgeCases(t *testing.T) {
	if hitTest(nil, 0.5, 0.5) != nil {
		t.Error("empty tree must miss")
	}
	zeroApp := []runner.Node{{Depth: 0}}
	if hitTest(zeroApp, 0.5, 0.5) != nil {
		t.Error("zero-size app must miss")
	}
	// Identifier on a larger node beats label on a smaller one only within
	// the 2x preference window.
	parent := 0
	tree := []runner.Node{
		{Depth: 0, Frame: runner.Rect{Width: 100, Height: 100}},
		{Depth: 1, Identifier: "wrap", ParentIndex: &parent,
			Frame: runner.Rect{X: 0, Y: 0, Width: 60, Height: 60}},
		{Depth: 2, Label: "leaf", ParentIndex: &parent,
			Frame: runner.Rect{X: 10, Y: 10, Width: 50, Height: 50}},
	}
	got := hitTest(tree, 0.3, 0.3)
	if got == nil || got.Identifier != "wrap" {
		t.Errorf("hitTest = %+v, want wrap (id within 2x of leaf)", got)
	}
}

func TestHitTestIgnoresFullScreenNodes(t *testing.T) {
	// A labeled node covering the whole app (splash image, backdrop) must
	// never become a selector; with nothing else under the point the tap
	// falls back to coordinates.
	tree := []runner.Node{
		{Depth: 0, Frame: runner.Rect{Width: 402, Height: 874}},
		{Depth: 1, Type: "Image", Label: "RobusTest",
			Frame: runner.Rect{X: 0, Y: 0, Width: 402, Height: 874}},
	}
	if got := hitTest(tree, 0.5, 0.5); got != nil {
		t.Errorf("full-screen node resolved: %+v", got)
	}
}

func TestKeyRuneCoverage(t *testing.T) {
	tests := []struct {
		usage uint32
		shift bool
		want  rune
	}{
		{0x04, false, 'a'}, {0x1D, false, 'z'}, {0x04, true, 'A'},
		{0x1E, false, '1'}, {0x27, false, '0'}, {0x27, true, ')'},
		{0x2C, false, ' '}, {0x2D, true, '_'}, {0x34, false, '\''},
		{0x38, true, '?'},
	}
	for _, tt := range tests {
		got, ok := keyRune(tt.usage, tt.shift)
		if !ok || got != tt.want {
			t.Errorf("keyRune(%#x, %v) = %q %v, want %q", tt.usage, tt.shift, got, ok, tt.want)
		}
	}
	if _, ok := keyRune(0x52, false); ok { // arrow key: not a character
		t.Error("arrow key must not map to a rune")
	}
}

// An assertion turns a recording into a test: without one, a replay
// proves the steps executed, not that the journey worked.
func TestAssertRecordsVisibilityWithoutTouching(t *testing.T) {
	tree := []runner.Node{
		{Index: 0, Type: "Application", Frame: runner.Rect{Width: 100, Height: 200}},
		{Index: 1, Type: "Button", Identifier: "products-screen", Depth: 1,
			Frame: runner.Rect{X: 10, Y: 20, Width: 50, Height: 30}},
	}
	r := &Recorder{now: time.Now, tree: tree, treeAt: time.Now(), appID: "com.example"}
	if !r.Assert(0.35, 0.175) {
		t.Fatal("Assert should resolve the button")
	}
	steps := r.Steps()
	if len(steps) != 1 {
		t.Fatalf("steps = %+v", steps)
	}
	if steps[0].Kind != "assertVisible" || steps[0].ID != "products-screen" {
		t.Errorf("step = %+v", steps[0])
	}
	if steps[0].Pre == "" {
		t.Error("assertion should carry the screen it was taken against")
	}
	yaml := ExportMaestro("com.example", steps)
	if !strings.Contains(yaml, "- assertVisible:\n    id: \"products-screen\"") {
		t.Errorf("yaml missing assertion:\n%s", yaml)
	}
	if _, err := runner.ValidateFlow([]byte(yaml)); err != nil {
		t.Fatalf("assertion flow does not parse under the runner: %v\n%s", err, yaml)
	}
}

// Asserting on empty space would be a lie — it would claim only that the
// screen has pixels there — so it is refused rather than degraded to a
// coordinate.
func TestAssertRefusesUnresolvablePoint(t *testing.T) {
	tree := []runner.Node{{Index: 0, Type: "Application", Frame: runner.Rect{Width: 100, Height: 200}}}
	r := &Recorder{now: time.Now, tree: tree, treeAt: time.Now(), appID: "com.example"}
	if r.Assert(0.5, 0.5) {
		t.Fatal("Assert should refuse a point with nothing selectable")
	}
	if len(r.Steps()) != 0 {
		t.Errorf("nothing should have been recorded: %+v", r.Steps())
	}
}

// Text typed but not yet flushed must land before the assertion, or the
// flow asserts on a screen it has not finished producing.
func TestAssertFlushesPendingText(t *testing.T) {
	tree := []runner.Node{
		{Index: 0, Type: "Application", Frame: runner.Rect{Width: 100, Height: 200}},
		{Index: 1, Type: "Button", Identifier: "ok", Depth: 1, Frame: runner.Rect{X: 0, Y: 0, Width: 50, Height: 30}},
	}
	r := &Recorder{now: time.Now, tree: tree, treeAt: time.Now(), appID: "com.example", text: []rune("hi")}
	if !r.Assert(0.25, 0.075) {
		t.Fatal("Assert should resolve")
	}
	steps := r.Steps()
	if len(steps) != 2 || steps[0].Kind != "inputText" || steps[1].Kind != "assertVisible" {
		t.Fatalf("expected inputText then assertVisible, got %+v", steps)
	}
}

// A legacy Home button (code 0) records the same pressKey as the home
// gesture; an unrecognized legacy code is dropped rather than invented.
func TestLegacyHomeButton(t *testing.T) {
	rec, _ := newTestRecorder(t, testTree())
	rec.OnEvent(input.Event{Kind: input.EventLegacyButton, Code: 0})
	rec.OnEvent(input.Event{Kind: input.EventLegacyButton, Code: 2}) // unknown: dropped
	steps := rec.Steps()
	if len(steps) != 1 || steps[0].Kind != "pressKey" || steps[0].Input != "Home" {
		t.Fatalf("steps = %+v", steps)
	}
}

// A touch-up with no matching touch-down (a mirror hiccup, or a gesture
// whose down was swallowed) must be ignored, not turned into a step.
func TestTouchUpWithoutPendingIsIgnored(t *testing.T) {
	rec, _ := newTestRecorder(t, testTree())
	rec.OnEvent(touch(input.TouchUp, 0.5, 0.5))
	if steps := rec.Steps(); len(steps) != 0 {
		t.Errorf("orphan touch-up recorded: %+v", steps)
	}
}

// A zero-sized application frame yields no normalized bounds — dividing by
// it would produce NaN/Inf coordinates for the console overlay.
func TestNormalizedBoundsZeroApp(t *testing.T) {
	tree := []runner.Node{{Depth: 0, Frame: runner.Rect{Width: 0, Height: 0}}}
	if got := normalizedBounds(tree, &tree[0]); got != nil {
		t.Errorf("zero-size app must yield nil bounds, got %+v", got)
	}
}

// A node with neither identifier nor label is not a durable target, so
// hitTest skips it and the tap falls back to coordinates.
func TestHitTestSkipsAnonymousNodes(t *testing.T) {
	parent := 0
	tree := []runner.Node{
		{Depth: 0, Frame: runner.Rect{Width: 100, Height: 100}},
		{Depth: 1, ParentIndex: &parent,
			Frame: runner.Rect{X: 0, Y: 0, Width: 40, Height: 40}}, // no id, no label
	}
	if got := hitTest(tree, 0.1, 0.1); got != nil {
		t.Errorf("anonymous node must not resolve: %+v", got)
	}
}
