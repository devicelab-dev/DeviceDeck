package server

import (
	"context"
	"log/slog"

	"github.com/devicelab-dev/DeviceDeck/internal/platform"
	"github.com/devicelab-dev/DeviceDeck/internal/sim"
)

// MultiLister concatenates several device sources. A failing source is
// logged and skipped — a broken adb must not blank out the simulator
// list (or vice versa); the error surfaces only when every source fails.
type MultiLister []DeviceLister

// Booted implements DeviceLister over all sources.
func (m MultiLister) Booted(ctx context.Context) ([]sim.Device, error) {
	var devices []sim.Device
	var firstErr error
	failures := 0
	for _, l := range m {
		ds, err := l.Booted(ctx)
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
