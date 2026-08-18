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
