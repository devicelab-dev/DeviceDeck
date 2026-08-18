// Package sim discovers iOS simulators and captures their screens via
// simctl. Command execution is injected so tests run without Xcode.
package sim

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Device is one simulator known to CoreSimulator.
type Device struct {
	UDID   string `json:"udid"`
	Name   string `json:"name"`
	OS     string `json:"os"`
	Booted bool   `json:"booted"`
}

// runFunc executes a command and returns stdout. The default implementation
// shells out; tests substitute fixtures.
type runFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

// Client lists devices and takes screenshots through simctl.
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

// All returns every available simulator, booted or not — the console
// shows the whole inventory so users see what they could boot, not just
// what already runs.
func (c *Client) All(ctx context.Context) ([]Device, error) {
	return c.list(ctx)
}

// Boot starts a stopped simulator and brings the Simulator app forward
// so its screen actually renders (framebuffer capture needs a running
// render server).
func (c *Client) Boot(ctx context.Context, udid string) error {
	if _, err := c.run(ctx, "xcrun", "simctl", "boot", udid); err != nil {
		return err
	}
	_, err := c.run(ctx, "open", "-a", "Simulator")
	return err
}

// Booted returns every currently booted simulator.
func (c *Client) Booted(ctx context.Context) ([]Device, error) {
	all, err := c.list(ctx)
	if err != nil {
		return nil, err
	}
	var booted []Device
	for _, d := range all {
		if d.Booted {
			booted = append(booted, d)
		}
	}
	return booted, nil
}

// Screenshot captures a PNG of the device's screen. simctl only writes to
// files or stdout; "-" selects stdout so no temp file is needed.
func (c *Client) Screenshot(ctx context.Context, udid string) ([]byte, error) {
	return c.run(ctx, "xcrun", "simctl", "io", udid, "screenshot", "--type=png", "-")
}

func (c *Client) list(ctx context.Context) ([]Device, error) {
	out, err := c.run(ctx, "xcrun", "simctl", "list", "devices", "-j")
	if err != nil {
		return nil, err
	}
	var payload struct {
		Devices map[string][]struct {
			UDID        string `json:"udid"`
			Name        string `json:"name"`
			State       string `json:"state"`
			IsAvailable bool   `json:"isAvailable"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, fmt.Errorf("parse simctl device list: %w", err)
	}
	var devices []Device
	for runtime, list := range payload.Devices {
		for _, d := range list {
			if !d.IsAvailable {
				continue
			}
			devices = append(devices, Device{
				UDID:   d.UDID,
				Name:   d.Name,
				OS:     runtimeName(runtime),
				Booted: d.State == "Booted",
			})
		}
	}
	return devices, nil
}

// runtimeName turns "com.apple.CoreSimulator.SimRuntime.iOS-18-6" into
// "iOS 18.6". Unrecognized identifiers pass through unchanged.
func runtimeName(id string) string {
	const prefix = "com.apple.CoreSimulator.SimRuntime."
	if !strings.HasPrefix(id, prefix) {
		return id
	}
	rest := strings.TrimPrefix(id, prefix)
	platform, version, found := strings.Cut(rest, "-")
	if !found {
		return rest
	}
	return platform + " " + strings.ReplaceAll(version, "-", ".")
}
