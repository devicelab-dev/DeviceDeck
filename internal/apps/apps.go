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
	ID       string    `json:"id"` // bundle id or package name
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Platform string    `json:"platform"`
	Version  string    `json:"version,omitempty"`  // "1.4.0 (42)": marketing version and build
	MinOS    string    `json:"minOS,omitempty"`    // "iOS 15.0" or "Android API 24"
	Arch     []string  `json:"arch,omitempty"`     // simulator slices, or the APK's native ABIs
	Size     int64     `json:"size"`               // bytes
	Modified time.Time `json:"modified"`           // the file's date: when it was built, unless copied since
	Warnings []string  `json:"warnings,omitempty"` // what may stop it running here, found up front
}

// Listed is a registered build as offered for one device: the build, and
// whether that device already has it.
type Listed struct {
	App
	Installed bool `json:"installed"`
}

// PlatformOf is the platform of a device id: Android for an adb serial,
// iOS for a simulator UDID.
func PlatformOf(udid string) string {
	if platform.IsAndroidSerial(udid) {
		return Android
	}
	return IOS
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
	apps    []App
	skipped []Skipped
	device  Device
}

// Skipped is a build found in an --app folder that cannot be used, and why.
type Skipped struct {
	Path, Reason string
}

// NewCatalog registers each --app path. A build named directly must be
// usable, or the whole call fails, so a typo stops the start rather than
// surfacing at the first launch. A folder registers every build inside it;
// one it cannot use is skipped with the reason, so a device build left in
// the folder does not block the rest.
func NewCatalog(paths []string, device Device) (*Catalog, error) {
	c := &Catalog{device: device}
	for _, p := range paths {
		if isBuildFolder(p) {
			if err := c.addFolder(p); err != nil {
				return nil, err
			}
			continue
		}
		app, err := Identify(p)
		if err != nil {
			return nil, err
		}
		c.apps = append(c.apps, app)
	}
	return c, nil
}

// isBuildFolder is a plain folder, as opposed to a .app bundle (which is a
// folder too) or a file.
func isBuildFolder(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir() && !strings.EqualFold(filepath.Ext(p), ".app")
}

// maxFolderDepth bounds the search, so pointing --app at a large tree (a
// home folder, a source checkout) stays quick.
const maxFolderDepth = 4

// addFolder registers the builds found in dir.
func (c *Catalog) addFolder(dir string) error {
	found := findBuilds(dir)
	if len(found) == 0 {
		return fmt.Errorf("--app %s: no .app or .apk builds in this folder", dir)
	}
	for _, p := range found {
		app, err := Identify(p)
		if err != nil {
			c.skipped = append(c.skipped, Skipped{Path: p, Reason: err.Error()})
			continue
		}
		c.apps = append(c.apps, app)
	}
	return nil
}

// findBuilds walks dir for .app bundles, .apk files, and .ipa files (so a
// device build is reported rather than silently missed).
func findBuilds(dir string) []string {
	var found []string
	root := strings.Count(filepath.Clean(dir), string(filepath.Separator))
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir // an unreadable folder: search the rest
		}
		switch ext := strings.ToLower(filepath.Ext(p)); {
		case d.IsDir() && ext == ".app":
			found = append(found, p)
			return filepath.SkipDir
		case d.IsDir() && strings.Count(p, string(filepath.Separator))-root >= maxFolderDepth:
			return filepath.SkipDir
		case !d.IsDir() && (ext == ".apk" || ext == ".ipa"):
			found = append(found, p)
		}
		return nil
	})
	return found
}

// Skipped lists the builds found in --app folders that could not be used.
// A nil catalog has none.
func (c *Catalog) Skipped() []Skipped {
	if c == nil {
		return nil
	}
	return c.skipped
}

// Apps lists the registered builds. A nil catalog has none.
func (c *Catalog) Apps() []App {
	if c == nil {
		return nil
	}
	return c.apps
}

// Ensure installs appID on udid if it is a registered build for that
// device's platform and the device does not have it yet. An app registered
// only for the other platform, and missing here, is an error that says so
// instead of a launch failing with the platform tool's own message. Any
// other app is left alone: an unregistered app is the caller's to install.
func (c *Catalog) Ensure(ctx context.Context, udid, appID string) error {
	want := PlatformOf(udid)
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

// For lists the builds that suit udid's platform, each marked with whether
// the device already has it: what a person picking an app to launch needs.
// A nil catalog offers nothing.
func (c *Catalog) For(ctx context.Context, udid string) []Listed {
	out := []Listed{}
	if c == nil {
		return out
	}
	want := PlatformOf(udid)
	for _, a := range c.apps {
		if a.Platform == want {
			out = append(out, Listed{App: a, Installed: c.device.Installed(ctx, udid, a.ID)})
		}
	}
	return out
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
