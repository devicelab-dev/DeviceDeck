package emu

import (
	"context"
	"strings"
	"testing"
)

// dumpsys is the part of `dumpsys package` SetPermission reads: TestHive's
// own permission between two Android ones, one with flags after a colon.
const dumpsys = `Packages:
  Package [com.x] (abc):
    requested permissions:
      android.permission.INTERNET
      com.x.DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION
      android.permission.CAMERA
      android.permission.ACCESS_COARSE_LOCATION: restricted=true
    install permissions:
      android.permission.INTERNET: granted=true
`

func TestEmuSettingsCommands(t *testing.T) {
	ctx := context.Background()
	c := &Client{run: fixture(map[string]string{
		"adb -s emulator-5554 shell cmd uimode night yes":                                      "Night mode: yes",
		"adb -s emulator-5554 shell cmd uimode night no":                                       "Night mode: no",
		"adb -s emulator-5554 emu geo fix -122.009 37.3349":                                    "OK",
		"adb -s emulator-5554 shell dumpsys package com.x":                                     dumpsys,
		"adb -s emulator-5554 shell pm grant com.x android.permission.CAMERA":                  "",
		"adb -s emulator-5554 shell pm revoke com.x android.permission.ACCESS_COARSE_LOCATION": "",
	})}
	calls := map[string]func() error{
		"dark":   func() error { return c.SetAppearance(ctx, "emulator-5554", true) },
		"light":  func() error { return c.SetAppearance(ctx, "emulator-5554", false) },
		"geo":    func() error { return c.SetLocation(ctx, "emulator-5554", 37.3349, -122.009) },
		"camera": func() error { return c.SetPermission(ctx, "emulator-5554", "com.x", "camera", true) },
		// Only coarse is declared: fine is never sent, and the revoke still counts.
		"coarse only": func() error { return c.SetPermission(ctx, "emulator-5554", "com.x", "location", false) },
	}
	for name, call := range calls {
		if err := call(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestEmuSettingsFailures(t *testing.T) {
	ctx := context.Background()
	c := &Client{run: fixture(map[string]string{
		"adb -s emulator-5554 shell dumpsys package com.x":    dumpsys,
		"adb -s emulator-5554 shell dumpsys package com.none": "Unable to find package: com.none\n",
		// camera is declared, but the grant itself fails.
	})}
	tests := []struct {
		name, want string
		err        error
	}{
		{"appearance", "night mode", c.SetAppearance(ctx, "emulator-5554", true)},
		{"location", "set location", c.SetLocation(ctx, "emulator-5554", 1, 2)},
		{"undeclared", "does not declare notifications", c.SetPermission(ctx, "emulator-5554", "com.x", "notifications", true)},
		{"grant fails", "grant camera for com.x", c.SetPermission(ctx, "emulator-5554", "com.x", "camera", true)},
		{"not installed", "not installed", c.SetPermission(ctx, "emulator-5554", "com.none", "camera", true)},
		{"dumpsys fails", "read com.y's permissions", c.SetPermission(ctx, "emulator-5554", "com.y", "camera", true)},
		{"unknown name", "one of: calendar, camera", c.SetPermission(ctx, "emulator-5554", "com.x", "siri", true)},
		{"bad package", "not an Android package", c.SetPermission(ctx, "emulator-5554", "x; rm", "camera", true)},
	}
	for _, tt := range tests {
		if tt.err == nil || !strings.Contains(tt.err.Error(), tt.want) {
			t.Errorf("%s: err %v, want it to mention %q", tt.name, tt.err, tt.want)
		}
	}
}
