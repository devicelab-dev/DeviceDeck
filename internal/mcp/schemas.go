package mcp

// Tool descriptions. Written for the agent reading tools/list: what the tool
// does and when to reach for it, not how it is implemented.
const (
	descListDevices = "List every simulator and emulator on the machine, running or not, with its " +
		"udid (iOS) or serial (Android), name, platform, and boot state. Start here to find a device."
	descPageURL = "Return the URL of a device's automation page. Open it with your own browser tools " +
		"(Playwright, Puppeteer) to drive the device by selector: the native UI is mirrored as real DOM, " +
		"so app accessibility identifiers are data-testid and roles/labels are ARIA."
	descUITree = "Fetch the device's current UI tree as JSON — every visible native element with its " +
		"identifier, role, label, value, and frame. Read it to decide what to act on."
	descBoot = "Boot a simulator or emulator by udid/serial and wait for it to be ready. A no-op if it " +
		"is already booted."
	descInstall = "Install an app on a device from a file path on the machine running the server: a " +
		".app for an iOS Simulator, a .apk for an Android emulator (not a device .ipa — that is a " +
		"real-device build). Use it before launch_app when the app is not yet installed; launch_app " +
		"also accepts an appFile to install-then-launch in one call."
	descLaunch = "Launch an app on a device and wait until it is taking input. Fresh by default — the " +
		"app's data is wiped so it starts at a first-run screen, the clean slate a new session expects; " +
		"pass fresh=false to resume where the last session left off."
	descTap = "Tap an element by its testid (the app's accessibility identifier). Resolves the id " +
		"against the current UI tree and taps its centre — a durable selector, not a coordinate. To type " +
		"into a field, tap it, then drive the keyboard through the device page (device_page_url) with " +
		"your browser tools, which verify each keystroke against the device."
	descAssert = "Check that an element is on screen, by testid (its accessibility identifier) or by " +
		"text (a substring of a visible label or value). A read-only assertion against the device's own " +
		"tree — the same check a captured flow records."
	descSnapshot = "Read the screen as a compact, ref-stable list an agent acts on: each interactive " +
		"element as `eN role \"name\" testid=… @x,y`. Refs stay stable across calls — act on eN from a " +
		"prior snapshot — and a new element is marked `*`. mode: \"interactive\" (default, controls only), " +
		"\"full\" (adds named text), or \"diff\" (what changed since the last snapshot: + new, - gone, = " +
		"same). A pending system dialog is surfaced on the first line. Prefer this over ui_tree for acting."
	descLongPress = "Long-press an element by its testid (its accessibility identifier): resolves the id " +
		"against the current tree and holds a press on its centre — for context menus and reorder handles " +
		"a quick tap will not trigger."
	descSwipe = "Scroll the screen in a direction (up, down, left, right). The direction names where the " +
		"content moves, so \"up\" reveals what is below. Use it to bring an off-screen element into view, " +
		"then snapshot again."
	descPress = "Deliver a hardware-style key or system gesture the mirror has no on-screen button for: " +
		"home, appSwitcher, notifications, lock, or enter (submit the keyboard)."
	descFindElement = "Search the current screen for an element by testid, role (button, textbox, tab, …), " +
		"a substring of its name, or a substring of visible text — any combination narrows. Returns each " +
		"match as `role \"name\" testid=… @x,y`. Use it to confirm a target is present and how to address it " +
		"without reading the whole tree."
	descAppSkills = "Read what is known about a specific app — the facts you cannot infer from the tree, " +
		"like which testid is the real submit button, that the app resets in memory so a relaunch is a " +
		"clean slate, or that a screen is tappable a beat after it appears. launch_app returns these too. " +
		"Write new ones as Markdown under app-skills/<bundleId>/ as you learn the app."
	descOpenURL = "Open a URL on the device — an https link, or a custom deep-link scheme the app " +
		"registered (myapp://checkout) — to jump a flow straight to a screen instead of navigating there " +
		"by hand."
	descScreenshot = "Capture the device's current screen as a PNG image. Use it to see the device " +
		"when structure (ui_tree) is not enough — a rendered layout, an image, a visual state."
)

// schemaNone is the input schema for a tool that takes no arguments.
func schemaNone() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

// schemaDevice is the schema for a tool that takes only a device.
func schemaDevice() map[string]any {
	return object(map[string]any{"udid": deviceProp()}, []string{"udid"})
}

// schemaDeviceApp is the schema for a tool taking a device and an optional
// app to scope to. When appRequired is false the app narrows the result but
// may be omitted.
func schemaDeviceApp(appRequired bool) map[string]any {
	props := map[string]any{"udid": deviceProp(), "app": appProp()}
	required := []string{"udid"}
	if appRequired {
		// device_page_url still works without an app, but naming one scopes
		// the page to a single bundle, which is what tests want — so it is
		// documented as expected, not enforced.
		required = []string{"udid"}
	}
	return object(props, required)
}

// schemaLaunch is the schema for launch_app: a device, an app, and the
// fresh/resume toggle.
func schemaLaunch() map[string]any {
	props := map[string]any{
		"udid": deviceProp(),
		"app":  appProp(),
		"fresh": map[string]any{
			"type":        "boolean",
			"description": "Wipe app data and start at a first-run screen (default true). false resumes the app as left.",
		},
	}
	return object(props, []string{"udid", "app"})
}

func schemaInstall() map[string]any {
	return object(map[string]any{
		"udid": deviceProp(),
		"appFile": map[string]any{
			"type":        "string",
			"description": "Path to the app file on the server machine: a .app (iOS Simulator) or .apk (Android emulator).",
		},
	}, []string{"udid", "appFile"})
}

// schemaTap is the schema for tap: a device, the testid to tap, and an
// optional app to scope the tree resolution to.
func schemaTap() map[string]any {
	props := map[string]any{
		"udid":   deviceProp(),
		"app":    appProp(),
		"testid": map[string]any{"type": "string", "description": "The element's testid (its app accessibility identifier)."},
	}
	return object(props, []string{"udid", "testid"})
}

// schemaAssert is the schema for assert_visible: a device and one of testid or
// text to look for.
func schemaAssert() map[string]any {
	props := map[string]any{
		"udid":   deviceProp(),
		"app":    appProp(),
		"testid": map[string]any{"type": "string", "description": "Match an element by this exact accessibility identifier."},
		"text":   map[string]any{"type": "string", "description": "Match an element whose visible label or value contains this text."},
	}
	return object(props, []string{"udid"})
}

func deviceProp() map[string]any {
	return map[string]any{"type": "string", "description": "Device udid (iOS) or serial (Android), from list_devices."}
}

func appProp() map[string]any {
	return map[string]any{"type": "string", "description": "App bundle id (iOS) or package name (Android)."}
}

func object(props map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required}
}

// schemaSnapshot is the schema for the snapshot tool: a device, an optional
// app to scope to, and the view mode.
func schemaSnapshot() map[string]any {
	props := map[string]any{
		"udid": deviceProp(),
		"app":  appProp(),
		"mode": map[string]any{
			"type":        "string",
			"enum":        []string{"interactive", "full", "diff"},
			"description": "interactive (default): controls only. full: adds named text. diff: changes since the last snapshot.",
		},
	}
	return object(props, []string{"udid"})
}

// schemaSwipe is the schema for the swipe tool: a device and a direction.
func schemaSwipe() map[string]any {
	props := map[string]any{
		"udid": deviceProp(),
		"direction": map[string]any{
			"type":        "string",
			"enum":        []string{"up", "down", "left", "right"},
			"description": "where the content moves; \"up\" reveals what is below",
		},
	}
	return object(props, []string{"udid", "direction"})
}

// schemaPress is the schema for the press tool: a device and a key/gesture.
func schemaPress() map[string]any {
	props := map[string]any{
		"udid": deviceProp(),
		"key": map[string]any{
			"type":        "string",
			"enum":        []string{"home", "appSwitcher", "notifications", "lock", "enter"},
			"description": "the hardware key or system gesture to deliver",
		},
	}
	return object(props, []string{"udid", "key"})
}

// schemaFind is the schema for find_element: a device, an optional app, and
// any of the match criteria.
func schemaFind() map[string]any {
	props := map[string]any{
		"udid":   deviceProp(),
		"app":    appProp(),
		"testid": map[string]any{"type": "string", "description": "exact accessibility identifier"},
		"role":   map[string]any{"type": "string", "description": "exact role: button, textbox, tab, …"},
		"name":   map[string]any{"type": "string", "description": "substring of the element's name"},
		"text":   map[string]any{"type": "string", "description": "substring of a visible label or value"},
	}
	return object(props, []string{"udid"})
}

// schemaApp is the schema for a tool keyed only on an app bundle id.
func schemaApp() map[string]any {
	return object(map[string]any{"app": appProp()}, []string{"app"})
}

// schemaOpenURL is the schema for open_url: a device and the URL to open.
func schemaOpenURL() map[string]any {
	props := map[string]any{
		"udid": deviceProp(),
		"url":  map[string]any{"type": "string", "description": "the https or custom-scheme URL to open"},
	}
	return object(props, []string{"udid", "url"})
}
