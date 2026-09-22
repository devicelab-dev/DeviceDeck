package main

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/apps"
	"github.com/devicelab-dev/DeviceDeck/internal/home"
	"github.com/devicelab-dev/DeviceDeck/internal/server"
)

// runnerCacheDir is where the iOS runner's builds are kept, under the
// DeviceDeck home; empty when the home cannot be resolved.
func runnerCacheDir() string {
	dir, err := home.Dir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "cache", "devicelab-ios-runner-builds")
}

// engineDetail describes an engine start for the console's loading screen.
// The first start for an iOS version builds the runner, which takes about a
// minute; saying so turns a long wait into an expected one.
func engineDetail(cacheDir string, devices server.DeviceLister, android bootChecker) func(udid string) string {
	return func(udid string) string {
		if apps.PlatformOf(udid) == apps.Android {
			if !android.BootCompleted(context.Background(), udid) {
				return "Waiting for Android to finish booting"
			}
			return "Installing and starting the Android driver"
		}
		ver := iosVersionOf(devices, udid)
		if ver != "" && !runnerBuilt(cacheDir, ver) {
			return "Building the iOS runner for iOS " + ver + " (first time only, about a minute)"
		}
		return "Starting the iOS runner"
	}
}

// bootChecker reports whether an emulator has finished booting.
type bootChecker interface {
	BootCompleted(ctx context.Context, serial string) bool
}

// androidWaiter waits for an emulator to finish booting.
type androidWaiter interface {
	WaitBooted(ctx context.Context, serial string) error
}

// bootThenWarm is the engine warm-up serve uses: an emulator is waited on
// until Android has finished booting, since its driver cannot be installed
// before then, and then the engine is started as for any device.
type bootThenWarm struct {
	android androidWaiter
	engines server.EngineWarmer
}

// androidBootTimeout bounds the wait for a cold emulator.
const androidBootTimeout = 3 * time.Minute

// Warm implements server.EngineWarmer.
func (b bootThenWarm) Warm(ctx context.Context, udid string) error {
	if apps.PlatformOf(udid) == apps.Android {
		wctx, cancel := context.WithTimeout(ctx, androidBootTimeout)
		defer cancel()
		if err := b.android.WaitBooted(wctx, udid); err != nil {
			return err
		}
	}
	return b.engines.Warm(ctx, udid)
}

// iosVersionOf is a simulator's iOS version ("26.2"), or "" if not listed.
func iosVersionOf(devices server.DeviceLister, udid string) string {
	all, _ := devices.All(context.Background())
	for _, d := range all {
		if d.UDID == udid {
			return strings.TrimPrefix(d.OS, "iOS ")
		}
	}
	return ""
}

// runnerBuilt reports whether a runner build for this iOS version is cached
// (the runner keeps one per version, each with its .xctestrun).
func runnerBuilt(cacheDir, ver string) bool {
	found, _ := filepath.Glob(filepath.Join(cacheDir, "sim-ios"+ver+"-*", "Build", "Products", "*.xctestrun"))
	return len(found) > 0
}
