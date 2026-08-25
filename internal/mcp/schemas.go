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
	descLaunch = "Launch an app on a device and wait until it is taking input. Fresh by default — the " +
		"app's data is wiped so it starts at a first-run screen, the clean slate a new session expects; " +
		"pass fresh=false to resume where the last session left off."
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

func deviceProp() map[string]any {
	return map[string]any{"type": "string", "description": "Device udid (iOS) or serial (Android), from list_devices."}
}

func appProp() map[string]any {
	return map[string]any{"type": "string", "description": "App bundle id (iOS) or package name (Android)."}
}

func object(props map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required}
}
