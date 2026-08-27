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

// ResetApp clears the package's data so the next launch is a first run.
func TestResetApp(t *testing.T) {
	c := &Client{run: fixture(map[string]string{
		"adb -s emulator-5554 shell pm clear com.example": "Success\n",
	})}
	if err := c.ResetApp(context.Background(), "emulator-5554", "com.example"); err != nil {
		t.Fatalf("ResetApp: %v", err)
	}
}

// pm clear prints Failed for a package it does not know, exiting zero, so
// the result is read from the output.
func TestResetAppDetectsMissingPackage(t *testing.T) {
	c := &Client{run: fixture(map[string]string{
		"adb -s emulator-5554 shell pm clear com.nope": "Failed\n",
	})}
	err := c.ResetApp(context.Background(), "emulator-5554", "com.nope")
	if err == nil || !strings.Contains(err.Error(), "is it installed") {
		t.Fatalf("expected an installed-check error, got %v", err)
	}
}

// A dead adb surfaces as the reset failing outright.
func TestResetAppRunError(t *testing.T) {
	c := &Client{run: fixture(nil)} // no fixture → run returns an error
	err := c.ResetApp(context.Background(), "emulator-5554", "com.example")
	if err == nil || !strings.Contains(err.Error(), "reset com.example") {
		t.Fatalf("expected a reset error, got %v", err)
	}
}

// All lists running emulators plus stopped AVDs, deduping a running AVD
// against its -list-avds entry and prefixing the stopped ones with avd:.
func TestAll(t *testing.T) {
	c := &Client{run: fixture(map[string]string{
		"adb devices": "List of devices attached\nemulator-5554\tdevice\n",
		"adb -s emulator-5554 shell getprop ro.product.model":         "Pixel 7\n",
		"adb -s emulator-5554 shell getprop ro.build.version.release": "14\n",
		"adb -s emulator-5554 emu avd name":                           "Pixel_7\nOK\n",
		"emulator -list-avds":                                         "Pixel_7\nPixel_8\n\n",
	})}
	devices, err := c.All(context.Background())
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	// Running Pixel_7 stays as its serial; Pixel_7 is deduped out of the
	// AVD list; Pixel_8 is added as a stopped avd:.
	if len(devices) != 2 {
		t.Fatalf("devices = %+v, want 2", devices)
	}
	if devices[0].UDID != "emulator-5554" || devices[1].UDID != AVDPrefix+"Pixel_8" {
		t.Errorf("devices = %+v", devices)
	}
}

// A failure listing running devices fails All — the inventory is unknown,
// not empty.
func TestAllBootedError(t *testing.T) {
	c := &Client{run: fixture(nil)} // no "adb devices" fixture → Booted errors
	if _, err := c.All(context.Background()); err == nil {
		t.Fatal("expected error when adb devices fails")
	}
}

// Without the emulator tool on PATH, All still returns the running devices
// (the -list-avds block is skipped) and an unnameable running device does
// not poison the dedup set.
func TestAllWithoutEmulatorTool(t *testing.T) {
	c := &Client{run: fixture(map[string]string{
		"adb devices": "List of devices attached\nemulator-5554\tdevice\n",
		"adb -s emulator-5554 shell getprop ro.product.model":         "Pixel 7\n",
		"adb -s emulator-5554 shell getprop ro.build.version.release": "14\n",
		// no "emu avd name" → avdName returns ""; no "emulator -list-avds"
	})}
	devices, err := c.All(context.Background())
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(devices) != 1 || devices[0].UDID != "emulator-5554" {
		t.Errorf("devices = %+v, want just the running one", devices)
	}
}

// NewClient wires the real command runner; exercise its closure once
// through a command that fails, to cover the error-wrapping path.
func TestNewClient(t *testing.T) {
	c := NewClient()
	if _, err := c.run(context.Background(), "definitely-not-a-real-binary-xyz"); err == nil {
		t.Error("expected the real runner to error on a missing binary")
	}
	// The success path: a real command whose stdout comes back verbatim.
	out, err := c.run(context.Background(), "echo", "ok")
	if err != nil || strings.TrimSpace(string(out)) != "ok" {
		t.Errorf("echo through the real runner: out=%q err=%v", out, err)
	}
}

// Kill powers an emulator off via its console; a dead adb surfaces.
func TestKill(t *testing.T) {
	c := &Client{run: fixture(map[string]string{
		"adb -s emulator-5554 emu kill": "OK\n",
	})}
	if err := c.Kill(context.Background(), "emulator-5554"); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	if err := (&Client{run: fixture(nil)}).Kill(context.Background(), "emulator-5554"); err == nil {
		t.Error("expected a kill error when adb fails")
	}
}

func TestInstall(t *testing.T) {
	var got string
	c := &Client{run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		got = name + " " + strings.Join(args, " ")
		return []byte("Success\n"), nil
	}}
	if err := c.Install(context.Background(), "emulator-5554", "/path/app.apk"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !strings.Contains(got, "adb -s emulator-5554 install -r /path/app.apk") {
		t.Fatalf("call = %q", got)
	}
}

func TestInstallFailureOnStdout(t *testing.T) {
	c := &Client{run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("Failure [INSTALL_FAILED_INVALID_APK]"), nil
	}}
	if err := c.Install(context.Background(), "emulator-5554", "/x.apk"); err == nil {
		t.Fatal("expected an error on Failure output")
	}
}

func TestInstallError(t *testing.T) {
	c := &Client{run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, errors.New("device offline")
	}}
	if err := c.Install(context.Background(), "emulator-5554", "/x.apk"); err == nil {
		t.Fatal("expected an install error")
	}
}
