// Package apps is the set of app builds DeviceDeck was told about with
// --app. Nothing is installed up front: when a device is asked to launch one
// of these apps and does not have it yet, the build is installed first. A
// person, a test and an agent can then all just launch the app by its id.
package apps

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/platform"
)

// Platforms an app build targets.
const (
	IOS     = "iOS"
	Android = "Android"
)

// App is one registered build and what was read from it.
type App struct {
	ID       string // bundle id or package name
	Name     string // display name for people
	Path     string
	Platform string
	Version  string   // "1.4.0 (42)": marketing version and build
	MinOS    string   // "iOS 15.0" or "Android API 24"
	Arch     []string // simulator slices, or the APK's native ABIs (none: pure Java/Kotlin)
	Size     int64
	Modified time.Time // the file's date: when it was built, unless it was copied since
	Warnings []string  // what may stop it running here, found up front
}

// hostArch is this Mac's CPU as simulator slices and Android ABIs name it.
var hostArch = map[string][2]string{"arm64": {"arm64", "arm64-v8a"}, "amd64": {"x86_64", "x86_64"}}[runtime.GOARCH]

// Identify reads a build: a .app is an iOS simulator build, a .apk an
// Android one; a device .ipa is refused with what to build instead.
func Identify(path string) (App, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return App{}, fmt.Errorf("--app %s: no such file", path)
	}
	if err != nil {
		return App{}, fmt.Errorf("--app %s: %w", path, err)
	}
	base := filepath.Base(path)
	app := App{Path: path, Name: strings.TrimSuffix(base, filepath.Ext(base)), Modified: info.ModTime()}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".app":
		err = readIOS(&app)
	case ".apk":
		err = readAndroid(&app)
	case ".ipa":
		return App{}, fmt.Errorf("%s is a device build; a simulator needs the .app from a simulator build", base)
	default:
		return App{}, fmt.Errorf("%s is not an app build; pass a .app (iOS) or .apk (Android)", base)
	}
	if err != nil {
		return App{}, fmt.Errorf("read %s: %w", base, err)
	}
	return app, nil
}

// Device is what the catalog needs from the devices: whether an app is
// there, and a way to install it.
type Device interface {
	Installed(ctx context.Context, udid, appID string) bool
	Install(ctx context.Context, udid, appPath string) error
}

// Catalog is the registered builds and the devices to install them on.
type Catalog struct {
	apps   []App
	device Device
}

// NewCatalog identifies each path. Any unreadable build fails the whole
// call, so a typo in --app stops the start rather than surfacing at the
// first launch.
func NewCatalog(paths []string, device Device) (*Catalog, error) {
	c := &Catalog{device: device}
	for _, p := range paths {
		app, err := Identify(p)
		if err != nil {
			return nil, err
		}
		c.apps = append(c.apps, app)
	}
	return c, nil
}

// Apps lists the registered builds.
func (c *Catalog) Apps() []App { return c.apps }

// Ensure installs appID on udid if it is a registered build for that
// device's platform and the device does not have it yet. An app registered
// only for the other platform, and missing here, is an error that says so
// instead of a launch failing with the platform tool's own message. Any
// other app is left alone: an unregistered app is the caller's to install.
func (c *Catalog) Ensure(ctx context.Context, udid, appID string) error {
	want := IOS
	if platform.IsAndroidSerial(udid) {
		want = Android
	}
	match, other := c.find(appID, want)
	switch {
	case match == nil && other == nil:
		return nil
	case c.device.Installed(ctx, udid, appID):
		return nil
	case match == nil:
		return fmt.Errorf("%s is registered only as an %s build (%s); %s is an %s device",
			appID, other.Platform, other.Path, udid, want)
	}
	slog.Info("installing app on first launch", "app", appID, "udid", udid, "from", match.Path)
	if err := c.device.Install(ctx, udid, match.Path); err != nil {
		return fmt.Errorf("install %s from %s: %w", appID, match.Path, err)
	}
	return nil
}

// find returns the registered build of appID for platform, and failing that
// one for another platform.
func (c *Catalog) find(appID, platform string) (match, other *App) {
	for i := range c.apps {
		switch a := &c.apps[i]; {
		case a.ID != appID:
		case a.Platform == platform:
			return a, nil
		default:
			other = a
		}
	}
	return nil, other
}
