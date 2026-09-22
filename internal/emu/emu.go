// Package emu discovers running Android emulators and captures their
// screens through adb — the Android counterpart of internal/sim.
package emu

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

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

// OpenURL opens a URL on the emulator via an ACTION_VIEW intent — an http
// link in the browser, or a custom scheme the app registered — so a flow can
// jump straight to a deep-linked screen.
func (c *Client) OpenURL(ctx context.Context, serial, rawURL string) error {
	out, err := c.run(ctx, "adb", "-s", serial, "shell", "am", "start",
		"-a", "android.intent.action.VIEW", "-d", rawURL)
	if err != nil {
		return fmt.Errorf("open %s on %s: %w", rawURL, serial, err)
	}
	// am reports an unresolved intent on stdout with a zero exit code.
	if strings.Contains(string(out), "Error:") {
		return fmt.Errorf("open %s on %s: %s", rawURL, serial, strings.TrimSpace(string(out)))
	}
	return nil
}

// LaunchApp starts appID fresh on serial, stopping it first if it is
// already running — the Android counterpart of the simulator's launch.
// monkey is used rather than `am start` because it needs only the
// package name, not the activity, which a caller naming an app by its
// bundle id does not have.
func (c *Client) LaunchApp(ctx context.Context, serial, appID string) error {
	_, _ = c.run(ctx, "adb", "-s", serial, "shell", "am", "force-stop", appID)
	out, err := c.run(ctx, "adb", "-s", serial, "shell", "monkey", "-p", appID,
		"-c", "android.intent.category.LAUNCHER", "1")
	if err != nil {
		return fmt.Errorf("launch %s on %s: %w", appID, serial, err)
	}
	// monkey reports a missing package on stdout with a zero exit code.
	if strings.Contains(string(out), "No activities found") {
		return fmt.Errorf("launch %s on %s: no launchable activity (is it installed?)", appID, serial)
	}
	return nil
}

// Install adds an APK to the emulator. -r reinstalls over an existing copy
// so re-running a build does not fail on "already installed". adb can
// report a failure on stdout with a zero exit code, so the output is
// checked as well as the process error.
func (c *Client) Install(ctx context.Context, serial, apkPath string) error {
	out, err := c.run(ctx, "adb", "-s", serial, "install", "-r", apkPath)
	if err != nil {
		return fmt.Errorf("install %s on %s: %w", apkPath, serial, err)
	}
	if strings.Contains(string(out), "Failure") {
		return fmt.Errorf("install %s on %s: %s", apkPath, serial, strings.TrimSpace(string(out)))
	}
	return nil
}

// Installed reports whether package appID is installed on the emulator;
// `pm path` prints its APK location only when it is.
func (c *Client) Installed(ctx context.Context, serial, appID string) bool {
	out, err := c.run(ctx, "adb", "-s", serial, "shell", "pm", "path", appID)
	return err == nil && strings.Contains(string(out), "package:")
}

// BootCompleted reports whether Android on serial has finished booting.
// adb lists an emulator well before that, while services such as the
// package manager are still starting and an install fails with "Can't find
// service: package".
func (c *Client) BootCompleted(ctx context.Context, serial string) bool {
	return c.prop(ctx, serial, "sys.boot_completed") == "1"
}

// bootPoll is how often WaitBooted checks; a var so tests need not wait.
var bootPoll = time.Second

// WaitBooted blocks until Android on serial has finished booting, or ctx
// ends.
func (c *Client) WaitBooted(ctx context.Context, serial string) error {
	for !c.BootCompleted(ctx, serial) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("android on %s did not finish booting: %w", serial, ctx.Err())
		case <-time.After(bootPoll):
		}
	}
	return nil
}

// Kill powers the emulator off via its console. DeviceDeck calls this on
// exit for the emulators it drove — the counterpart to the simulator's
// Shutdown — so a session leaves no detached emulator running. `adb emu
// kill` is the graceful console stop; the process was started with Setsid
// precisely so it could outlive DeviceDeck until asked to stop here.
func (c *Client) Kill(ctx context.Context, serial string) error {
	if _, err := c.run(ctx, "adb", "-s", serial, "emu", "kill"); err != nil {
		return fmt.Errorf("kill %s: %w", serial, err)
	}
	return nil
}

// ResetApp clears appID's stored data so the next launch begins as a first
// run — logged out, no cached state — the Android counterpart of the
// simulator's data wipe. `pm clear` deletes the package's data directory
// wholesale, which is exactly the blank slate the login example specs
// reach for today by uninstalling and reinstalling the apk.
func (c *Client) ResetApp(ctx context.Context, serial, appID string) error {
	out, err := c.run(ctx, "adb", "-s", serial, "shell", "pm", "clear", appID)
	if err != nil {
		return fmt.Errorf("reset %s on %s: %w", appID, serial, err)
	}
	// pm clear prints "Success" or "Failed" on stdout with a zero exit
	// code, so the text is the only signal that the package existed.
	if !strings.Contains(string(out), "Success") {
		return fmt.Errorf("reset %s on %s: %s (is it installed?)", appID, serial, strings.TrimSpace(string(out)))
	}
	return nil
}

// Screenshot captures a PNG of the emulator's screen.
func (c *Client) Screenshot(ctx context.Context, serial string) ([]byte, error) {
	return c.run(ctx, "adb", "-s", serial, "exec-out", "screencap", "-p")
}

// AVDPrefix marks a device id that names a stopped AVD rather than a
// running emulator's adb serial (an AVD has no serial until it boots).
const AVDPrefix = "avd:"

// All returns running emulators plus stopped AVDs, so the console shows
// the whole Android inventory. Stopped AVDs are identified by name with
// the avd: prefix; running ones are matched back to their AVD (via the
// emulator console) and deduplicated.
func (c *Client) All(ctx context.Context) ([]sim.Device, error) {
	running, err := c.Booted(ctx)
	if err != nil {
		return nil, err
	}
	inUse := map[string]bool{}
	for i := range running {
		if name := c.avdName(ctx, running[i].UDID); name != "" {
			inUse[name] = true
			running[i].AVD = name
		}
	}
	// Best-effort: adb working without the emulator tool installed still
	// lists running devices.
	if out, err := c.run(ctx, "emulator", "-list-avds"); err == nil {
		for _, name := range strings.Split(string(out), "\n") {
			name = strings.TrimSpace(name)
			if name == "" || inUse[name] {
				continue
			}
			running = append(running, sim.Device{
				UDID: AVDPrefix + name,
				Name: name,
				AVD:  name,
				OS:   "android",
			})
		}
	}
	return running, nil
}

// bootArgs are the emulator flags DeviceDeck boots with.
//
// -no-window is the load-bearing one. macOS throttles an occluded
// window's rendering, and the emulator's own window is occluded exactly
// when DeviceDeck is in use, because the screen the user is watching is
// the browser. Measured on an M4 Pro with the same scroll workload, the
// guest's own encoder produced 4.0 frames a second with the window
// buried and 15.8 with it frontmost — so the device stops producing
// frames for any transport to carry, and the console looks broken for a
// reason that has nothing to do with the console. Headless removes the
// window that could be occluded: 13.4 fps at the guest, 18 delivered to
// the browser, against 3 before.
var bootArgs = []string{"-no-snapshot-save", "-no-audio", "-no-window"}

// bootCommand builds the emulator invocation for an AVD id, which may
// carry the list's "avd:" prefix.
func bootCommand(id string) []string {
	return append([]string{"-avd", strings.TrimPrefix(id, AVDPrefix)}, bootArgs...)
}

// Boot launches a stopped AVD detached from this process; it appears in
// adb (and the device list) once Android finishes booting.
//
// Coverage waiver: the process spawn needs a real emulator on PATH, so it
// only runs end-to-end; the arguments it spawns with are covered via
// bootCommand.
func (c *Client) Boot(ctx context.Context, id string) error {
	args := bootCommand(id)
	name := args[1]
	cmd := exec.Command("emulator", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch emulator %s: %w", name, err)
	}
	// The emulator outlives DeviceDeck by design; release the process.
	go func() { _ = cmd.Wait() }()
	return nil
}

// avdName asks a running emulator which AVD it is.
func (c *Client) avdName(ctx context.Context, serial string) string {
	out, err := c.run(ctx, "adb", "-s", serial, "emu", "avd", "name")
	if err != nil {
		return ""
	}
	// Output is the name on the first line, then "OK".
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return strings.TrimSpace(line)
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
