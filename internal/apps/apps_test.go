package apps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakePlist answers Info.plist reads from a table keyed by the plist path.
func fakePlist(t *testing.T, ids map[string]string, err error) {
	t.Helper()
	prev := plistRead
	plistRead = func(plist, key string) (string, error) {
		if key != "CFBundleIdentifier" {
			t.Errorf("read key %q", key)
		}
		return ids[plist], err
	}
	t.Cleanup(func() { plistRead = prev })
}

// iosBuild makes an empty .app folder, which is all Identify stats.
func iosBuild(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(p, 0o750); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestIdentify(t *testing.T) {
	app := iosBuild(t, "TestHive.app")
	fakePlist(t, map[string]string{filepath.Join(app, "Info.plist"): "dev.devicelab.testhive"}, nil)
	got, err := Identify(app)
	if err != nil || got != (App{ID: "dev.devicelab.testhive", Name: "TestHive", Path: app, Platform: IOS}) {
		t.Fatalf("Identify(.app) = %+v, %v", got, err)
	}
	apk, _ := filepath.Abs(filepath.Join("..", "home", "android", "devicelab-android-driver.apk"))
	if got, err := Identify(apk); err != nil || got.Platform != Android || got.ID != "dev.devicelab.driver.android" {
		t.Fatalf("Identify(.apk) = %+v, %v", got, err)
	}
}

func TestIdentifyErrors(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{"Store.ipa": "x", "notes.txt": "x"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	noID := iosBuild(t, "NoID.app")
	badPlist := iosBuild(t, "Bad.app")
	fakePlist(t, map[string]string{}, nil)
	for path, want := range map[string]string{
		filepath.Join(dir, "missing.app"): "no such file",
		filepath.Join(dir, "Store.ipa"):   "device build",
		filepath.Join(dir, "notes.txt"):   "not an app build",
		noID:                              "no app id found",
	} {
		if _, err := Identify(path); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Identify(%s) err = %v, want %q", filepath.Base(path), err, want)
		}
	}
	fakePlist(t, nil, errors.New("plutil failed"))
	if _, err := Identify(badPlist); err == nil || !strings.Contains(err.Error(), "plutil failed") {
		t.Errorf("plist read failure: %v", err)
	}
}

func TestPlistReadReal(t *testing.T) {
	app := iosBuild(t, "Real.app")
	plist := `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict>` +
		`<key>CFBundleIdentifier</key><string>com.example.real</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(app, "Info.plist"), []byte(plist), 0o600); err != nil {
		t.Fatal(err)
	}
	if id, err := plistRead(filepath.Join(app, "Info.plist"), "CFBundleIdentifier"); err != nil || id != "com.example.real" {
		t.Errorf("plistRead = %q, %v", id, err)
	}
}

// fakeDevice records installs and reports which apps are already present.
type fakeDevice struct {
	present  map[string]bool
	failWith error
	installs []string
}

func (f *fakeDevice) Installed(_ context.Context, udid, appID string) bool {
	return f.present[udid+"/"+appID]
}

func (f *fakeDevice) Install(_ context.Context, udid, path string) error {
	f.installs = append(f.installs, udid+" <- "+filepath.Base(path))
	return f.failWith
}

func TestCatalogEnsure(t *testing.T) {
	app := iosBuild(t, "TestHive.app")
	fakePlist(t, map[string]string{filepath.Join(app, "Info.plist"): "dev.devicelab.testhive"}, nil)
	apk, _ := filepath.Abs(filepath.Join("..", "home", "android", "devicelab-android-driver.apk"))
	dev := &fakeDevice{present: map[string]bool{"SIM2/dev.devicelab.testhive": true}}
	c, err := NewCatalog([]string{app, apk}, dev)
	if err != nil || len(c.Apps()) != 2 {
		t.Fatalf("NewCatalog = %v, %v", c, err)
	}
	ctx := context.Background()
	for _, call := range [][2]string{
		{"SIM1", "dev.devicelab.testhive"},                // missing: install the .app
		{"SIM2", "dev.devicelab.testhive"},                // already there: nothing
		{"emulator-5554", "dev.devicelab.driver.android"}, // missing: install the .apk
		{"SIM1", "com.unregistered"},                      // not ours: nothing
	} {
		if err := c.Ensure(ctx, call[0], call[1]); err != nil {
			t.Fatalf("Ensure%v: %v", call, err)
		}
	}
	want := "SIM1 <- TestHive.app,emulator-5554 <- devicelab-android-driver.apk"
	if got := strings.Join(dev.installs, ","); got != want {
		t.Errorf("installs = %s, want %s", got, want)
	}
	// The iOS build asked for on an Android device: a clear error, no install.
	err = c.Ensure(ctx, "emulator-5554", "dev.devicelab.testhive")
	if err == nil || !strings.Contains(err.Error(), "registered only as an iOS build") {
		t.Errorf("wrong-platform launch: %v", err)
	}
	// ...unless that device already has an app by that id.
	dev.present["emulator-5556/dev.devicelab.testhive"] = true
	if err := c.Ensure(ctx, "emulator-5556", "dev.devicelab.testhive"); err != nil {
		t.Errorf("installed elsewhere-registered app: %v", err)
	}
	dev.failWith = errors.New("disk full")
	if err := c.Ensure(ctx, "SIM3", "dev.devicelab.testhive"); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("install failure: %v", err)
	}
	if _, err := NewCatalog([]string{"/nope/Missing.app"}, dev); err == nil {
		t.Error("an unreadable build must fail the catalog")
	}
}
