package apps

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// command runs a macOS tool and returns its output; replaced in tests.
var command = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// plist is the part of an iOS Info.plist DeviceDeck reads.
type plist struct {
	ID          string   `json:"CFBundleIdentifier"`
	DisplayName string   `json:"CFBundleDisplayName"`
	BundleName  string   `json:"CFBundleName"`
	Version     string   `json:"CFBundleShortVersionString"`
	Build       string   `json:"CFBundleVersion"`
	MinOS       string   `json:"MinimumOSVersion"`
	Executable  string   `json:"CFBundleExecutable"`
	Platforms   []string `json:"CFBundleSupportedPlatforms"`
}

// readIOS fills app from a .app bundle: its Info.plist (via plutil, which
// reads binary plists), the executable's slices (via lipo), and its size.
// A device build carries iPhoneOS where a simulator build has
// iPhoneSimulator; installing it would fail, so it is refused here.
func readIOS(app *App) error {
	p, err := readPlist(filepath.Join(app.Path, "Info.plist"))
	if err != nil {
		return err
	}
	if len(p.Platforms) > 0 && !slices.Contains(p.Platforms, "iPhoneSimulator") {
		return fmt.Errorf("built for %s, not the simulator; build for an iOS Simulator destination", strings.Join(p.Platforms, ", "))
	}
	app.Platform, app.ID, app.Version = IOS, p.ID, version(p.Version, p.Build)
	app.Name = firstOf(p.DisplayName, p.BundleName, app.Name)
	if p.MinOS != "" {
		app.MinOS = "iOS " + p.MinOS
	}
	if archs, err := command("lipo", "-archs", filepath.Join(app.Path, p.Executable)); err == nil {
		app.Arch = strings.Fields(string(archs))
	}
	if len(app.Arch) > 0 && !slices.Contains(app.Arch, hostArch[0]) {
		app.Warnings = append(app.Warnings, fmt.Sprintf("no %s slice (has %s); it will not run on this Mac's simulators",
			hostArch[0], strings.Join(app.Arch, ", ")))
	}
	app.Size = treeSize(app.Path)
	return nil
}

// readPlist reads an Info.plist through plutil, which handles the binary
// plists Xcode writes.
func readPlist(path string) (plist, error) {
	var p plist
	out, err := command("plutil", "-convert", "json", "-o", "-", path)
	if err == nil {
		err = json.Unmarshal(out, &p)
	}
	if err != nil {
		return p, fmt.Errorf("read Info.plist: %w", err)
	}
	if p.ID == "" {
		return p, errors.New("no bundle id in Info.plist")
	}
	return p, nil
}

// readAndroid fills app from an APK: the manifest, and the native ABIs its
// lib/ folders ship. An APK with native code but none for this Mac's
// emulators installs and then crashes, so that is flagged up front.
func readAndroid(app *App) error {
	zr, err := zip.OpenReader(app.Path)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	m, err := readManifest(&zr.Reader)
	if err != nil {
		return err
	}
	app.Platform, app.ID, app.Version = Android, m.Package, version(m.VersionName, m.VersionCode)
	if m.MinSDK != "" {
		app.MinOS = "Android API " + m.MinSDK
	}
	app.Arch = nativeABIs(&zr.Reader)
	if len(app.Arch) > 0 && !slices.Contains(app.Arch, hostArch[1]) {
		app.Warnings = append(app.Warnings, fmt.Sprintf("native code only for %s; this Mac's emulators need %s",
			strings.Join(app.Arch, ", "), hostArch[1]))
	}
	app.Size = treeSize(app.Path)
	return nil
}

// nativeABIs lists the lib/<abi>/ folders in an APK, sorted.
func nativeABIs(zr *zip.Reader) []string {
	seen := map[string]bool{}
	for _, f := range zr.File {
		if parts := strings.SplitN(f.Name, "/", 3); len(parts) == 3 && parts[0] == "lib" {
			seen[parts[1]] = true
		}
	}
	abis := make([]string, 0, len(seen))
	for abi := range seen {
		abis = append(abis, abi)
	}
	sort.Strings(abis)
	return abis
}

// treeSize is a file's size, or the total of a bundle folder's files.
func treeSize(path string) int64 {
	var total int64
	// Unreadable entries are skipped: a size is a hint, not worth failing on.
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// version joins a marketing version and a build number: "1.4.0 (42)".
func version(name, build string) string {
	switch {
	case name == "" || name == build:
		return build
	case build == "":
		return name
	}
	return name + " (" + build + ")"
}

func firstOf(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
