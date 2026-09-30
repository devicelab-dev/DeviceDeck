// Package home gives DeviceDeck its own home folder and the driver files the
// imported maestro-runner packages expect to find there.
//
// DeviceDeck is a standalone tool: a clean Mac with no other tool installed
// must work after one install. The runner packages locate their Android
// driver APKs and their iOS build cache under a home folder they resolve on
// first use. Left alone they fall back to the current working directory, so
// Android cannot find its driver and iOS drops a build cache wherever the
// user happened to run the command. Prepare points them at DeviceDeck's own
// folder and writes the APKs there from copies embedded in this binary, so
// the driver always matches the runner code it was built against.
package home

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// EnvHome overrides the home folder; mainly for tests and side-by-side
// installs. Unset, the home is ~/.devicedeck.
const EnvHome = "DEVICEDECK_HOME"

// runnerHomeEnv is the variable the maestro-runner packages read their home
// from. DeviceDeck always sets it, so an unrelated maestro-runner install on
// the same machine is never borrowed.
const runnerHomeEnv = "MAESTRO_RUNNER_HOME"

// androidDrivers holds the devicelab Android driver and its instrumentation
// APK, copied from the pinned maestro-runner module by `make drivers`. A test
// fails when they drift from that module, so a dependency bump cannot ship
// stale APKs.
//
//go:embed android/*.apk
var androidDrivers embed.FS

// iosAgent holds the prebuilt devicelab iOS agent (the XCUITest apps, their
// xctestrun and manifest), copied from the pinned maestro-runner module by
// `make drivers`. The runner looks for it under its home's
// drivers/ios/devicelab-ios-agent/simulator; `all:` keeps the bundles'
// underscore-prefixed folders (_CodeSignature, _Testing_Foundation).
//
//go:embed all:ios
var iosAgent embed.FS

// agentRoot is the agent's folder under the drivers folder, as embedded.
const agentRoot = "ios/devicelab-ios-agent"

// Dir resolves DeviceDeck's home folder: $DEVICEDECK_HOME if set, else
// ~/.devicedeck. It does not create it.
func Dir() (string, error) {
	if d := os.Getenv(EnvHome); d != "" {
		return d, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w (set %s)", err, EnvHome)
	}
	return filepath.Join(h, ".devicedeck"), nil
}

// Prepare writes the embedded Android driver APKs under dir/drivers/android
// and the iOS agent under dir/drivers/ios, and points the runner packages at
// dir. It must run before any runner code resolves its home, because the
// runner caches that answer for the process.
func Prepare(dir string) error {
	drivers := filepath.Join(dir, "drivers")
	if err := installDrivers(filepath.Join(drivers, "android")); err != nil {
		return err
	}
	if err := installAgent(drivers); err != nil {
		return err
	}
	return os.Setenv(runnerHomeEnv, dir)
}

// installDrivers makes dst hold exactly the embedded APKs: each is written
// only when its bytes differ, and APKs left by an older build are removed so
// the runner's filename glob can never pick a stale one.
func installDrivers(dst string) error {
	if err := os.MkdirAll(dst, 0o750); err != nil {
		return fmt.Errorf("create driver folder: %w", err)
	}
	embedded, _ := fs.ReadDir(androidDrivers, "android") // compiled in: cannot fail
	want := map[string]bool{}
	for _, e := range embedded {
		want[e.Name()] = true
		data, _ := androidDrivers.ReadFile("android/" + e.Name())
		if err := writeIfChanged(filepath.Join(dst, e.Name()), data, 0o600); err != nil {
			return err
		}
	}
	return removeStale(dst, want)
}

// installAgent mirrors the embedded iOS agent under drivers: each file is
// written only when its bytes differ, and files an older build shipped are
// removed, so the agent's manifest and the files beside it always agree.
func installAgent(drivers string) error {
	want := map[string]bool{}
	err := fs.WalkDir(iosAgent, agentRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, _ := iosAgent.ReadFile(p) // compiled in: cannot fail
		dst := filepath.Join(drivers, filepath.FromSlash(p))
		want[strings.TrimPrefix(p, agentRoot+"/")] = true
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return fmt.Errorf("create agent folder: %w", err)
		}
		return writeIfChanged(dst, data, fileMode(data))
	})
	if err != nil {
		return err
	}
	return pruneAgent(filepath.Join(drivers, filepath.FromSlash(agentRoot)), want)
}

// machOMagic are the first four bytes of a Mach-O executable or library,
// thin (either byte order, 32 or 64 bit) or universal. An embedded file
// loses its mode, and these are the files the simulator must be able to
// execute.
var machOMagic = [][]byte{
	{0xfe, 0xed, 0xfa, 0xce}, {0xfe, 0xed, 0xfa, 0xcf}, {0xce, 0xfa, 0xed, 0xfe},
	{0xcf, 0xfa, 0xed, 0xfe}, {0xca, 0xfe, 0xba, 0xbe}, {0xbe, 0xba, 0xfe, 0xca},
}

// fileMode is 0700 for a Mach-O binary and 0600 for anything else.
func fileMode(data []byte) os.FileMode {
	for _, m := range machOMagic {
		if bytes.HasPrefix(data, m) {
			return 0o700
		}
	}
	return 0o600
}

// pruneAgent removes files under dir that this build does not ship; want
// holds the shipped files' slash paths relative to dir. It works inside an
// os.Root, so a symlink planted in the folder cannot redirect a removal
// outside it.
func pruneAgent(dir string, want map[string]bool) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open agent folder: %w", err)
	}
	defer func() { _ = root.Close() }()
	return fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || want[p] {
			return err
		}
		if err := root.Remove(p); err != nil {
			return fmt.Errorf("remove stale agent file %s: %w", p, err)
		}
		return nil
	})
}

// writeIfChanged writes data to path through a temporary file and a rename,
// so a crash mid-write never leaves a truncated driver for adb or simctl to
// install.
func writeIfChanged(path string, data []byte, mode os.FileMode) error {
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, data) {
		return nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return fmt.Errorf("write driver %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install driver %s: %w", filepath.Base(path), err)
	}
	return nil
}

// removeStale deletes APKs in dst that this build does not ship.
func removeStale(dst string, want map[string]bool) error {
	present, err := os.ReadDir(dst)
	if err != nil {
		return fmt.Errorf("read driver folder: %w", err)
	}
	for _, e := range present {
		if !strings.HasSuffix(e.Name(), ".apk") || want[e.Name()] {
			continue
		}
		if err := os.Remove(filepath.Join(dst, e.Name())); err != nil {
			return fmt.Errorf("remove stale driver %s: %w", e.Name(), err)
		}
	}
	return nil
}
