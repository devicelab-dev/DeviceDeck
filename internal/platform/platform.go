// Package platform answers which platform a device identifier belongs
// to — the one routing fact video capture, the tree engine, and input
// all need without depending on each other.
package platform

import "strings"

// IsAndroidSerial reports whether udid names an adb device rather than
// an iOS simulator: emulator serials look like "emulator-5554",
// simulator UDIDs are UUIDs. DeviceDeck targets emulators only
// (brief §3), so the prefix is the whole grammar.
func IsAndroidSerial(udid string) bool {
	return strings.HasPrefix(udid, "emulator-")
}

// ValidID reports whether id could name a device: a simulator UUID, an adb
// serial, an avd:<name> entry, or "booted". Those use only letters, digits
// and . _ : -, so anything else, such as the "<udid>" placeholder in the
// startup guide pasted as is, is refused before it reaches a device tool.
func ValidID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			c == '.' || c == '_' || c == ':' || c == '-'
		if !ok {
			return false
		}
	}
	return true
}
