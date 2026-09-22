package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/DeviceDeck/internal/home"
	"github.com/devicelab-dev/DeviceDeck/internal/sim"
)

func TestEngineDetail(t *testing.T) {
	cache := t.TempDir()
	built := filepath.Join(cache, "sim-ios26.2-abc123", "Build", "Products")
	if err := os.MkdirAll(built, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(built, "Runner.xctestrun"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	devices := &fakePlatform{devices: []sim.Device{
		{UDID: "NEW", OS: "iOS 27.0"}, {UDID: "OLD", OS: "iOS 26.2"},
	}}
	detail := engineDetail(cache, devices, fakeBoot{booted: true})
	for udid, want := range map[string]string{
		"NEW":           "Building the iOS runner for iOS 27.0 (first time only",
		"OLD":           "Starting the iOS runner",
		"UNKNOWN":       "Starting the iOS runner",
		"emulator-5554": "Installing and starting the Android driver",
	} {
		if got := detail(udid); !strings.HasPrefix(got, want) {
			t.Errorf("detail(%s) = %q, want %q", udid, got, want)
		}
	}
	if got := engineDetail(cache, devices, fakeBoot{})("emulator-5554"); got != "Waiting for Android to finish booting" {
		t.Errorf("booting emulator: %q", got)
	}
	if got := engineDetail(cache, &fakePlatform{listErr: errors.New("simctl down")}, fakeBoot{})("OLD"); got != "Starting the iOS runner" {
		t.Errorf("listing failure: %q", got)
	}
}

func TestRunnerCacheDir(t *testing.T) {
	t.Setenv(home.EnvHome, "/opt/dd")
	if got := runnerCacheDir(); got != "/opt/dd/cache/devicelab-ios-runner-builds" {
		t.Errorf("runnerCacheDir = %q", got)
	}
	t.Setenv(home.EnvHome, "")
	t.Setenv("HOME", "")
	if got := runnerCacheDir(); got != "" {
		t.Errorf("no home: %q", got)
	}
}

// fakeBoot answers boot checks and waits.
type fakeBoot struct {
	booted  bool
	waitErr error
	waited  *[]string
}

func (f fakeBoot) BootCompleted(context.Context, string) bool { return f.booted }

func (f fakeBoot) WaitBooted(_ context.Context, serial string) error {
	if f.waited != nil {
		*f.waited = append(*f.waited, serial)
	}
	return f.waitErr
}

// recordWarm records which devices were warmed.
type recordWarm struct{ warmed *[]string }

func (r recordWarm) Warm(_ context.Context, udid string) error {
	*r.warmed = append(*r.warmed, udid)
	return nil
}

func TestBootThenWarm(t *testing.T) {
	var waited, warmed []string
	b := bootThenWarm{android: fakeBoot{waited: &waited}, engines: recordWarm{&warmed}}
	for _, udid := range []string{"SIM", "emulator-5554"} {
		if err := b.Warm(context.Background(), udid); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(waited, ",") != "emulator-5554" || strings.Join(warmed, ",") != "SIM,emulator-5554" {
		t.Errorf("waited %v, warmed %v; only the emulator waits for boot", waited, warmed)
	}
	stuck := bootThenWarm{android: fakeBoot{waitErr: errors.New("did not finish booting")}, engines: recordWarm{&warmed}}
	if err := stuck.Warm(context.Background(), "emulator-5556"); err == nil {
		t.Error("a boot that never finishes must fail the warm-up")
	}
}
