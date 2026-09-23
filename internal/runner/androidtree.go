package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/devicelab-dev/maestro-runner/pkg/config"
	"github.com/devicelab-dev/maestro-runner/pkg/device"
	dlandroid "github.com/devicelab-dev/maestro-runner/pkg/driver/devicelab"
	"github.com/devicelab-dev/maestro-runner/pkg/maestro"
)

// AndroidEngine owns the devicelab Android driver session for one
// emulator: APK install, on-device server start, and the WebSocket
// session that serves UI hierarchies. The Android counterpart of the
// XCUITest Engine, behind the same engineAPI.
type AndroidEngine struct {
	dev     *device.AndroidDevice
	client  *maestro.Client
	adapter *maestro.Adapter
	// The driver's WebSocket client multiplexes concurrent calls by
	// request ID, so snapshots and input run in parallel — serializing
	// them here starved tree polls behind slow input and made the page
	// act on stale geometry. mu guards only local state.
	mu      sync.Mutex
	screenW int
	screenH int
	// systemIDs are the resource-id prefixes of windows that are not the
	// app's: SystemUI and the current keyboard. Looked up once per session.
	systemIDs []string
}

// StartAndroidEngine installs (if needed) and starts the devicelab Android
// driver on serial, returning a ready engine. Mirrors the assembly
// maestro-runner's CLI performs, minus the flow executor. The driver start
// is retried, because it races dexopt on a cold start — see driverStartRetries.
//
// Coverage waiver: StartAndroidEngine, bringUpAndroidDriver, and Stop drive a
// real adb + emulator and are exercised end-to-end; the retry policy
// (retryStart) and conversion (convertAndroidElements) are unit-tested.
func StartAndroidEngine(_ context.Context, serial string) (*AndroidEngine, error) {
	dev, err := device.New(serial)
	if err != nil {
		return nil, fmt.Errorf("android device %s: %w", serial, err)
	}
	// Install once, outside the retry: it is idempotent and not the flaky
	// step. The driver *start* is — see retryStart.
	if err := dev.InstallDeviceLabDriver(config.GetDriversDir("android")); err != nil {
		return nil, fmt.Errorf("install devicelab android driver: %w", err)
	}
	return retryStart(driverStartRetries,
		func() (*AndroidEngine, error) { return bringUpAndroidDriver(dev) },
		func() { _ = dev.StopDeviceLabDriver() })
}

// driverStartRetries bounds how many times the driver bring-up is
// re-attempted. The devicelab Android driver crashes on startup roughly one
// run in ten — a dexopt race where its own crash-check fires while
// `am instrument` is still warming after a cold start — and the failure
// surfaces downstream as an empty tree and "element not found". The crash is
// transient: a fresh start almost always succeeds, so a bounded retry turns
// a ~10% session-start failure into a negligible one. It belongs here, at
// our seam around the driver, because maestro-runner does not retry and this
// is our reliability problem to own, not the dependency's.
const driverStartRetries = 3

// retryStart runs start up to n times, calling reset between failed attempts
// to clear a partially-started driver before the next try. Returns the first
// success, or the last error if every attempt fails. Kept free of any device
// type so the retry policy is unit-tested without a live emulator; the actual
// bring-up it wraps (bringUpAndroidDriver) carries the e2e coverage waiver.
func retryStart(n int, start func() (*AndroidEngine, error), reset func()) (*AndroidEngine, error) {
	var err error
	for attempt := 1; attempt <= n; attempt++ {
		var eng *AndroidEngine
		if eng, err = start(); err == nil {
			return eng, nil
		}
		slog.Warn("android driver start failed; retrying",
			"attempt", attempt, "of", n, "error", err)
		reset()
	}
	return nil, err
}

// bringUpAndroidDriver starts the driver on an installed device and opens a
// session, returning a ready engine. Split from StartAndroidEngine so the
// retry loop can re-run exactly the flaky part. Cleans up its own partial
// state on failure so a retry starts clean.
//
// Coverage waiver: this and Stop drive a real adb + emulator and are
// exercised end-to-end; the retry policy (retryStart) and conversion logic
// (convertAndroidElements) are unit-tested.
func bringUpAndroidDriver(dev *device.AndroidDevice) (*AndroidEngine, error) {
	if err := dev.StartDeviceLabDriver(device.DefaultDeviceLabDriverConfig()); err != nil {
		return nil, fmt.Errorf("start devicelab android driver: %w", err)
	}
	var client *maestro.Client
	if socket := dev.DeviceLabDriverSocket(); socket != "" {
		client = maestro.NewClient(socket)
	} else {
		client = maestro.NewClientTCP(dev.DeviceLabDriverLocalPort())
	}
	if err := client.Connect(); err != nil {
		_ = dev.StopDeviceLabDriver()
		return nil, fmt.Errorf("connect devicelab android driver: %w", err)
	}
	adapter := maestro.NewAdapter(client)
	if _, err := adapter.CreateSession(); err != nil {
		_ = client.Close()
		_ = dev.StopDeviceLabDriver()
		return nil, fmt.Errorf("create driver session: %w", err)
	}
	tuneAndroidSession(dev, adapter)
	return &AndroidEngine{dev: dev, client: client, adapter: adapter}, nil
}

// tuneAndroidSession applies the two settings that keep the mirror usable,
// both best-effort. Part of bringUpAndroidDriver's e2e coverage waiver: it
// drives a real adb session and is exercised end-to-end.
func tuneAndroidSession(dev *device.AndroidDevice, adapter *maestro.Adapter) {
	// Cap UIAutomator's wait-for-idle: page source runs through it, and
	// with the default the mirror's tree polls block for seconds during
	// typing or animation — the page then taps against stale geometry
	// (e.g. a layout the keyboard has since shifted). 50ms keeps dumps
	// near-live; the mirror's own refresh loop provides the settling.
	if err := adapter.SetAppiumSettings(map[string]interface{}{"waitForIdleTimeout": 50}); err != nil {
		slog.Warn("android driver: setting waitForIdleTimeout failed", "error", err)
	}
	// Keep the soft keyboard down: text lands via key injection, and the
	// IME opening reshapes the layout mid-flow, which races every click
	// that follows typing. Best-effort — a failure only risks flakier
	// geometry, not a broken session.
	if _, err := dev.Shell("settings put secure show_ime_with_hard_keyboard 0"); err != nil {
		slog.Warn("android driver: disabling soft keyboard failed", "error", err)
	}
}

// Snapshot returns the full UI hierarchy. appBundleID is accepted for
// engineAPI parity but unused: Android page source is always the whole
// screen, which is exactly what the mirror wants.
func (e *AndroidEngine) Snapshot(_ context.Context, _ string) ([]Node, error) {
	w, h, err := e.ScreenSize()
	if err != nil {
		return nil, err
	}
	xml, err := androidSnapshotXML(e.client)
	if err != nil {
		return nil, fmt.Errorf("android page source: %w", err)
	}
	elems, err := dlandroid.ParsePageSource(xml)
	if err != nil {
		return nil, fmt.Errorf("parse android page source: %w", err)
	}
	return convertAndroidElements(withoutSystemWindows(elems, e.systemIDPrefixes()), w, h), nil
}

// systemUIIDPrefix marks the system's own chrome: the status bar and the
// navigation bar are SystemUI windows, and every id in them carries it.
const systemUIIDPrefix = "com.android.systemui:"

// systemIDPrefixes returns the id prefixes of windows that are not the
// app's: SystemUI, plus the current keyboard when it can be read. The
// keyboard stays a window while any field holds focus — even with its keys
// hidden — and its root spans the screen, so left in the mirror it sits
// over the app and takes the clicks meant for it.
func (e *AndroidEngine) systemIDPrefixes() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.systemIDs != nil {
		return e.systemIDs
	}
	e.systemIDs = []string{systemUIIDPrefix}
	out, err := e.dev.Shell("settings get secure default_input_method")
	if pkg := imePackage(out); err == nil && pkg != "" {
		e.systemIDs = append(e.systemIDs, pkg+":")
	} else {
		slog.Warn("android driver: keyboard package unknown; its window may reach the mirror", "error", err)
	}
	return e.systemIDs
}

// imePackage is the package of an input method setting such as
// "com.google.android.inputmethod.latin/com.android.inputmethod.latin.LatinIME".
func imePackage(setting string) string {
	pkg, _, found := strings.Cut(strings.TrimSpace(setting), "/")
	if !found || pkg == "" {
		return ""
	}
	return pkg
}

// withoutSystemWindows drops the windows that are not the app's from a
// complete dump: any window carrying an id with one of prefixes. The
// complete dump covers every window — which is why the app's own drawers
// and dialogs are in it — so it also carries the status bar (a clock that
// changes every minute, notification icons an agent reads as the app's
// content) and the keyboard.
func withoutSystemWindows(in []*dlandroid.ParsedElement, prefixes []string) []*dlandroid.ParsedElement {
	system := make(map[*dlandroid.ParsedElement]bool)
	for _, e := range in {
		if hasAnyPrefix(e.ResourceID, prefixes) {
			system[windowOf(e)] = true
		}
	}
	if len(system) == 0 {
		return in
	}
	out := make([]*dlandroid.ParsedElement, 0, len(in))
	for _, e := range in {
		if !system[windowOf(e)] {
			out = append(out, e)
		}
	}
	return out
}

// hasAnyPrefix reports whether s starts with one of prefixes.
func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// windowOf is the root of the window e belongs to.
func windowOf(e *dlandroid.ParsedElement) *dlandroid.ParsedElement {
	for e.Parent != nil {
		e = e.Parent
	}
	return e
}

// snapshotIdleMs caps the driver's wait-for-idle before a dump, for the
// same reason tuneAndroidSession caps it for page source: the mirror polls
// and settles itself, and a long idle wait blocks every poll during typing
// or animation.
const snapshotIdleMs = 50

// androidSnapshotXML asks the driver for its complete dump (UI.snapshot)
// rather than page source (UI.getSource). Only the complete dump carries a
// field's hint: without it an empty field reports its placeholder as its
// text, and the mirror showed an unfilled form as filled — an agent then
// sat at a disabled "Next" with nothing left to type.
func androidSnapshotXML(client *maestro.Client) (string, error) {
	resp, err := client.Call("UI.snapshot", map[string]any{"waitForIdleMs": snapshotIdleMs})
	if err != nil {
		return "", err
	}
	var result maestro.SourceResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("parse snapshot result: %w", err)
	}
	return result.XML, nil
}

// SnapshotState satisfies engineAPI. Like the rest of AndroidEngine it runs
// only against a real emulator (coverage waiver, as AndroidEngine.Snapshot).
// Android's AccessibilityService page source carries no app-lifecycle state,
// so AppState is empty and a caller treats "unknown" and empty alike — the
// foreground annotation is simply absent, never a false claim.
func (e *AndroidEngine) SnapshotState(ctx context.Context, appBundleID string) (Snapshot, error) {
	nodes, err := e.Snapshot(ctx, appBundleID)
	return Snapshot{Nodes: nodes}, err
}

// Stop tears the session down, gracefully first.
func (e *AndroidEngine) Stop(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	_ = e.adapter.DeleteSession()
	_ = e.client.Close()
	return e.dev.StopDeviceLabDriver()
}

// Input injection: server-side via the same driver session the tree
// uses — no per-event adb process, which is what makes the device page
// feel live. Calls run concurrently with snapshots: the WebSocket client
// multiplexes by request ID.
//
// Coverage waiver: these are one-line delegations to the driver session,
// exercised end-to-end against a real emulator.

// Click taps at pixel coordinates.
func (e *AndroidEngine) Click(x, y int) error {
	return e.adapter.Click(x, y)
}

// Swipe drags between pixel coordinates over durationMs.
func (e *AndroidEngine) Swipe(x1, y1, x2, y2, durationMs int) error {
	return e.adapter.SwipeCoords(x1, y1, x2, y2, durationMs)
}

// Text types into the focused element via `input text` over adb: it
// appends at the cursor (SendKeysToActive's setText would clobber the
// field), runs in ~300ms per word (the driver's per-key events took
// seconds), and — deliberately — does not touch the driver session, so
// tree snapshots keep flowing while text lands and the mirror never
// serves stale geometry to the click that follows typing.
func (e *AndroidEngine) Text(text string) error {
	_, err := e.dev.Shell("input text " + shellQuoteInputText(text))
	return err
}

// shellQuoteInputText prepares text for `input text`: spaces become %s
// (the tool's own escape), and the whole argument is single-quoted for
// the shell with embedded quotes escaped.
func shellQuoteInputText(text string) string {
	escaped := strings.ReplaceAll(text, " ", "%s")
	escaped = strings.ReplaceAll(escaped, "'", `'\''`)
	return "'" + escaped + "'"
}

// KeyCode presses an Android keycode (Enter, Backspace, arrows…).
func (e *AndroidEngine) KeyCode(code int) error {
	return e.adapter.PressKeyCode(code)
}

// ScreenSize reports the device's pixel dimensions, cached after the
// first query (wm size never changes on an emulator session).
func (e *AndroidEngine) ScreenSize() (int, int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.screenSizeLocked()
}

// screenSizeLocked is ScreenSize for callers already holding e.mu.
func (e *AndroidEngine) screenSizeLocked() (int, int, error) {
	if e.screenW != 0 {
		return e.screenW, e.screenH, nil
	}
	out, err := e.dev.Shell("wm size")
	if err != nil {
		return 0, 0, fmt.Errorf("wm size: %w", err)
	}
	w, h, err := parseWmSize(out)
	if err != nil {
		return 0, 0, err
	}
	e.screenW, e.screenH = w, h
	return w, h, nil
}

// parseWmSize extracts dimensions from `wm size` output, preferring the
// override line (active resolution) over the physical one.
func parseWmSize(out string) (int, int, error) {
	var w, h int
	for _, line := range strings.Split(out, "\n") {
		if _, err := fmt.Sscanf(strings.TrimSpace(line), "Override size: %dx%d", &w, &h); err == nil {
			return w, h, nil
		}
		_, _ = fmt.Sscanf(strings.TrimSpace(line), "Physical size: %dx%d", &w, &h)
	}
	if w == 0 || h == 0 {
		return 0, 0, fmt.Errorf("unparseable wm size output: %q", out)
	}
	return w, h, nil
}

// androidTypes maps Android widget classes (by simple name) onto the
// XCUITest type vocabulary the device page's role table already speaks —
// one mirror, two platforms. Unmapped classes keep their simple name and
// act as plain text carriers, same as unmapped iOS types.
var androidTypes = map[string]string{
	"Button":                    "Button",
	"ImageButton":               "Button",
	"EditText":                  "TextField",
	"AutoCompleteTextView":      "TextField",
	"MultiAutoCompleteTextView": "TextField",
	"TextView":                  "StaticText",
	"ImageView":                 "Image",
	"CheckBox":                  "CheckBox",
	"CheckedTextView":           "CheckBox",
	"Switch":                    "Switch",
	"SwitchCompat":              "Switch",
	"ToggleButton":              "Switch",
	"RadioButton":               "RadioButton",
	"SeekBar":                   "Slider",
	"ProgressBar":               "ProgressIndicator",
	"RecyclerView":              "CollectionView",
	"ListView":                  "CollectionView",
	"GridView":                  "CollectionView",
	"ScrollView":                "ScrollView",
	"ViewPager":                 "CollectionView",
	"Spinner":                   "Picker",
	"Toolbar":                   "ToolBar",
	"ActionBar":                 "NavigationBar",
	"TabLayout":                 "TabBar",
	"WebView":                   "WebView",
}

// convertAndroidElements maps the flattened UIAutomator hierarchy into
// DeviceDeck's Node shape. Field mapping follows the mirror's contract:
// identifier = resource-id suffix (what Maestro id: matches and React
// Native testID becomes), label = content-desc unless it merely repeats
// the identifier (see androidLabel), value = text, placeholder = hint.
//
// A synthetic full-screen root is prepended as the mirror's scale
// reference: element bounds are screen-absolute, but the app's own root
// view shrinks under adjustResize when the keyboard opens — scaling
// against it would shift every rendered element (and thus every click)
// downward by the keyboard's height.
func convertAndroidElements(in []*dlandroid.ParsedElement, screenW, screenH int) []Node {
	indexOf := make(map[*dlandroid.ParsedElement]int, len(in))
	for i, e := range in {
		indexOf[e] = i + 1
	}
	out := make([]Node, len(in)+1)
	out[0] = Node{
		Index:    0,
		Type:     "Application",
		Frame:    Rect{Width: float64(screenW), Height: float64(screenH)},
		Enabled:  true,
		Hittable: true,
	}
	for i, e := range in {
		parent := 0
		if pi, ok := indexOf[e.Parent]; ok {
			parent = pi
		}
		out[i+1] = androidNode(e, i+1, parent)
	}
	return out
}

// fieldValue is what a field holds. An empty Android field reports its
// hint as its text; text equal to the hint is that hint showing, not a
// value. (A value typed to match the hint exactly reads as empty — the
// dump has no showing-hint flag to tell the two apart.)
func fieldValue(text, hint string) string {
	if hint != "" && text == hint {
		return ""
	}
	return text
}

// androidNode converts one parsed element into a Node at index, parented
// to parent (0, the synthetic root, when the element has no parent in the
// dump).
func androidNode(e *dlandroid.ParsedElement, index, parent int) Node {
	id := resourceIDSuffix(e.ResourceID)
	return Node{
		Index:       index,
		Type:        buttonIfClickable(androidType(e.ClassName), e.Clickable),
		ClassName:   e.ClassName,
		Label:       androidLabel(e.ContentDesc, id),
		Identifier:  id,
		Value:       fieldValue(e.Text, e.HintText),
		Placeholder: e.HintText,
		Frame: Rect{
			X: float64(e.Bounds.X), Y: float64(e.Bounds.Y),
			Width: float64(e.Bounds.Width), Height: float64(e.Bounds.Height),
		},
		Enabled:     e.Enabled,
		Focused:     e.Focused,
		Selected:    e.Selected,
		Hittable:    e.Displayed,
		Depth:       e.Depth + 1,
		ParentIndex: &parent,
	}
}

// androidLabel is the node's label: its content-desc, unless that only
// repeats the identifier.
//
// React Native on Android copies a view's testID into its content-desc as
// well as its resource-id, so a <Text testID="cart-total-text">$8.10</Text>
// arrives as label "cart-total-text", value "$8.10". Every consumer reads
// label before value, so the test ID displaced the text a user actually
// sees: the mirror printed "cart-total-text" as the element's text, an
// agent could not read the total, and IDs polluted text matching. A label
// equal to the identifier carries nothing the identifier does not, so it
// is dropped and the node falls back to its own text. A control left with
// no text still has a name — the device page names an otherwise nameless
// control by its identifier.
func androidLabel(contentDesc, identifier string) string {
	if contentDesc == identifier {
		return ""
	}
	return contentDesc
}

// buttonRoleKeeps names the types whose own role already conveys how they
// are operated, so a clickable one keeps its type rather than becoming a
// button — the input controls, and the structural containers (a list, a
// scroll view, a tab bar) that are clickable without being buttons.
var buttonRoleKeeps = map[string]bool{
	"Button": true, "TextField": true, "CheckBox": true, "Switch": true,
	"RadioButton": true, "Slider": true, "Picker": true, "CollectionView": true,
	"ScrollView": true, "ToolBar": true, "NavigationBar": true, "TabBar": true,
	"WebView": true, "ProgressIndicator": true,
}

// buttonIfClickable promotes a clickable generic container to Button.
//
// Android renders a tappable control — a React Native <TouchableOpacity>,
// most obviously — as a plain ViewGroup with android:clickable, its
// visible label sitting in a child TextView. Left as a generic, an agent
// reading the tree sees no button to press: it drives by getByTestId but
// cannot reason "click the Sign In button", which is how test authoring
// works. iOS reports the same control as a Button, so promoting the
// clickable container gives one role vocabulary across both platforms.
// Controls that already carry an interactive role, and structural
// containers that merely happen to be clickable, keep their type.
func buttonIfClickable(t string, clickable bool) string {
	if clickable && !buttonRoleKeeps[t] {
		return "Button"
	}
	return t
}

// androidType reduces a fully-qualified class name to the mirror's type
// vocabulary.
func androidType(className string) string {
	simple := className
	if idx := strings.LastIndex(className, "."); idx >= 0 {
		simple = className[idx+1:]
	}
	if mapped, ok := androidTypes[simple]; ok {
		return mapped
	}
	return simple
}

// frameworkIDPrefix marks ids Android itself assigns: a dialog's buttons
// are android:id/button1..3 whatever they say, and every screen has an
// android:id/content.
const frameworkIDPrefix = "android:id/"

// resourceIDSuffix strips the "package:id/" prefix so selectors read the
// way developers wrote them ("username-input", not
// "com.testhiveapp:id/username-input") — matching the iOS identifier
// shape and Maestro's suffix matching. Framework ids come back empty: the
// app never chose them, so as test ids they name nothing — a generated
// test read getByTestId('button1') for "LOGOUT", which is the positive
// button of whichever dialog happens to be open. Without one, the control
// is named by its text.
func resourceIDSuffix(resourceID string) string {
	if strings.HasPrefix(resourceID, frameworkIDPrefix) {
		return ""
	}
	if idx := strings.Index(resourceID, ":id/"); idx >= 0 {
		return resourceID[idx+len(":id/"):]
	}
	return resourceID
}
