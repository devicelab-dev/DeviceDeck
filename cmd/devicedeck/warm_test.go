package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestEngineDetail(t *testing.T) {
	detail := engineDetail(fakeBoot{booted: true})
	for udid, want := range map[string]string{
		"DD000000-0000-4000-8000-000000000001": "Starting the iOS agent",
		"emulator-5554":                        "Installing and starting the Android driver",
	} {
		if got := detail(udid); got != want {
			t.Errorf("detail(%s) = %q, want %q", udid, got, want)
		}
	}
	if got := engineDetail(fakeBoot{})("emulator-5554"); got != "Waiting for Android to finish booting" {
		t.Errorf("booting emulator: %q", got)
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
