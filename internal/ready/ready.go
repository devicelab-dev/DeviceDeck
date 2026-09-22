// Package ready decides which device `devicedeck serve --ready` brings up and
// reports one machine-readable status object for an agent to gate on — the
// same shape a tool's doctor command returns, so an agent can ask "is a
// device ready, and which one" before it starts driving.
package ready

import (
	"fmt"
	"strconv"
	"strings"
)

// Device is the minimal shape ResolveDevice needs: a booted device is
// preferred, otherwise the newest iOS runtime at or above the floor.
type Device struct {
	UDID   string `json:"udid"`
	Name   string `json:"name"`
	OS     string `json:"os"`
	Booted bool   `json:"booted"`
}

// FloorMajor and FloorMinor are the oldest iOS runtime worth defaulting to:
// iOS 26.2 is where the SimRenderServer crash tax that plagued earlier
// runtimes ends, so an unattended default should not pick anything older.
const (
	FloorMajor = 26
	FloorMinor = 2
)

// ResolveDevice picks the device to make ready and says why. A booted device
// wins outright — it is already up and cheapest to use, whatever its runtime.
// Otherwise the newest iOS runtime at or above the floor is chosen, so a
// fresh boot lands on a runtime without the old render-server instability.
// It returns ok=false with a reason when nothing qualifies.
func ResolveDevice(devices []Device) (chosen Device, reason string, ok bool) {
	var bestBooted, bestFloor *Device
	for i := range devices {
		d := &devices[i]
		if d.Booted {
			if bestBooted == nil || newer(*d, *bestBooted) {
				bestBooted = d
			}
			continue
		}
		if major, minor, isIOS := iosVersion(d.OS); isIOS && atLeastFloor(major, minor) {
			if bestFloor == nil || newer(*d, *bestFloor) {
				bestFloor = d
			}
		}
	}
	if bestBooted != nil {
		return *bestBooted, fmt.Sprintf("already booted (%s)", bestBooted.OS), true
	}
	if bestFloor != nil {
		return *bestFloor, fmt.Sprintf("newest iOS runtime at or above %d.%d (%s)", FloorMajor, FloorMinor, bestFloor.OS), true
	}
	return Device{}, fmt.Sprintf("no booted device and no iOS runtime at or above %d.%d installed", FloorMajor, FloorMinor), false
}

// Status is the doctor-style report `--ready` prints: which device was
// picked and why, whether the app is installed and the mirror attached, plus
// any warnings — everything an agent needs to decide it can start.
type Status struct {
	Ready        bool     `json:"ready"`
	UDID         string   `json:"udid,omitempty"`
	Name         string   `json:"name,omitempty"`
	OS           string   `json:"os,omitempty"`
	Reason       string   `json:"reason"`
	App          string   `json:"app,omitempty"`
	AppInstalled bool     `json:"appInstalled"`
	MirrorURL    string   `json:"mirrorUrl,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

// BuildStatus assembles the report from the resolved device and what serve
// learned bringing it up. baseURL is the server's address, used to hand back
// the mirror URL an agent opens.
func BuildStatus(baseURL string, d Device, reason string, ok bool, app string, appInstalled bool, warnings []string) Status {
	s := Status{Ready: ok, Reason: reason, App: app, AppInstalled: appInstalled, Warnings: warnings}
	if ok {
		s.UDID, s.Name, s.OS = d.UDID, d.Name, d.OS
		s.MirrorURL = fmt.Sprintf("%s/device/%s", strings.TrimRight(baseURL, "/"), d.UDID)
		if app != "" {
			s.MirrorURL += "?app=" + app
		}
	}
	return s
}

// iosVersion parses "iOS 26.2" into its major and minor numbers. A minor
// component is optional ("iOS 27" is 27.0). Returns isIOS=false for a
// non-iOS OS string.
func iosVersion(os string) (major, minor int, isIOS bool) {
	fields := strings.Fields(os)
	if len(fields) < 2 || !strings.EqualFold(fields[0], "iOS") {
		return 0, 0, false
	}
	parts := strings.SplitN(fields[1], ".", 2)
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	if len(parts) == 2 {
		minor, _ = strconv.Atoi(parts[1])
	}
	return major, minor, true
}

// MeetsFloor reports whether an OS name such as "iOS 26.2" is an iOS
// runtime at or above the floor, the one DeviceDeck prefers.
func MeetsFloor(osName string) bool {
	major, minor, isIOS := iosVersion(osName)
	return isIOS && atLeastFloor(major, minor)
}

// atLeastFloor reports whether a major.minor is at or above the iOS floor.
func atLeastFloor(major, minor int) bool {
	if major != FloorMajor {
		return major > FloorMajor
	}
	return minor >= FloorMinor
}

// newer reports whether a is a newer iOS runtime than b. A non-iOS OS sorts
// as oldest, so a booted Android never outranks a booted iOS by version but
// still wins over nothing.
func newer(a, b Device) bool {
	am, an, aok := iosVersion(a.OS)
	bm, bn, bok := iosVersion(b.OS)
	if aok != bok {
		return aok
	}
	if am != bm {
		return am > bm
	}
	return an > bn
}
