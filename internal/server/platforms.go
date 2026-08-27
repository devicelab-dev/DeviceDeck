package server

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
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

// Install implements AppLauncher with platform routing.
func (r LaunchRouter) Install(ctx context.Context, udid, appPath string) error {
	if platform.IsAndroidSerial(udid) {
		return r.Android.Install(ctx, udid, appPath)
	}
	return r.IOS.Install(ctx, udid, appPath)
}

// validateAppFile rejects an install path that cannot work before the
// install is attempted, with a message that says what to do instead: a
// Simulator runs a simulator build (`.app`), not a device `.ipa`, and an
// emulator takes a `.apk` — and the extension must match the target.
func validateAppFile(udid, path string) error {
	if path == "" {
		return fmt.Errorf("appFile path is required")
	}
	android := platform.IsAndroidSerial(udid)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".apk":
		if !android {
			return fmt.Errorf("%s is an Android APK, but %s is an iOS Simulator", filepath.Base(path), udid)
		}
	case ".app":
		if android {
			return fmt.Errorf("%s is an iOS app bundle, but %s is an Android emulator", filepath.Base(path), udid)
		}
	case ".ipa":
		return fmt.Errorf("%s is a device build (.ipa); an iOS Simulator needs the .app (a simulator build)", filepath.Base(path))
	default:
		return fmt.Errorf("unsupported app file %q — use a .app (iOS Simulator) or .apk (Android emulator)", filepath.Base(path))
	}
	return nil
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
