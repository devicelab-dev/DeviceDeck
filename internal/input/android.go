package input

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/platform"
)

// AndroidInjector is the device-side executor for translated Android
// input — implemented by the runner's AndroidEngine on top of the
// devicelab driver session (server-side injection, no per-event adb
// process spawn).
type AndroidInjector interface {
	Click(x, y int) error
	Swipe(x1, y1, x2, y2, durationMs int) error
	Text(text string) error
	KeyCode(code int) error
	ScreenSize() (w, h int, err error)
}

// Router sends each frame to its platform's backend: iOS frames go to
// the HID sidecar verbatim; Android frames are assembled into driver
// calls. Implements the server's FrameSender.
type Router struct {
	ios interface {
		SendFrame(ctx context.Context, udid string, frame []byte) error
	}
	resolve func(ctx context.Context, udid string) (AndroidInjector, error)

	mu          sync.Mutex
	translators map[string]*androidTranslator
}

// NewRouter wires the platform router. resolve provides the Android
// injector for a serial (typically runner.Engines.AndroidInjector).
func NewRouter(ios *Manager, resolve func(ctx context.Context, udid string) (AndroidInjector, error)) *Router {
	return &Router{ios: ios, resolve: resolve, translators: make(map[string]*androidTranslator)}
}

// SendFrame routes one encoded input frame by platform.
func (r *Router) SendFrame(ctx context.Context, udid string, frame []byte) error {
	if !platform.IsAndroidSerial(udid) {
		return r.ios.SendFrame(ctx, udid, frame)
	}
	inj, err := r.resolve(ctx, udid)
	if err != nil {
		return err
	}
	r.mu.Lock()
	tr, ok := r.translators[udid]
	if !ok {
		tr = &androidTranslator{now: time.Now}
		r.translators[udid] = tr
	}
	r.mu.Unlock()
	return tr.handle(inj, frame)
}

// tapSlopPx is the maximum down→up travel that still counts as a tap;
// anything farther becomes a swipe. Matches Android's own touch slop
// ballpark at typical densities.
const tapSlopPx = 24

// androidTranslator assembles the page's streamed touch/key frames into
// discrete driver calls: down..move..up → tap or swipe, key usages →
// text and keycodes. One per device; the page streams one gesture at a
// time.
type androidTranslator struct {
	mu     sync.Mutex
	now    func() time.Time
	w, h   int
	down   bool
	downX  float64
	downY  float64
	lastX  float64
	lastY  float64
	downAt time.Time
	// Typed characters batch here and flush on idle or before any
	// non-typing event: one driver call per word instead of one slow
	// WebSocket round-trip per keystroke (which starved tree polls).
	textBuf    []rune
	flushTimer *time.Timer
}

// textFlushDelay is how long typing may pause before the buffered text
// ships — shorter than a human keystroke gap, longer than an automated
// typer's per-character delay.
const textFlushDelay = 200 * time.Millisecond

func (t *androidTranslator) handle(inj AndroidInjector, frame []byte) error {
	ev, ok := Decode(frame)
	if !ok {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	switch ev.Kind {
	case EventTouch:
		if err := t.flushTextLocked(inj); err != nil {
			return err
		}
		return t.touch(inj, ev)
	case EventKey:
		return t.key(inj, ev)
	case EventGesture:
		return t.gesture(inj, ev.Gesture)
	case EventLegacyButton:
		// Legacy buttons: 0 = home, 1 = lock (power).
		if ev.Code == 0 {
			return inj.KeyCode(keycodeHome)
		}
		return inj.KeyCode(keycodePower)
	default:
		// Two-finger and iOS-specific frames: no Android mapping —
		// dropped rather than guessed.
		return nil
	}
}

// Android keycodes for system-level actions.
const (
	keycodeHome          = 3
	keycodePower         = 26
	keycodeAppSwitch     = 187
	keycodeNotifications = 83 // KEYCODE_NOTIFICATION
)

// gesture maps iOS-shaped system gestures onto their Android keycodes.
func (t *androidTranslator) gesture(inj AndroidInjector, g Gesture) error {
	switch g {
	case GestureSwipeToHome:
		return inj.KeyCode(keycodeHome)
	case GestureAppSwitcher:
		return inj.KeyCode(keycodeAppSwitch)
	case GestureNotificationCenter:
		return inj.KeyCode(keycodeNotifications)
	case GestureLockScreen:
		return inj.KeyCode(keycodePower)
	}
	return nil
}

// touch runs the gesture state machine. Coordinates arrive normalized;
// the screen size (cached from the injector) scales them to pixels.
func (t *androidTranslator) touch(inj AndroidInjector, ev Event) error {
	if t.w == 0 {
		w, h, err := inj.ScreenSize()
		if err != nil {
			return err
		}
		t.w, t.h = w, h
	}
	switch ev.Phase {
	case TouchDown:
		t.down = true
		t.downX, t.downY = ev.X, ev.Y
		t.lastX, t.lastY = ev.X, ev.Y
		t.downAt = t.now()
	case TouchMove:
		t.lastX, t.lastY = ev.X, ev.Y
	case TouchUp:
		if !t.down {
			return nil
		}
		t.down = false
		return t.finishGesture(inj, ev)
	}
	return nil
}

// finishGesture converts a completed down..up sequence into a tap or a
// swipe with the gesture's real duration.
func (t *androidTranslator) finishGesture(inj AndroidInjector, up Event) error {
	x1, y1 := int(t.downX*float64(t.w)), int(t.downY*float64(t.h))
	x2, y2 := int(up.X*float64(t.w)), int(up.Y*float64(t.h))
	dx, dy := x2-x1, y2-y1
	if dx*dx+dy*dy <= tapSlopPx*tapSlopPx {
		start := t.now()
		err := inj.Click(x2, y2)
		slog.Debug("android input: click", "x", x2, "y", y2, "took", time.Since(start), "error", err)
		return err
	}
	durMs := int(t.now().Sub(t.downAt).Milliseconds())
	if durMs < 50 {
		durMs = 50
	}
	if durMs > 2000 {
		durMs = 2000
	}
	err := inj.Swipe(x1, y1, x2, y2, durMs)
	slog.Debug("android input: swipe", "durMs", durMs, "error", err)
	return err
}

// key maps one HID usage to driver input: printable characters batch
// into the text buffer (uppercased under shift), control keys flush the
// buffer and press an Android keycode.
func (t *androidTranslator) key(inj AndroidInjector, ev Event) error {
	const shift = 0x02
	if r, ok := hidRune(ev.Usage, ev.Mod&shift != 0); ok {
		t.textBuf = append(t.textBuf, r)
		if t.flushTimer != nil {
			t.flushTimer.Stop()
		}
		t.flushTimer = time.AfterFunc(textFlushDelay, func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			// Frames are fire-and-forget from the page; an idle-flush
			// failure has no caller to report to.
			_ = t.flushTextLocked(inj)
		})
		return nil
	}
	if code, ok := hidKeycode(ev.Usage); ok {
		if err := t.flushTextLocked(inj); err != nil {
			return err
		}
		return inj.KeyCode(code)
	}
	return nil
}

// flushTextLocked ships the buffered characters as one driver call.
// Caller holds t.mu.
func (t *androidTranslator) flushTextLocked(inj AndroidInjector) error {
	if t.flushTimer != nil {
		t.flushTimer.Stop()
		t.flushTimer = nil
	}
	if len(t.textBuf) == 0 {
		return nil
	}
	text := string(t.textBuf)
	t.textBuf = nil
	start := time.Now()
	err := inj.Text(text)
	slog.Debug("android input: text", "text", text, "took", time.Since(start), "error", err)
	return err
}

// hidRune maps a USB HID usage to the character it types, honoring
// shift for letters and the common shifted symbols.
func hidRune(usage uint32, shifted bool) (rune, bool) {
	switch {
	case usage >= 0x04 && usage <= 0x1d: // a-z
		r := rune('a' + usage - 0x04)
		if shifted {
			r = r - 'a' + 'A'
		}
		return r, true
	case usage >= 0x1e && usage <= 0x27: // 1-9, 0
		plain := "1234567890"
		shift := "!@#$%^&*()"
		if shifted {
			return rune(shift[usage-0x1e]), true
		}
		return rune(plain[usage-0x1e]), true
	}
	plain := map[uint32]rune{
		0x2c: ' ', 0x2d: '-', 0x2e: '=', 0x2f: '[', 0x30: ']', 0x31: '\\',
		0x33: ';', 0x34: '\'', 0x35: '`', 0x36: ',', 0x37: '.', 0x38: '/',
	}
	shiftMap := map[uint32]rune{
		0x2c: ' ', 0x2d: '_', 0x2e: '+', 0x2f: '{', 0x30: '}', 0x31: '|',
		0x33: ':', 0x34: '"', 0x35: '~', 0x36: '<', 0x37: '>', 0x38: '?',
	}
	if shifted {
		r, ok := shiftMap[usage]
		return r, ok
	}
	r, ok := plain[usage]
	return r, ok
}

// hidKeycode maps control-key HID usages to Android keycodes.
func hidKeycode(usage uint32) (int, bool) {
	codes := map[uint32]int{
		0x28: 66,  // Enter
		0x29: 111, // Escape
		0x2a: 67,  // Backspace (KEYCODE_DEL)
		0x2b: 61,  // Tab
		0x4f: 22,  // DPAD_RIGHT
		0x50: 21,  // DPAD_LEFT
		0x51: 20,  // DPAD_DOWN
		0x52: 19,  // DPAD_UP
	}
	c, ok := codes[usage]
	return c, ok
}
