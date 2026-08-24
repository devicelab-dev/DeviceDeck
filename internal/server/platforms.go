package server

import (
	"context"
	"log/slog"
	"strings"

	"github.com/devicelab-dev/DeviceDeck/internal/platform"
	"github.com/devicelab-dev/DeviceDeck/internal/sim"
)

// MultiLister concatenates several device sources. A failing source is
// logged and skipped — a broken adb must not blank out the simulator
// list (or vice versa); the error surfaces only when every source fails.
type MultiLister []DeviceLister

// Booted implements DeviceLister over all sources.
func (m MultiLister) Booted(ctx context.Context) ([]sim.Device, error) {
	return m.collect(ctx, func(l DeviceLister) ([]sim.Device, error) { return l.Booted(ctx) })
}

// All implements DeviceLister over all sources.
func (m MultiLister) All(ctx context.Context) ([]sim.Device, error) {
	return m.collect(ctx, func(l DeviceLister) ([]sim.Device, error) { return l.All(ctx) })
}

func (m MultiLister) collect(_ context.Context, list func(DeviceLister) ([]sim.Device, error)) ([]sim.Device, error) {
	var devices []sim.Device
	var firstErr error
	failures := 0
	for _, l := range m {
		ds, err := list(l)
		if err != nil {
			failures++
			if firstErr == nil {
				firstErr = err
			}
			slog.Debug("device listing source failed", "error", err)
			continue
		}
		devices = append(devices, ds...)
	}
	if failures == len(m) && firstErr != nil {
		return nil, firstErr
	}
	return devices, nil
}

// BootRouter picks the platform's boot backend per device id: Android
// ids are adb serials or avd:-prefixed AVD names; everything else is a
// simulator UDID.
type BootRouter struct {
	IOS     DeviceBooter
	Android DeviceBooter
}

// Boot implements DeviceBooter with platform routing.
func (r BootRouter) Boot(ctx context.Context, id string) error {
	if platform.IsAndroidSerial(id) || strings.HasPrefix(id, "avd:") {
		return r.Android.Boot(ctx, id)
	}
	return r.IOS.Boot(ctx, id)
}

// LaunchRouter picks the platform's app-launch backend per device.
type LaunchRouter struct {
	IOS     AppLauncher
	Android AppLauncher
}

// LaunchApp implements AppLauncher with platform routing.
func (r LaunchRouter) LaunchApp(ctx context.Context, udid, appID string) error {
	if platform.IsAndroidSerial(udid) {
		return r.Android.LaunchApp(ctx, udid, appID)
	}
	return r.IOS.LaunchApp(ctx, udid, appID)
}

// ResetApp implements AppLauncher with platform routing.
func (r LaunchRouter) ResetApp(ctx context.Context, udid, appID string) error {
	if platform.IsAndroidSerial(udid) {
		return r.Android.ResetApp(ctx, udid, appID)
	}
	return r.IOS.ResetApp(ctx, udid, appID)
}

// SimShutdowner powers an iOS simulator off; EmuKiller does the same for
// an Android emulator. The two verbs differ because the platforms do:
// simctl shuts down, adb kills the emulator console.
type SimShutdowner interface {
	Shutdown(ctx context.Context, udid string) error
}

// EmuKiller powers an Android emulator off via its console.
type EmuKiller interface {
	Kill(ctx context.Context, serial string) error
}

// ShutdownRouter powers a device off through the right platform backend,
// so exit cleanup can shut down the devices DeviceDeck drove without
// caring which kind each one is.
type ShutdownRouter struct {
	IOS     SimShutdowner
	Android EmuKiller
}

// Shutdown powers udid off via its platform backend.
func (r ShutdownRouter) Shutdown(ctx context.Context, udid string) error {
	if platform.IsAndroidSerial(udid) {
		return r.Android.Kill(ctx, udid)
	}
	return r.IOS.Shutdown(ctx, udid)
}

// ScreenshotRouter picks the platform's screenshot backend per device.
type ScreenshotRouter struct {
	IOS     Screenshotter
	Android Screenshotter
}

// Screenshot implements Screenshotter with platform routing.
func (r ScreenshotRouter) Screenshot(ctx context.Context, udid string) ([]byte, error) {
	if platform.IsAndroidSerial(udid) {
		return r.Android.Screenshot(ctx, udid)
	}
	return r.IOS.Screenshot(ctx, udid)
}
