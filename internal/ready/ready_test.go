package ready

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResolveDevice(t *testing.T) {
	tests := []struct {
		name     string
		devices  []Device
		wantUDID string
		wantOK   bool
		reason   string
	}{
		{
			name: "a booted device wins outright",
			devices: []Device{
				{UDID: "old", OS: "iOS 26.2", Booted: false},
				{UDID: "up", OS: "iOS 18.6", Booted: true},
			},
			wantUDID: "up", wantOK: true, reason: "already booted",
		},
		{
			name: "newest booted among several, minor tiebreak",
			devices: []Device{
				{UDID: "a", OS: "iOS 27.0", Booted: true},
				{UDID: "b", OS: "iOS 27.1", Booted: true},
			},
			wantUDID: "b", wantOK: true, reason: "already booted",
		},
		{
			name: "a booted iOS outranks a booted Android by version",
			devices: []Device{
				{UDID: "emu", OS: "Android 16", Booted: true},
				{UDID: "ios", OS: "iOS 26.2", Booted: true},
			},
			wantUDID: "ios", wantOK: true,
		},
		{
			name: "no booted: newest runtime at or above the floor",
			devices: []Device{
				{UDID: "older", OS: "iOS 26.2"},
				{UDID: "newest", OS: "iOS 27.1"},
				{UDID: "toolow", OS: "iOS 18.6"},
			},
			wantUDID: "newest", wantOK: true, reason: "newest iOS runtime",
		},
		{
			name: "floor is inclusive at 26.2",
			devices: []Device{
				{UDID: "floor", OS: "iOS 26.2"},
				{UDID: "below", OS: "iOS 26.1"},
			},
			wantUDID: "floor", wantOK: true,
		},
		{
			name:    "nothing qualifies",
			devices: []Device{{UDID: "x", OS: "iOS 18.6"}, {UDID: "a", OS: "Android 16"}},
			wantOK:  false, reason: "no booted device",
		},
		{
			name:     "a booted android beats nothing",
			devices:  []Device{{UDID: "emu", OS: "Android 16", Booted: true}},
			wantUDID: "emu", wantOK: true, reason: "already booted",
		},
		{
			name:     "iOS 27 with no minor parses as 27.0",
			devices:  []Device{{UDID: "major", OS: "iOS 27"}},
			wantUDID: "major", wantOK: true,
		},
		{
			name:    "a malformed iOS version is ignored",
			devices: []Device{{UDID: "bad", OS: "iOS not-a-number"}},
			wantOK:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason, ok := ResolveDevice(tt.devices)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (reason %q)", ok, tt.wantOK, reason)
			}
			if ok && got.UDID != tt.wantUDID {
				t.Errorf("chose %q, want %q", got.UDID, tt.wantUDID)
			}
			if tt.reason != "" && !strings.Contains(reason, tt.reason) {
				t.Errorf("reason %q, want containing %q", reason, tt.reason)
			}
		})
	}
}

func TestBuildStatus(t *testing.T) {
	d := Device{UDID: "U1", Name: "iPhone", OS: "iOS 26.2"}
	s := BuildStatus("http://127.0.0.1:8787/", d, "already booted", true, "com.x", true, []string{"video off"})
	if !s.Ready || s.UDID != "U1" || s.App != "com.x" || !s.AppInstalled {
		t.Fatalf("status wrong: %+v", s)
	}
	if s.MirrorURL != "http://127.0.0.1:8787/device/U1?app=com.x" {
		t.Errorf("mirror url = %q", s.MirrorURL)
	}
	if len(s.Warnings) != 1 {
		t.Errorf("warnings dropped: %+v", s.Warnings)
	}
	// It serializes to JSON an agent can parse.
	b, _ := json.Marshal(s)
	if !strings.Contains(string(b), `"ready":true`) {
		t.Errorf("json wrong: %s", b)
	}

	// Not ready: no device fields, no mirror url, reason carried.
	ns := BuildStatus("http://x", Device{}, "no device", false, "", false, nil)
	if ns.Ready || ns.MirrorURL != "" || ns.UDID != "" || ns.Reason != "no device" {
		t.Errorf("not-ready status wrong: %+v", ns)
	}
	// Ready with no app: mirror url has no query.
	na := BuildStatus("http://x", d, "booted", true, "", false, nil)
	if na.MirrorURL != "http://x/device/U1" {
		t.Errorf("no-app url = %q", na.MirrorURL)
	}
}

func TestMeetsFloor(t *testing.T) {
	for os, want := range map[string]bool{
		"iOS 26.2": true, "iOS 27.0": true, "iOS 27": true,
		"iOS 26.1": false, "iOS 18.6": false, "android": false, "watchOS 11.0": false,
	} {
		if got := MeetsFloor(os); got != want {
			t.Errorf("MeetsFloor(%q) = %v, want %v", os, got, want)
		}
	}
}
