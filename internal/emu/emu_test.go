package emu

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fixture returns a runFunc serving canned outputs keyed by joined args.
func fixture(outputs map[string]string) runFunc {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		key := name + " " + strings.Join(args, " ")
		if out, ok := outputs[key]; ok {
			return []byte(out), nil
		}
		return nil, errors.New("no fixture for: " + key)
	}
}

func TestBooted(t *testing.T) {
	c := &Client{run: fixture(map[string]string{
		"adb devices": "List of devices attached\n" +
			"emulator-5554\tdevice\n" +
			"emulator-5556\toffline\n" + // not ready: skipped
			"R58M123ABC\tdevice\n" + // real device: not ours (brief §3)
			"\n",
		"adb -s emulator-5554 shell getprop ro.product.model":         "sdk_gphone64_arm64\n",
		"adb -s emulator-5554 shell getprop ro.build.version.release": "14\n",
	})}
	devices, err := c.Booted(context.Background())
	if err != nil {
		t.Fatalf("Booted: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices = %+v, want only the ready emulator", devices)
	}
	d := devices[0]
	if d.UDID != "emulator-5554" || d.Name != "sdk_gphone64_arm64" || d.OS != "android 14" || !d.Booted {
		t.Errorf("device = %+v", d)
	}
}

func TestBootedPropFailureFallsBackToSerial(t *testing.T) {
	c := &Client{run: fixture(map[string]string{
		"adb devices": "List of devices attached\nemulator-5554\tdevice\n",
	})}
	devices, err := c.Booted(context.Background())
	if err != nil || len(devices) != 1 {
		t.Fatalf("Booted: %v %+v", err, devices)
	}
	if devices[0].Name != "emulator-5554" || devices[0].OS != "android " {
		t.Errorf("device = %+v, want serial fallback name", devices[0])
	}
}

func TestBootedADBMissing(t *testing.T) {
	c := &Client{run: fixture(nil)}
	if _, err := c.Booted(context.Background()); err == nil {
		t.Fatal("expected error when adb is unavailable")
	}
}

func TestScreenshot(t *testing.T) {
	c := &Client{run: fixture(map[string]string{
		"adb -s emulator-5554 exec-out screencap -p": "PNGDATA",
	})}
	png, err := c.Screenshot(context.Background(), "emulator-5554")
	if err != nil || string(png) != "PNGDATA" {
		t.Fatalf("Screenshot = %q, %v", png, err)
	}
}

// -no-window is not cosmetic: macOS throttles an occluded window's
// rendering, and the emulator's window is occluded whenever DeviceDeck is
// being used, because the screen the user watches is the browser. Losing
// this flag drops the device's own frame production by roughly 4x, which
// looks like a broken console.
func TestBootArgsRunHeadless(t *testing.T) {
	got := bootCommand("avd:e2e_emulator")
	if got[0] != "-avd" || got[1] != "e2e_emulator" {
		t.Fatalf("avd prefix not stripped: %v", got)
	}
	want := map[string]bool{"-no-window": false, "-no-snapshot-save": false, "-no-audio": false}
	for _, arg := range got {
		if _, ok := want[arg]; ok {
			want[arg] = true
		}
	}
	for arg, present := range want {
		if !present {
			t.Errorf("emulator boot is missing %s: %v", arg, got)
		}
	}
	// An id without the prefix is passed through unchanged.
	if bare := bootCommand("e2e_emulator"); bare[1] != "e2e_emulator" {
		t.Errorf("bare id mangled: %v", bare)
	}
}

func TestLaunchApp(t *testing.T) {
	c := &Client{run: fixture(map[string]string{
		"adb -s emulator-5554 shell am force-stop com.example":                                   "",
		"adb -s emulator-5554 shell monkey -p com.example -c android.intent.category.LAUNCHER 1": "Events injected: 1\n",
	})}
	if err := c.LaunchApp(context.Background(), "emulator-5554", "com.example"); err != nil {
		t.Fatalf("LaunchApp: %v", err)
	}
}

// monkey reports a missing package on stdout and still exits zero, so the
// error has to be read out of the output rather than the exit code.
func TestLaunchAppDetectsMissingActivity(t *testing.T) {
	c := &Client{run: fixture(map[string]string{
		"adb -s emulator-5554 shell am force-stop com.nope":                                   "",
		"adb -s emulator-5554 shell monkey -p com.nope -c android.intent.category.LAUNCHER 1": "** No activities found to run, monkey aborted.",
	})}
	err := c.LaunchApp(context.Background(), "emulator-5554", "com.nope")
	if err == nil || !strings.Contains(err.Error(), "no launchable activity") {
		t.Fatalf("expected a launchable-activity error, got %v", err)
	}
}

// A force-stop failure means it was not running; only the launch matters.
func TestLaunchAppIgnoresForceStopFailure(t *testing.T) {
	c := &Client{run: fixture(map[string]string{
		"adb -s emulator-5554 shell monkey -p com.example -c android.intent.category.LAUNCHER 1": "Events injected: 1\n",
	})}
	if err := c.LaunchApp(context.Background(), "emulator-5554", "com.example"); err != nil {
		t.Errorf("force-stop failure should not fail the launch: %v", err)
	}
}

// adb itself failing is distinct from monkey reporting no activity.
func TestLaunchAppReportsAdbFailure(t *testing.T) {
	c := &Client{run: fixture(map[string]string{
		"adb -s emulator-5554 shell am force-stop com.example": "",
	})}
	if err := c.LaunchApp(context.Background(), "emulator-5554", "com.example"); err == nil {
		t.Error("a failed monkey invocation must be reported")
	}
}
