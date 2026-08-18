// Package emu discovers running Android emulators and captures their
// screens through adb — the Android counterpart of internal/sim.
package emu

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/devicelab-dev/DeviceDeck/internal/platform"
	"github.com/devicelab-dev/DeviceDeck/internal/sim"
)

// runFunc executes a command and returns stdout. The default
// implementation shells out; tests substitute fixtures.
type runFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

// Client lists emulators and takes screenshots through adb.
type Client struct {
	run runFunc
}

// NewClient builds a Client backed by real command execution.
func NewClient() *Client {
	return &Client{run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		out, err := exec.CommandContext(ctx, name, args...).Output()
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return out, nil
	}}
}

// Booted returns every running emulator, described like a simulator so
// the console and API treat both platforms uniformly.
func (c *Client) Booted(ctx context.Context) ([]sim.Device, error) {
	out, err := c.run(ctx, "adb", "devices")
	if err != nil {
		return nil, fmt.Errorf("adb devices: %w", err)
	}
	var devices []sim.Device
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != "device" || !platform.IsAndroidSerial(fields[0]) {
			continue
		}
		devices = append(devices, sim.Device{
			UDID:   fields[0],
			Name:   c.describe(ctx, fields[0]),
			OS:     "android " + c.prop(ctx, fields[0], "ro.build.version.release"),
			Booted: true,
		})
	}
	return devices, nil
}

// Screenshot captures a PNG of the emulator's screen.
func (c *Client) Screenshot(ctx context.Context, serial string) ([]byte, error) {
	return c.run(ctx, "adb", "-s", serial, "exec-out", "screencap", "-p")
}

// describe names a device by its AVD model, falling back to the serial.
func (c *Client) describe(ctx context.Context, serial string) string {
	if model := c.prop(ctx, serial, "ro.product.model"); model != "" {
		return model
	}
	return serial
}

// prop reads one system property, empty on failure — listing must not
// break because a device answered slowly.
func (c *Client) prop(ctx context.Context, serial, name string) string {
	out, err := c.run(ctx, "adb", "-s", serial, "shell", "getprop", name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
