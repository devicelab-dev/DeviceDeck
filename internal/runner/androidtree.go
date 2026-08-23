package runner

import (
	"context"
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
}

// StartAndroidEngine installs (if needed) and starts the devicelab
// Android driver on serial, returning a ready engine. Mirrors the
// assembly maestro-runner's CLI performs, minus the flow executor.
//
// Coverage waiver: StartAndroidEngine and Stop drive a real adb +
// emulator and are exercised end-to-end; conversion logic lives in
// convertAndroidElements, which is unit-tested.
func StartAndroidEngine(_ context.Context, serial string) (*AndroidEngine, error) {
	dev, err := device.New(serial)
	if err != nil {
		return nil, fmt.Errorf("android device %s: %w", serial, err)
	}
	if err := dev.InstallDeviceLabDriver(config.GetDriversDir("android")); err != nil {
		return nil, fmt.Errorf("install devicelab android driver: %w", err)
	}
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
	return &AndroidEngine{dev: dev, client: client, adapter: adapter}, nil
}

// Snapshot returns the full UI hierarchy. appBundleID is accepted for
// engineAPI parity but unused: Android page source is always the whole
// screen, which is exactly what the mirror wants.
func (e *AndroidEngine) Snapshot(_ context.Context, _ string) ([]Node, error) {
	w, h, err := e.ScreenSize()
	if err != nil {
		return nil, err
	}
	xml, err := e.adapter.Source()
	if err != nil {
		return nil, fmt.Errorf("android page source: %w", err)
	}
	elems, err := dlandroid.ParsePageSource(xml)
	if err != nil {
		return nil, fmt.Errorf("parse android page source: %w", err)
	}
	return convertAndroidElements(elems, w, h), nil
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
// Native testID becomes), label = content-desc, value = text,
// placeholder = hint.
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
	root := 0
	out := make([]Node, len(in)+1)
	out[0] = Node{
		Index:    0,
		Type:     "Application",
		Frame:    Rect{Width: float64(screenW), Height: float64(screenH)},
		Enabled:  true,
		Hittable: true,
	}
	for i, e := range in {
		n := Node{
			Index:       i + 1,
			Type:        buttonIfClickable(androidType(e.ClassName), e.Clickable),
			Label:       e.ContentDesc,
			Identifier:  resourceIDSuffix(e.ResourceID),
			Value:       e.Text,
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
			ParentIndex: &root,
		}
		if e.Parent != nil {
			if pi, ok := indexOf[e.Parent]; ok {
				n.ParentIndex = &pi
			}
		}
		out[i+1] = n
	}
	return out
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

// resourceIDSuffix strips the "package:id/" prefix so selectors read the
// way developers wrote them ("username-input", not
// "com.testhiveapp:id/username-input") — matching the iOS identifier
// shape and Maestro's suffix matching.
func resourceIDSuffix(resourceID string) string {
	if idx := strings.Index(resourceID, ":id/"); idx >= 0 {
		return resourceID[idx+len(":id/"):]
	}
	return resourceID
}
