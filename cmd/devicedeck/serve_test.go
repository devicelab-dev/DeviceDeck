package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/home"
	"github.com/devicelab-dev/DeviceDeck/internal/ready"
	"github.com/devicelab-dev/DeviceDeck/internal/sim"
)

func writeStub(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveBinaryExplicit(t *testing.T) {
	bin := writeStub(t, t.TempDir(), "devicedeck-hid")
	got, err := resolveBinary("devicedeck-hid", bin)
	if err != nil || got != bin {
		t.Fatalf("resolveBinary = %q, %v", got, err)
	}
}

func TestResolveBinaryEnv(t *testing.T) {
	bin := writeStub(t, t.TempDir(), "devicedeck-video")
	t.Setenv("DEVICEDECK_VIDEO", bin)
	got, err := resolveBinary("devicedeck-video", "")
	if err != nil || got != bin {
		t.Fatalf("resolveBinary via env = %q, %v", got, err)
	}
}

func TestResolveBinaryDevBuildPath(t *testing.T) {
	root := t.TempDir()
	release := filepath.Join(root, "sidecar/.build/release")
	if err := os.MkdirAll(release, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := writeStub(t, release, "devicedeck-hid")
	t.Chdir(root)
	got, err := resolveBinary("devicedeck-hid", "")
	if err != nil || got != filepath.Join("sidecar/.build/release", "devicedeck-hid") {
		t.Fatalf("resolveBinary dev path = %q, %v (stub at %s)", got, err, bin)
	}
}

func TestResolveBinarySkipsDirsAndMissing(t *testing.T) {
	// A directory with the right name must not be accepted as the binary.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "devicedeck-hid"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEVICEDECK_HID", filepath.Join(dir, "devicedeck-hid"))
	t.Chdir(t.TempDir()) // ensure the relative dev-build path also misses
	if _, err := resolveBinary("devicedeck-hid", ""); err == nil {
		t.Fatal("expected not-found error")
	}
}

// A path someone named explicitly must be honoured or refused. Falling
// through to a different binary would run a build they did not ask for
// and never mention it — a typo becomes a mystery.
func TestResolveBinaryRefusesBadExplicitPath(t *testing.T) {
	// A working discovery path exists, so a fallthrough would silently
	// succeed and hide the mistake.
	root := t.TempDir()
	release := filepath.Join(root, "sidecar/.build/release")
	if err := os.MkdirAll(release, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStub(t, release, "devicedeck-hid")
	t.Chdir(root)

	_, err := resolveBinary("devicedeck-hid", "/nonexistent/hid")
	if err == nil {
		t.Fatal("a named path that does not exist must be refused")
	}
	if !strings.Contains(err.Error(), "/nonexistent/hid") {
		t.Errorf("error should name the offending path: %v", err)
	}

	t.Setenv("DEVICEDECK_HID", "/also/missing")
	if _, err := resolveBinary("devicedeck-hid", ""); err == nil ||
		!strings.Contains(err.Error(), "DEVICEDECK_HID") {
		t.Errorf("a bad env path should be refused and named: %v", err)
	}
}

// A directory is not an executable, however real the path is.
func TestResolveBinaryRefusesDirectory(t *testing.T) {
	dir := t.TempDir()
	if _, err := resolveBinary("devicedeck-hid", dir); err == nil {
		t.Fatal("a directory must not resolve as the sidecar")
	}
}

// The usage text is what a first contact sees; it has to name the
// command that matters and where the console appears.
func TestUsageNamesTheEssentials(t *testing.T) {
	var b strings.Builder
	usage(&b)
	for _, want := range []string{"devicedeck serve", "127.0.0.1:8787", "/device/", "version"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("usage does not mention %q:\n%s", want, b.String())
		}
	}
}

// With no flag, no env, and nothing on disk next to the executable or in
// the dev-build path, resolveBinary must report a build-it-yourself error
// rather than a bare "not found".
func TestResolveBinaryNotFoundAnywhere(t *testing.T) {
	os.Unsetenv("DEVICEDECK_HID")
	t.Chdir(t.TempDir()) // empty cwd: the relative dev-build path misses
	_, err := resolveBinary("devicedeck-hid", "")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want a not-found error", err)
	}
}

// arg reads os.Args by position and folds a missing argument into the
// empty string, so the top-level dispatch reads uniformly whether or not
// a subcommand was typed.
func TestPrepareHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(home.EnvHome, dir)
	t.Setenv("MAESTRO_RUNNER_HOME", "/elsewhere")
	if err := prepareHome(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("MAESTRO_RUNNER_HOME"); got != dir {
		t.Errorf("runner home = %q, want %q", got, dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "drivers", "android", "devicelab-android-driver.apk")); err != nil {
		t.Errorf("driver not installed: %v", err)
	}
}

func TestPrepareHomeErrors(t *testing.T) {
	t.Run("no home directory", func(t *testing.T) {
		t.Setenv(home.EnvHome, "")
		t.Setenv("HOME", "")
		if err := prepareHome(); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("home is not writable", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(home.EnvHome, file)
		if err := prepareHome(); err == nil || !strings.Contains(err.Error(), "prepare "+file) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestArg(t *testing.T) {
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	os.Args = []string{"devicedeck", "version"}
	if got := arg(1); got != "version" {
		t.Errorf("arg(1) = %q, want version", got)
	}
	if got := arg(9); got != "" {
		t.Errorf("arg past the end = %q, want empty", got)
	}
}

// fakePlatform stands in for the device lister, booter and launcher that
// bringReady drives, recording which calls landed and failing on demand.
type fakePlatform struct {
	devices    []sim.Device
	listErr    error
	bootErr    error
	installErr error
	launchErr  error
	calls      []string
}

func (f *fakePlatform) All(context.Context) ([]sim.Device, error)    { return f.devices, f.listErr }
func (f *fakePlatform) Booted(context.Context) ([]sim.Device, error) { return f.devices, f.listErr }
func (f *fakePlatform) Boot(_ context.Context, id string) error {
	f.calls = append(f.calls, "boot "+id)
	return f.bootErr
}

func (f *fakePlatform) LaunchApp(_ context.Context, udid, appID string) error {
	f.calls = append(f.calls, "launch "+udid+" "+appID)
	return f.launchErr
}

func (f *fakePlatform) ResetApp(_ context.Context, udid, appID string) error {
	f.calls = append(f.calls, "reset "+udid+" "+appID)
	return nil
}

func (f *fakePlatform) Install(_ context.Context, udid, path string) error {
	f.calls = append(f.calls, "install "+udid+" "+path)
	return f.installErr
}

func (f *fakePlatform) OpenURL(_ context.Context, udid, u string) error {
	f.calls = append(f.calls, "open "+udid+" "+u)
	return nil
}

func runBringReady(t *testing.T, f *fakePlatform, app string) ready.Status {
	t.Helper()
	var out bytes.Buffer
	bringReady(context.Background(), f, f, f, "http://127.0.0.1:8787", app, &out)
	var st ready.Status
	if err := json.Unmarshal(out.Bytes(), &st); err != nil {
		t.Fatalf("status is not JSON: %v\n%s", err, out.String())
	}
	return st
}

func TestBringReady(t *testing.T) {
	iosStopped := sim.Device{UDID: "IOS-UDID", Name: "iPhone 17 Pro", OS: "iOS 26.2"}
	androidBooted := sim.Device{UDID: "emulator-5554", Name: "Pixel", OS: "android", Booted: true}
	for _, tc := range []struct {
		name      string
		fake      *fakePlatform
		app       string
		wantReady bool
		wantUDID  string
		wantCalls []string
		wantWarn  string
		wantInst  bool
	}{
		{
			name: "booted Android is launched through the router, not simctl",
			fake: &fakePlatform{devices: []sim.Device{iosStopped, androidBooted}}, app: "dev.devicelab.testhive",
			wantReady: true, wantUDID: "emulator-5554",
			wantCalls: []string{"launch emulator-5554 dev.devicelab.testhive"}, wantInst: true,
		},
		{
			name:      "stopped iOS at the floor is booted",
			fake:      &fakePlatform{devices: []sim.Device{iosStopped}},
			wantReady: true, wantUDID: "IOS-UDID", wantCalls: []string{"boot IOS-UDID"},
		},
		{
			name:      "boot failure becomes a warning",
			fake:      &fakePlatform{devices: []sim.Device{iosStopped}, bootErr: errors.New("simctl exploded")},
			wantReady: true, wantUDID: "IOS-UDID", wantWarn: "boot: simctl exploded",
		},
		{
			name: "app file is installed, not launched",
			fake: &fakePlatform{devices: []sim.Device{androidBooted}}, app: "/tmp/TestHive.apk",
			wantReady: true, wantUDID: "emulator-5554",
			wantCalls: []string{"install emulator-5554 /tmp/TestHive.apk"}, wantInst: true, wantWarn: "installed the build",
		},
		{
			name: "install failure is a warning and the app is not installed",
			fake: &fakePlatform{devices: []sim.Device{androidBooted}, installErr: errors.New("no space")}, app: "/tmp/TestHive.apk",
			wantReady: true, wantUDID: "emulator-5554", wantWarn: "install: no space",
		},
		{
			name: "launch failure is a warning",
			fake: &fakePlatform{devices: []sim.Device{androidBooted}, launchErr: errors.New("not installed")}, app: "com.x",
			wantReady: true, wantUDID: "emulator-5554", wantWarn: "launch: not installed", wantInst: true,
		},
		{
			name: "listing failure reports not ready",
			fake: &fakePlatform{listErr: errors.New("adb down")},
		},
		{
			name: "nothing at the floor reports not ready",
			fake: &fakePlatform{devices: []sim.Device{{UDID: "OLD", OS: "iOS 18.6"}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := runBringReady(t, tc.fake, tc.app)
			if st.Ready != tc.wantReady || st.UDID != tc.wantUDID {
				t.Fatalf("status = %+v", st)
			}
			if st.AppInstalled != tc.wantInst {
				t.Errorf("appInstalled = %v, want %v", st.AppInstalled, tc.wantInst)
			}
			if tc.wantCalls != nil && !reflect.DeepEqual(tc.fake.calls, tc.wantCalls) {
				t.Errorf("calls = %q, want %q", tc.fake.calls, tc.wantCalls)
			}
			if tc.wantWarn != "" && !strings.Contains(strings.Join(st.Warnings, "\n"), tc.wantWarn) {
				t.Errorf("warnings = %q, want one containing %q", st.Warnings, tc.wantWarn)
			}
			if tc.wantReady && !strings.HasPrefix(st.MirrorURL, "http://127.0.0.1:8787/device/"+tc.wantUDID) {
				t.Errorf("mirrorUrl = %q", st.MirrorURL)
			}
		})
	}
}

func TestIsAppFileAndReadyAppID(t *testing.T) {
	for _, tc := range []struct {
		app    string
		isFile bool
		id     string
	}{
		{"dev.devicelab.testhive", false, "dev.devicelab.testhive"},
		{"/tmp/TestHive.app", true, ""},
		{"TestHive.apk", true, ""},
		{"build/TestHive", true, ""},
		{"", false, ""},
	} {
		if got := isAppFile(tc.app); got != tc.isFile {
			t.Errorf("isAppFile(%q) = %v, want %v", tc.app, got, tc.isFile)
		}
		if got := readyAppID(tc.app); got != tc.id {
			t.Errorf("readyAppID(%q) = %q, want %q", tc.app, got, tc.id)
		}
	}
}

func TestParseServeFlags(t *testing.T) {
	got, err := parseServeFlags([]string{"--addr", ":9999", "--ready", "--app", "com.x", "--fps", "15", "--keep-devices"})
	if err != nil {
		t.Fatal(err)
	}
	want := &serveFlags{addr: ":9999", ready: true, readyApp: "com.x", fps: 15, keepDevices: true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("flags = %+v, want %+v", got, want)
	}
	if def, err := parseServeFlags(nil); err != nil || def.addr != "127.0.0.1:8787" || def.fps != 30 {
		t.Errorf("defaults = %+v, %v", def, err)
	}
	if _, err := parseServeFlags([]string{"--no-such-flag"}); err == nil {
		t.Error("unknown flag accepted")
	}
}

type fakeKiller struct {
	killed []string
	err    error
}

func (k *fakeKiller) Kill(_ context.Context, serial string) error {
	k.killed = append(k.killed, serial)
	return k.err
}

func TestPowerOffAndroid(t *testing.T) {
	k := &fakeKiller{err: errors.New("console gone")}
	powerOffAndroid(context.Background(), []string{"IOS-UDID", "emulator-5554", "emulator-5556"}, k)
	if want := []string{"emulator-5554", "emulator-5556"}; !reflect.DeepEqual(k.killed, want) {
		t.Errorf("killed = %q, want %q", k.killed, want)
	}
}

func TestWaitForExit(t *testing.T) {
	errCh := make(chan error, 1)
	errCh <- errors.New("listen: address in use")
	if err := waitForExit(errCh); err == nil {
		t.Fatal("listener failure not returned")
	}
	done := make(chan error, 1)
	go func() { done <- waitForExit(make(chan error)) }()
	// Give Notify time to register before the signal is raised, otherwise
	// SIGTERM's default action ends the test process.
	time.Sleep(50 * time.Millisecond)
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Errorf("signal exit returned %v", err)
	}
}

func TestBuildStack(t *testing.T) {
	st := buildStack("/nonexistent/devicedeck-hid", "/nonexistent/devicedeck-video", 30)
	if st.srv == nil || st.emu == nil || len(st.devices) != 2 || st.launches.Android != st.emu {
		t.Fatalf("stack not wired: %+v", st)
	}
	srv := httptest.NewServer(st.srv.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("console status = %d", resp.StatusCode)
	}
}
