package runner

import (
	"context"
	"errors"
	"time"
)

// Snapshot is a UI tree together with the app-lifecycle state that came
// with it — the target's XCUIApplication.state on iOS, empty where the
// platform does not report one. The state travels with the tree because
// it is the same dump: no signal in the nodes reports foreground/
// background reliably (per-node hittable oscillates while a screen
// animates into the background), so a caller reads AppState, not the
// tree, to know the app is frontmost.
type Snapshot struct {
	Nodes    []Node
	AppState string
}

// Snapshotter is the one tree operation settling needs. Narrow on purpose:
// it lets the settle logic be exercised against a scripted sequence of
// trees, with no engine and no device.
type Snapshotter func(ctx context.Context) (Snapshot, error)

// SettleOptions bounds a settle wait. Zero values take the defaults below.
type SettleOptions struct {
	// Interval between samples. The device-side dump is the real cost,
	// so this is a floor, not a cadence.
	Interval time.Duration
	// Quiet is how many consecutive identical ScreenHash samples count as
	// the screen having stopped moving.
	Quiet int
	// Cap bounds the whole wait. A tap on static text changes nothing,
	// ever, and a caller must get an answer anyway.
	Cap time.Duration
	// Idle, when set, is asked once a screen looks quiet, before that is
	// accepted: a screen can stop moving and still be mid-transition — iOS
	// keeps a pushed-away screen in the tree ~430ms after the animation
	// ends — and only the app knows it has not finished. Its errors are
	// ignored: the tree remains the judge.
	Idle func(ctx context.Context) error
}

// Defaults for SettleOptions. The cap is measured, not picked: every
// TestHive transition settles well inside it, and a genuine no-op action
// costs exactly this long, so lower is better right up to the point
// where a slow screen starts being reported as unchanged.
const (
	settleInterval = 100 * time.Millisecond
	settleQuiet    = 3
	settleCap      = 2 * time.Second
)

func (o SettleOptions) withDefaults() SettleOptions {
	if o.Interval <= 0 {
		o.Interval = settleInterval
	}
	if o.Quiet <= 0 {
		o.Quiet = settleQuiet
	}
	if o.Cap <= 0 {
		o.Cap = settleCap
	}
	return o
}

// Settle samples the tree until the screen has changed from `after` and
// then stopped moving, or the cap passes. It returns the last tree seen
// either way.
//
// This is the barrier an automation client waits on after an action.
// "Changed" is judged by InteractionHash, because the most common
// action in any form — moving focus to the next field — leaves
// ScreenHash identical by design and would otherwise sit at the cap.
// "Quiet" is judged by ScreenHash, because focus flipping while a
// keyboard opens must not hold the wait open. A tree returned at the cap
// with its InteractionHash still equal to `after` is the caller's signal
// that nothing happened, which is how a swallowed tap becomes visible
// after the fact: it is the one outcome the device reports identically
// to a no-op.
//
// An empty `after` asks only for quiet, which is what a caller wants on
// first contact, before it has any hash to compare against.
func Settle(ctx context.Context, snap Snapshotter, after string, opts SettleOptions) (Snapshot, error) {
	opts = opts.withDefaults()
	deadline := time.Now().Add(opts.Cap)
	var last Snapshot
	lastScreen, quiet, idled := "", 0, false
	for {
		got, err := snap(ctx)
		if err != nil {
			return Snapshot{}, err
		}
		last = got
		quiet, lastScreen = countQuiet(got.Nodes, lastScreen, quiet)
		idled = idled && quiet > 1 // a new screen must be idled afresh
		changed := after == "" || InteractionHash(got.Nodes) != after
		if changed && quiet >= opts.Quiet {
			if idled || opts.Idle == nil {
				return last, nil
			}
			_ = opts.Idle(ctx)
			idled = true
			continue // re-read at once: did finishing change the screen?
		}
		if !sleepUntil(ctx, opts.Interval, deadline) {
			return last, nil
		}
	}
}

// countQuiet advances the run of identical screens, resetting on change.
func countQuiet(nodes []Node, lastScreen string, quiet int) (int, string) {
	screen := ScreenHash(nodes)
	if screen == lastScreen {
		return quiet + 1, screen
	}
	return 1, screen
}

// sleepUntil waits one interval and reports whether there is still time
// for another sample. A cancelled context ends the wait at once.
func sleepUntil(ctx context.Context, interval time.Duration, deadline time.Time) bool {
	if time.Now().Add(interval).After(deadline) {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(interval):
		return true
	}
}

// LaunchOptions bounds a launch wait. Zero values take the defaults.
type LaunchOptions struct {
	// Window is how long the app keeps swallowing taps after its screen
	// is already in the tree.
	Window time.Duration
	// Appear bounds how long the app is given to put anything in the
	// tree at all. Beyond this the launch itself is the problem.
	Appear time.Duration
	// Interval between samples while waiting for the tree to appear.
	Interval time.Duration
}

// Defaults for LaunchOptions. The window is measured, not picked: on
// TestHive under iOS 26.2, taps 150ms and 330ms after the login field
// appeared were swallowed, and taps from 570ms on landed. The screen
// hash is identical throughout, so the window cannot be detected, only
// waited out.
const (
	launchWindow = 750 * time.Millisecond
	launchAppear = 20 * time.Second
)

func (o LaunchOptions) withDefaults() LaunchOptions {
	if o.Window <= 0 {
		o.Window = launchWindow
	}
	if o.Appear <= 0 {
		o.Appear = launchAppear
	}
	if o.Interval <= 0 {
		o.Interval = settleInterval
	}
	return o
}

// ErrAppNotAppeared reports a launched app that never put anything in
// the tree. It is distinct from a snapshot error: the engine answered,
// every time, with nothing.
var ErrAppNotAppeared = errors.New("launched app did not show an actionable screen")

// AwaitLaunched returns once a launched app is taking input: its first
// actionable screen has appeared and the launch window has passed.
//
// This belongs on the launch call, not on the page. Every client that
// launches and then acts — a test, an agent, a person — hits the same
// window, and none of them can see it. Returning early from launch puts
// the burden on each of them to guess; returning when the app is ready
// lets a plain fill() written by someone who has never seen a device
// land on the first try.
func AwaitLaunched(ctx context.Context, snap Snapshotter, opts LaunchOptions) error {
	opts = opts.withDefaults()
	if err := awaitAppeared(ctx, snap, opts); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(opts.Window):
		return nil
	}
}

// interactiveTypes are the element types a person can act on. A screen
// is "there" for launch purposes once it offers one of these: a splash
// screen is in the tree a second before the first real screen and has
// nothing but an image and a label, and a window measured from the
// splash ends before the login field has even appeared.
var interactiveTypes = map[string]bool{
	"Button": true, "TextField": true, "SecureTextField": true, "SearchField": true,
	"TextView": true, "Switch": true, "Toggle": true, "Slider": true, "CheckBox": true,
	"RadioButton": true, "Link": true, "Tab": true, "Cell": true, "MenuItem": true,
}

// hasControl reports whether anything on screen can be acted on.
func hasControl(nodes []Node) bool {
	for _, n := range nodes {
		if interactiveTypes[n.Type] && n.Enabled {
			return true
		}
	}
	return false
}

// awaitAppeared polls until the tree offers something to act on. The
// last snapshot error wins over the generic timeout, because it says why.
func awaitAppeared(ctx context.Context, snap Snapshotter, opts LaunchOptions) error {
	deadline := time.Now().Add(opts.Appear)
	var lastErr error
	for {
		got, err := snap(ctx)
		if err == nil && hasControl(got.Nodes) {
			return nil
		}
		lastErr = err
		if !sleepUntil(ctx, opts.Interval, deadline) {
			break
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if lastErr != nil {
		return lastErr
	}
	return ErrAppNotAppeared
}
