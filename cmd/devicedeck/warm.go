package main

import (
	"context"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/apps"
	"github.com/devicelab-dev/DeviceDeck/internal/server"
)

// engineDetail describes an engine start for the console's loading screen.
// On Android the wait is the emulator's boot and then the driver install;
// the iOS agent ships prebuilt, so starting it is the whole wait.
func engineDetail(android bootChecker) func(udid string) string {
	return func(udid string) string {
		if apps.PlatformOf(udid) != apps.Android {
			return "Starting the iOS agent"
		}
		if !android.BootCompleted(context.Background(), udid) {
			return "Waiting for Android to finish booting"
		}
		return "Installing and starting the Android driver"
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
