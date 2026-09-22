package server

import (
	"context"
	"errors"
	"testing"

	"github.com/devicelab-dev/DeviceDeck/internal/sim"
)

type stubLister struct {
	devices []sim.Device
	err     error
}

func (s stubLister) Booted(context.Context) ([]sim.Device, error) { return s.devices, s.err }
func (s stubLister) All(context.Context) ([]sim.Device, error)    { return s.devices, s.err }

func TestMultiLister(t *testing.T) {
	iosDev := sim.Device{UDID: "IOS-1", Booted: true}
	androidDev := sim.Device{UDID: "emulator-5554", Booted: true}

	t.Run("concatenates sources", func(t *testing.T) {
		m := MultiLister{stubLister{devices: []sim.Device{iosDev}}, stubLister{devices: []sim.Device{androidDev}}}
		devices, err := m.Booted(context.Background())
		if err != nil || len(devices) != 2 {
			t.Fatalf("devices = %+v, err %v", devices, err)
		}
	})

	t.Run("one broken source does not hide the other", func(t *testing.T) {
		m := MultiLister{stubLister{err: errors.New("adb missing")}, stubLister{devices: []sim.Device{iosDev}}}
		devices, err := m.Booted(context.Background())
		if err != nil || len(devices) != 1 || devices[0].UDID != "IOS-1" {
			t.Fatalf("devices = %+v, err %v", devices, err)
		}
	})

	t.Run("all sources failing surfaces the error", func(t *testing.T) {
		m := MultiLister{stubLister{err: errors.New("a")}, stubLister{err: errors.New("b")}}
		if _, err := m.Booted(context.Background()); err == nil {
			t.Fatal("expected error when every source fails")
		}
	})
}

type stubShots struct{ tag string }

func (s stubShots) Screenshot(context.Context, string) ([]byte, error) { return []byte(s.tag), nil }

func TestScreenshotRouter(t *testing.T) {
	r := ScreenshotRouter{IOS: stubShots{"ios"}, Android: stubShots{"android"}}
	if png, _ := r.Screenshot(context.Background(), "emulator-5554"); string(png) != "android" {
		t.Errorf("android screenshot routed to %q", png)
	}
	if png, _ := r.Screenshot(context.Background(), "EB69B42A-0000"); string(png) != "ios" {
		t.Errorf("ios screenshot routed to %q", png)
	}
}

func TestLaunchRouter(t *testing.T) {
	ios, android := &recordingLauncher{}, &recordingLauncher{}
	r := LaunchRouter{IOS: ios, Android: android}
	if err := r.LaunchApp(context.Background(), "emulator-5554", "com.example"); err != nil {
		t.Fatal(err)
	}
	if err := r.LaunchApp(context.Background(), "EB69B42A-4763", "com.example"); err != nil {
		t.Fatal(err)
	}
	if len(android.calls) != 1 || len(ios.calls) != 1 {
		t.Errorf("routing wrong: ios=%v android=%v", ios.calls, android.calls)
	}
	// ResetApp routes by the same rule.
	if err := r.ResetApp(context.Background(), "emulator-5554", "com.example"); err != nil {
		t.Fatal(err)
	}
	if err := r.ResetApp(context.Background(), "EB69B42A-4763", "com.example"); err != nil {
		t.Fatal(err)
	}
	if len(android.resets) != 1 || len(ios.resets) != 1 {
		t.Errorf("reset routing wrong: ios=%v android=%v", ios.resets, android.resets)
	}
	// Install routes by the same rule.
	if err := r.Install(context.Background(), "emulator-5554", "/x.apk"); err != nil {
		t.Fatal(err)
	}
	if err := r.Install(context.Background(), "EB69B42A-4763", "/x.app"); err != nil {
		t.Fatal(err)
	}
	if len(android.installs) != 1 || len(ios.installs) != 1 {
		t.Errorf("install routing wrong: ios=%v android=%v", ios.installs, android.installs)
	}
	// OpenURL routes by the same rule.
	if err := r.OpenURL(context.Background(), "emulator-5554", "x://a"); err != nil {
		t.Fatal(err)
	}
	if err := r.OpenURL(context.Background(), "EB69B42A-4763", "x://b"); err != nil {
		t.Fatal(err)
	}
	if len(android.opens) != 1 || len(ios.opens) != 1 {
		t.Errorf("openurl routing wrong: ios=%v android=%v", ios.opens, android.opens)
	}
}

func TestValidateAppFile(t *testing.T) {
	const ios, android = "EB69B42A-4763", "emulator-5554"
	for _, c := range []struct{ udid, path string }{{ios, "/x/My.app"}, {android, "/x/app.apk"}} {
		if err := validateAppFile(c.udid, c.path); err != nil {
			t.Errorf("validateAppFile(%s, %s) = %v, want nil", c.udid, c.path, err)
		}
	}
	for _, c := range []struct{ udid, path string }{
		{ios, ""},              // missing path
		{ios, "/x/App.ipa"},    // device build on a simulator
		{ios, "/x/app.apk"},    // apk on iOS
		{android, "/x/My.app"}, // .app on Android
		{ios, "/x/thing.zip"},  // unknown extension
	} {
		if err := validateAppFile(c.udid, c.path); err == nil {
			t.Errorf("validateAppFile(%s, %q) = nil, want error", c.udid, c.path)
		}
	}
}

type recordingLauncher struct {
	calls    []string
	resets   []string
	installs []string
	opens    []string
}

func (l *recordingLauncher) LaunchApp(_ context.Context, udid, appID string) error {
	l.calls = append(l.calls, udid+"/"+appID)
	return nil
}

func (l *recordingLauncher) OpenURL(_ context.Context, udid, rawURL string) error {
	l.opens = append(l.opens, udid+"/"+rawURL)
	return nil
}

func (l *recordingLauncher) ResetApp(_ context.Context, udid, appID string) error {
	l.resets = append(l.resets, udid+"/"+appID)
	return nil
}

func (l *recordingLauncher) Install(_ context.Context, udid, appPath string) error {
	l.installs = append(l.installs, udid+"/"+appPath)
	return nil
}

// All fans out the same way Booted does; it is exercised separately
// because the two share only their collect() core, not their call site.
func TestMultiListerAll(t *testing.T) {
	m := MultiLister{
		stubLister{devices: []sim.Device{{UDID: "IOS-1"}}},
		stubLister{devices: []sim.Device{{UDID: "emulator-5554"}}},
	}
	devices, err := m.All(context.Background())
	if err != nil || len(devices) != 2 {
		t.Fatalf("All devices = %+v, err %v", devices, err)
	}
}

type recordingBooter struct{ calls []string }

func (b *recordingBooter) Boot(_ context.Context, id string) error {
	b.calls = append(b.calls, id)
	return nil
}

// Boot routes Android serials and avd:-prefixed AVD names to the Android
// booter and every other id to the simulator booter.
func TestBootRouter(t *testing.T) {
	tests := []struct{ id, want string }{
		{"emulator-5554", "android"},
		{"avd:Pixel_7", "android"},
		{"EB69B42A-4763", "ios"},
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			ios, android := &recordingBooter{}, &recordingBooter{}
			r := BootRouter{IOS: ios, Android: android}
			if err := r.Boot(context.Background(), tc.id); err != nil {
				t.Fatal(err)
			}
			got := "ios"
			if len(android.calls) == 1 {
				got = "android"
			}
			if got != tc.want {
				t.Errorf("id %q routed to %s, want %s", tc.id, got, tc.want)
			}
		})
	}
}

// presence answers Installed with a fixed value and records the device.
type presence struct {
	yes  bool
	seen string
}

func (p *presence) Installed(_ context.Context, udid, _ string) bool {
	p.seen = udid
	return p.yes
}

func TestInstalledRouter(t *testing.T) {
	ios, android := &presence{yes: true}, &presence{}
	r := InstalledRouter{IOS: ios, Android: android}
	if !r.Installed(context.Background(), "AAAA-BBBB", "x") || ios.seen != "AAAA-BBBB" {
		t.Error("simulator ids go to the iOS check")
	}
	if r.Installed(context.Background(), "emulator-5554", "x") || android.seen != "emulator-5554" {
		t.Error("emulator serials go to the Android check")
	}
}
