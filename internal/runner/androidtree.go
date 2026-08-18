package runner

import (
	"context"
	"fmt"
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
	// mu serializes Snapshot calls: the WebSocket session handles one
	// request at a time, and the page polls faster than a slow dump.
	mu sync.Mutex
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
	return &AndroidEngine{dev: dev, client: client, adapter: adapter}, nil
}

// Snapshot returns the full UI hierarchy. appBundleID is accepted for
// engineAPI parity but unused: Android page source is always the whole
// screen, which is exactly what the mirror wants.
func (e *AndroidEngine) Snapshot(_ context.Context, _ string) ([]Node, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	xml, err := e.adapter.Source()
	if err != nil {
		return nil, fmt.Errorf("android page source: %w", err)
	}
	elems, err := dlandroid.ParsePageSource(xml)
	if err != nil {
		return nil, fmt.Errorf("parse android page source: %w", err)
	}
	return convertAndroidElements(elems), nil
}

// Stop tears the session down, gracefully first.
func (e *AndroidEngine) Stop(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	_ = e.adapter.DeleteSession()
	_ = e.client.Close()
	return e.dev.StopDeviceLabDriver()
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
func convertAndroidElements(in []*dlandroid.ParsedElement) []Node {
	indexOf := make(map[*dlandroid.ParsedElement]int, len(in))
	for i, e := range in {
		indexOf[e] = i
	}
	out := make([]Node, len(in))
	for i, e := range in {
		n := Node{
			Index:       i,
			Type:        androidType(e.ClassName),
			Label:       e.ContentDesc,
			Identifier:  resourceIDSuffix(e.ResourceID),
			Value:       e.Text,
			Placeholder: e.HintText,
			Frame: Rect{
				X: float64(e.Bounds.X), Y: float64(e.Bounds.Y),
				Width: float64(e.Bounds.Width), Height: float64(e.Bounds.Height),
			},
			Enabled:  e.Enabled,
			Focused:  e.Focused,
			Selected: e.Selected,
			Hittable: e.Displayed,
			Depth:    e.Depth,
		}
		if e.Parent != nil {
			if pi, ok := indexOf[e.Parent]; ok {
				n.ParentIndex = &pi
			}
		}
		out[i] = n
	}
	return out
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
