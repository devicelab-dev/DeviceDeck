package mcp

// Tool descriptions. Written for the agent reading tools/list: what the tool
// does and when to reach for it, not how it is implemented.
const (
	descListDevices = "List every simulator and emulator on the machine, running or not, with its " +
		"udid (iOS) or serial (Android), name, platform, and boot state. Start here to find a device."
	descListApps = "List the app builds the user registered with devicedeck --app — name, bundle id, " +
		"platform, version and minimum OS — plus any it could not use and why. Given a udid, list the " +
		"builds that suit that device, and whether each is installed or already launched there. Start " +
		"here to find the user's app: launch_app on a registered build installs it first."
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
	descTap = "Tap an element named by a ref from snapshot (e12), its testid (the app's accessibility " +
		"identifier), its visible text, or role and name. Resolved against the current UI tree and tapped " +
		"at its centre — a durable selector, not a coordinate. Text and role/name must match exactly one " +
		"element; if several do, use the ref or testid. To type into a field, use fill."
	descFill = "Type a value into a text field, replacing what it held, through the device's driver: it " +
		"focuses the field, types, and reads the field back — failing if the device does not end up " +
		"holding the value. Name the field like tap does (ref, testid, text, or role and name); value is " +
		"what to type (empty clears it); submit=true presses Enter afterwards."
	descAssert = "Check that an element is on screen, by testid (its accessibility identifier) or by " +
		"text (a substring of a visible label or value). A read-only assertion against the device's own " +
		"tree — the same check a captured flow records."
	descSnapshot = "Read the screen as a compact, ref-stable list an agent acts on: each interactive " +
		"element as `eN role \"name\" testid=… @x,y`. Refs stay stable across calls — act on eN from a " +
		"prior snapshot — and a new element is marked `*`. mode: \"interactive\" (default, controls only), " +
		"\"full\" (adds named text), or \"diff\" (what changed since the last snapshot: + new, - gone, = " +
		"same). A pending system dialog is surfaced on the first line. Prefer this over ui_tree for acting."
	descWaitFor = "Wait for the screen instead of sleeping: state \"visible\" (an element named like tap — " +
		"ref, testid, text, or role and name — is on screen), \"gone\" (no such element any more: a spinner " +
		"or dialog finished), or \"settled\" (the screen stopped changing). Fails when timeoutMs (default " +
		"10000, max 60000) runs out — never act as if the wait succeeded."
	descLongPress = "Long-press an element named like tap (ref, testid, text, or role and name): holds a " +
		"press on its centre — for context menus and reorder handles a quick tap will not trigger."
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
	descSettings = "Set device state a test would otherwise reach through the Settings app: appearance " +
		"(dark or light), a simulated GPS location, and an app's permissions granted or revoked without the " +
		"system prompt. Give any subset. Permission names: location, microphone, contacts, photos, calendar, " +
		"motion on both platforms; reminders on iOS; camera and notifications on Android. iOS ends the app " +
		"when some permissions change, so set permissions before launch_app."
	descScreenshot = "Capture the device's current screen as an image, to see what structure (snapshot, " +
		"ui_tree) cannot show — a rendered layout, an image, a visual state. Sent as a JPEG at most 1568px " +
		"on its long side, which is safe to keep in a conversation; full=true returns the original PNG " +
		"(large — only when a detail needs it). Act by selector, never by coordinates read off the image."
)

// schemaNone is the input schema for a tool that takes no arguments.
func schemaNone() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

// schemaOptionalDevice is the schema for a tool that works machine-wide and
// narrows to a device when given one.
func schemaOptionalDevice() map[string]any {
	return object(map[string]any{"udid": deviceProp()}, []string{})
}

// schemaDevice is the schema for a tool that takes only a device.
// schemaScreenshot is the schema for screenshot: a device and whether to
// return the original PNG instead of the capped JPEG.
func schemaScreenshot() map[string]any {
	return object(map[string]any{
		"udid": deviceProp(),
		"full": map[string]any{"type": "boolean", "description": "Return the original full-resolution PNG (large)."},
	}, []string{"udid"})
}

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

// schemaWaitFor is the schema for wait_for: a target like tap's, the state
// to wait for, and a timeout.
func schemaWaitFor() map[string]any {
	props := targetProps()
	props["state"] = map[string]any{"type": "string", "enum": []string{"visible", "gone", "settled"},
		"description": "visible (default), gone, or settled."}
	props["timeoutMs"] = map[string]any{"type": "integer", "description": "How long to wait; default 10000, max 60000."}
	return object(props, []string{"udid"})
}

// schemaTarget is the schema for tap and long_press: a device, one way of
// naming the element, and an optional app to scope the tree to.
func schemaTarget() map[string]any {
	return object(targetProps(), []string{"udid"})
}

// schemaFill is the schema for fill: a target like tap's, the value to type,
// and whether to press Enter after.
func schemaFill() map[string]any {
	props := targetProps()
	props["value"] = map[string]any{"type": "string", "description": "What the field must hold afterwards; empty clears it."}
	props["submit"] = map[string]any{"type": "boolean", "description": "Press Enter after typing."}
	return object(props, []string{"udid", "value"})
}

// targetProps are the ways an act tool can name an element; give one.
func targetProps() map[string]any {
	str := func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	return map[string]any{
		"udid":   deviceProp(),
		"app":    appProp(),
		"ref":    str("A ref from snapshot, e.g. e12 — preferred: it names exactly one element."),
		"testid": str("The element's testid (its app accessibility identifier)."),
		"text":   str("Visible text of the element (a case-insensitive substring of its label, value or placeholder); must match one element."),
		"role":   str("The element's role (button, textbox, tab, …), optionally with name."),
		"name":   str("A substring of the element's accessible name, with or without role."),
	}
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

// schemaSettings is the schema for device_settings: a device and any of
// appearance, location, and permissions to grant or revoke for an app.
func schemaSettings() map[string]any {
	names := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	props := map[string]any{
		"udid":       deviceProp(),
		"appearance": map[string]any{"type": "string", "enum": []string{"dark", "light"}},
		"location": object(map[string]any{
			"lat": map[string]any{"type": "number", "description": "latitude in degrees, -90 to 90"},
			"lon": map[string]any{"type": "number", "description": "longitude in degrees, -180 to 180"},
		}, []string{"lat", "lon"}),
		"app":    map[string]any{"type": "string", "description": "bundle id or package the permissions are for; required with grant or revoke"},
		"grant":  withDescription(names, "permissions to grant, e.g. [\"location\", \"photos\"]"),
		"revoke": withDescription(names, "permissions to revoke"),
	}
	return object(props, []string{"udid"})
}

// withDescription returns a copy of schema with a description.
func withDescription(schema map[string]any, desc string) map[string]any {
	out := map[string]any{"description": desc}
	for k, v := range schema {
		out[k] = v
	}
	return out
}

// schemaOpenURL is the schema for open_url: a device and the URL to open.
func schemaOpenURL() map[string]any {
	props := map[string]any{
		"udid": deviceProp(),
		"url":  map[string]any{"type": "string", "description": "the https or custom-scheme URL to open"},
	}
	return object(props, []string{"udid", "url"})
}
