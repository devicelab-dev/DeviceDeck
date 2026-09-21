// Package uisem is the Go side of the mirror's UI semantics: the map from a
// native element type to its ARIA-ish role, and which roles an agent acts
// on. It is the single Go source for that classification, shared by capture
// (desert lint) and the MCP snapshot, so the two never drift. The browser
// mirror (device.js) carries the same table for the DOM it serves; that
// copy lives across a language boundary and cannot be shared, but every
// Go consumer reads this one.
package uisem

// roles maps a native element type (XCUIElementType, or the Android classes
// normalised onto it) to the role a snapshot prints. A type with no entry is
// a plain text carrier — findable by its text, but not itself a control.
var roles = map[string]string{
	"Button":           "button",
	"Link":             "link",
	"TextField":        "textbox",
	"SecureTextField":  "textbox",
	"SearchField":      "searchbox",
	"Switch":           "switch",
	"Toggle":           "switch",
	"Slider":           "slider",
	"CheckBox":         "checkbox",
	"RadioButton":      "radio",
	"SegmentedControl": "radiogroup",
	"Stepper":          "spinbutton",
	"Picker":           "listbox",
	"PickerWheel":      "listbox",
	"Image":            "img",
	"Icon":             "img",
	"Cell":             "listitem",
	"Table":            "list",
	"CollectionView":   "list",
	"NavigationBar":    "navigation",
	"TabBar":           "tablist",
	"Tab":              "tab",
	"ToolBar":          "toolbar",
	"Alert":            "alertdialog",
	"Sheet":            "dialog",
	"Menu":             "menu",
	"MenuItem":         "menuitem",
	"StaticText":       "text",
}

// interactive is the set of roles an agent taps, types into, or toggles —
// the ones whose absence of a durable name or identifier costs a wrong or
// impossible action rather than merely a longer snapshot.
var interactive = map[string]bool{
	"button": true, "link": true, "textbox": true, "searchbox": true,
	"switch": true, "slider": true, "checkbox": true, "radio": true,
	"tab": true, "menuitem": true, "spinbutton": true, "listbox": true,
	"listitem": true,
}

// Role returns the ARIA-ish role for a native element type, or "" when the
// type carries no role and is only a text node.
func Role(elementType string) string {
	return roles[elementType]
}

// Interactive reports whether a native element type maps to a role an agent
// acts on.
func Interactive(elementType string) bool {
	return interactive[roles[elementType]]
}

// TextEntry reports whether a native element type is a field an agent
// types into. It is the one place that knows the three field types, so the
// recorder's secure-field tracking and the snapshot's naming cannot drift.
func TextEntry(elementType string) bool {
	return elementType == "TextField" || elementType == "SecureTextField" || elementType == "SearchField"
}

// Dialog reports whether a type is a modal a snapshot should surface at the
// top level, because nothing else on screen matters until it is handled.
func Dialog(elementType string) bool {
	r := roles[elementType]
	return r == "alertdialog" || r == "dialog"
}
