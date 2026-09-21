// Package sim discovers iOS simulators and captures their screens via
// simctl. Command execution is injected so tests run without Xcode.
package sim

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
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

// OpenURL opens a URL on the simulator — an https link in Safari, or a
// custom scheme routed to the app that registered it. It is how a flow jumps
// straight to a deep-linked screen instead of navigating there by hand.
func (c *Client) OpenURL(ctx context.Context, udid, rawURL string) error {
	if _, err := c.run(ctx, "xcrun", "simctl", "openurl", udid, rawURL); err != nil {
		return fmt.Errorf("open %s on %s: %w", rawURL, udid, err)
	}
	return nil
}

// LaunchApp starts appID fresh on udid, terminating it first if it is
// already running. Fresh rather than foreground: a flow — or an example
// spec — that assumes it begins at the app's first screen is otherwise
// at the mercy of whatever the last session left behind.
func (c *Client) LaunchApp(ctx context.Context, udid, appID string) error {
	// A terminate failure means it was not running, which is the state
	// we wanted anyway.
	_, _ = c.run(ctx, "xcrun", "simctl", "terminate", udid, appID)
	if _, err := c.run(ctx, "xcrun", "simctl", "launch", udid, appID); err != nil {
		return fmt.Errorf("launch %s on %s: %w", appID, udid, err)
	}
	return nil
}

// ResetApp wipes appID's stored data so the next launch begins as a first
// run — logged out, no cart, no cached state. simctl has no clear-data
// command, and uninstall/reinstall would need the .app the server does
// not hold, so the app's data container is emptied directly: for a React
// Native app the login token lives there (AsyncStorage under Library),
// not in the keychain, so this is enough (measured on TestHive). The
// container's own directories are left for iOS to repopulate at launch;
// only Library, Documents and tmp are cleared, never system-managed
// SystemData.
func (c *Client) ResetApp(ctx context.Context, udid, appID string) error {
	_, _ = c.run(ctx, "xcrun", "simctl", "terminate", udid, appID)
	out, err := c.run(ctx, "xcrun", "simctl", "get_app_container", udid, appID, "data")
	if err != nil {
		return fmt.Errorf("locate %s data on %s: %w", appID, udid, err)
	}
	dir := strings.TrimSpace(string(out))
	// A missing path would turn the wipe below into `rm -rf /Library`;
	// refuse rather than risk it. get_app_container prints the absolute
	// container path, so anything else means the app is not installed.
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("reset %s on %s: no data container (is it installed?)", appID, udid)
	}
	args := []string{"-rf"}
	for _, sub := range []string{"Library", "Documents", "tmp"} {
		args = append(args, filepath.Join(dir, sub))
	}
	if _, err := c.run(ctx, "rm", args...); err != nil {
		return fmt.Errorf("reset %s on %s: %w", appID, udid, err)
	}
	return nil
}

// Install adds an app bundle to the simulator. It takes a `.app` — a
// simulator build — not a device `.ipa`: an `.ipa` carries an on-device
// (arm64) slice that will not run on a simulator, and simctl rejects it.
// The caller validates the extension and gives that guidance; this just
// runs the install.
func (c *Client) Install(ctx context.Context, udid, appPath string) error {
	if _, err := c.run(ctx, "xcrun", "simctl", "install", udid, appPath); err != nil {
		return fmt.Errorf("install %s on %s: %w", appPath, udid, err)
	}
	return nil
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
