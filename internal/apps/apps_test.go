package apps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTools answers plutil and lipo; lipo's output is archs, and an empty
// plist means plutil fails.
func fakeTools(t *testing.T, plistJSON, archs string) {
	t.Helper()
	prev := command
	command = func(name string, args ...string) ([]byte, error) {
		switch {
		case name == "plutil" && plistJSON != "":
			return []byte(plistJSON), nil
		case name == "lipo" && archs != "":
			return []byte(archs + "\n"), nil
		}
		return nil, errors.New(name + " failed")
	}
	t.Cleanup(func() { command = prev })
}

// iosBuild makes a .app folder with one file, so it has a size.
func iosBuild(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(p, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "Info.plist"), []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const simPlist = `{"CFBundleIdentifier":"dev.devicelab.testhive","CFBundleDisplayName":"Test Hive",` +
	`"CFBundleShortVersionString":"1.4.0","CFBundleVersion":"42","MinimumOSVersion":"16.6",` +
	`"CFBundleExecutable":"TestHive","CFBundleSupportedPlatforms":["iPhoneSimulator"]}`

func TestIdentifyIOS(t *testing.T) {
	app := iosBuild(t, "TestHive.app")
	fakeTools(t, simPlist, "x86_64 "+hostArch[0])
	got, err := Identify(app)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "dev.devicelab.testhive" || got.Name != "Test Hive" || got.Version != "1.4.0 (42)" ||
		got.MinOS != "iOS 16.6" || got.Platform != IOS || got.Size != 5 || len(got.Warnings) != 0 {
		t.Errorf("Identify = %+v", got)
	}
}

func TestIdentifyIOSWarningsAndFallbacks(t *testing.T) {
	app := iosBuild(t, "Legacy.app")
	fakeTools(t, `{"CFBundleIdentifier":"com.legacy","CFBundleName":"Legacy","CFBundleVersion":"7"}`, "i386")
	got, err := Identify(app)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Legacy" || got.Version != "7" || got.MinOS != "" || len(got.Warnings) != 1 ||
		!strings.Contains(got.Warnings[0], "no "+hostArch[0]+" slice") {
		t.Errorf("Identify = %+v", got)
	}
	fakeTools(t, `{"CFBundleIdentifier":"com.nolipo"}`, "")
	if got, err := Identify(app); err != nil || got.Name != "Legacy" || len(got.Arch) != 0 || len(got.Warnings) != 0 {
		t.Errorf("no lipo answer = %+v, %v", got, err)
	}
}

func TestIdentifyIOSRefusals(t *testing.T) {
	app := iosBuild(t, "Device.app")
	for plist, want := range map[string]string{
		`{"CFBundleIdentifier":"com.x","CFBundleSupportedPlatforms":["iPhoneOS"]}`: "built for iPhoneOS, not the simulator",
		`{"CFBundleName":"NoID"}`: "no bundle id in Info.plist",
		`not json`:                "read Info.plist",
		``:                        "plutil failed",
	} {
		fakeTools(t, plist, "")
		if _, err := Identify(app); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("plist %.30q: err = %v, want %q", plist, err, want)
		}
	}
}

func TestIdentifyAndroid(t *testing.T) {
	got, err := Identify(writeAPK(t, testManifest(true), "lib/"+hostArch[1]+"/libx.so", "lib/x86/libx.so", "classes.dex"))
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "com.example.app" || got.Version != "2.3.1 (42)" || got.MinOS != "Android API 24" ||
		got.Platform != Android || len(got.Arch) != 2 || len(got.Warnings) != 0 || got.Size == 0 {
		t.Errorf("Identify = %+v", got)
	}
	armOnly, err := Identify(writeAPK(t, testManifest(true), "lib/armeabi-v7a/libx.so"))
	if err != nil || len(armOnly.Warnings) != 1 || !strings.Contains(armOnly.Warnings[0], "native code only for armeabi-v7a") {
		t.Errorf("32-bit ARM only = %+v, %v", armOnly, err)
	}
	pkgOnly := axml([]string{"manifest", "package", "com.bare"}, true, tag{0, []attr{{1, 2, 0, 0}}})
	if bare, err := Identify(writeAPK(t, pkgOnly)); err != nil || bare.MinOS != "" || bare.Version != "" {
		t.Errorf("bare manifest = %+v, %v", bare, err)
	}
}

func TestIdentifyRealDriverAPK(t *testing.T) {
	got, err := Identify(filepath.Join("..", "home", "android", "devicelab-android-driver.apk"))
	if err != nil || got.ID != "dev.devicelab.driver.android" || got.MinOS != "Android API 21" {
		t.Errorf("driver APK = %+v, %v", got, err)
	}
}

func TestIdentifyErrors(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{"Store.ipa": "x", "notes.txt": "x", "broken.apk": "not a zip"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	unreadable := filepath.Join(dir, "locked", "App.app")
	if err := os.MkdirAll(unreadable, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(unreadable), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(unreadable), 0o750) })
	for path, want := range map[string]string{
		filepath.Join(dir, "missing.app"): "no such file",
		unreadable:                        "permission denied",
		filepath.Join(dir, "Store.ipa"):   "device build",
		filepath.Join(dir, "notes.txt"):   "not an app build",
		filepath.Join(dir, "broken.apk"):  "read broken.apk",
		writeAPK(t, nil, "classes.dex"):   "no AndroidManifest.xml",
	} {
		if _, err := Identify(path); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Identify(%s) err = %v, want %q", filepath.Base(path), err, want)
		}
	}
}

func TestHelpers(t *testing.T) {
	for _, tc := range [][3]string{{"1.4.0", "42", "1.4.0 (42)"}, {"7", "7", "7"}, {"", "7", "7"}, {"1.0", "", "1.0"}} {
		if got := version(tc[0], tc[1]); got != tc[2] {
			t.Errorf("version(%q, %q) = %q", tc[0], tc[1], got)
		}
	}
	if firstOf("", "") != "" || firstOf("", "b", "c") != "b" {
		t.Error("firstOf")
	}
	if treeSize("/definitely/not/here") != 0 {
		t.Error("a missing path has no size")
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
	fakeTools(t, simPlist, hostArch[0])
	apk := filepath.Join("..", "home", "android", "devicelab-android-driver.apk")
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
	if got := strings.Join(dev.installs, ","); got != "SIM1 <- TestHive.app,emulator-5554 <- devicelab-android-driver.apk" {
		t.Errorf("installs = %s", got)
	}
	err = c.Ensure(ctx, "emulator-5554", "dev.devicelab.testhive")
	if err == nil || !strings.Contains(err.Error(), "registered only as an iOS build") {
		t.Errorf("wrong-platform launch: %v", err)
	}
	dev.present["emulator-5556/dev.devicelab.testhive"] = true
	if err := c.Ensure(ctx, "emulator-5556", "dev.devicelab.testhive"); err != nil {
		t.Errorf("an app already on the device: %v", err)
	}
	dev.failWith = errors.New("disk full")
	if err := c.Ensure(ctx, "SIM3", "dev.devicelab.testhive"); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("install failure: %v", err)
	}
	if _, err := NewCatalog([]string{"/nope/Missing.app"}, dev); err == nil {
		t.Error("an unreadable build must fail the catalog")
	}
}

func TestRealCommand(t *testing.T) {
	if out, err := command("echo", "ok"); err != nil || strings.TrimSpace(string(out)) != "ok" {
		t.Errorf("command = %q, %v", out, err)
	}
}
