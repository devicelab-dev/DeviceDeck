package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/capture"
	"github.com/devicelab-dev/DeviceDeck/internal/emu"
	"github.com/devicelab-dev/DeviceDeck/internal/home"
	"github.com/devicelab-dev/DeviceDeck/internal/input"
	"github.com/devicelab-dev/DeviceDeck/internal/platform"
	"github.com/devicelab-dev/DeviceDeck/internal/ready"
	"github.com/devicelab-dev/DeviceDeck/internal/runner"
	"github.com/devicelab-dev/DeviceDeck/internal/server"
	"github.com/devicelab-dev/DeviceDeck/internal/sim"
	"github.com/devicelab-dev/DeviceDeck/internal/video"
	"github.com/devicelab-dev/DeviceDeck/internal/web"
)

// serveFlags is what `serve` was asked to do, parsed once so the wiring in
// runServe reads as wiring.
type serveFlags struct {
	addr, hidPath, videoPath, readyApp string
	fps                                int
	keepDevices, ready                 bool
}

// parseServeFlags parses serve's arguments into serveFlags.
func parseServeFlags(args []string) (*serveFlags, error) {
	f := &serveFlags{}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.StringVar(&f.addr, "addr", "127.0.0.1:8787", "listen address")
	flags.StringVar(&f.hidPath, "sidecar", "", "path to devicedeck-hid (default: auto-discover)")
	flags.StringVar(&f.videoPath, "video-sidecar", "", "path to devicedeck-video (default: auto-discover)")
	flags.IntVar(&f.fps, "fps", 30, "video capture frame rate")
	flags.BoolVar(&f.keepDevices, "keep-devices", false,
		"leave the Android emulators DeviceDeck started running on exit (iOS simulators are shut down by the test runner regardless)")
	flags.BoolVar(&f.ready, "ready", false,
		"resolve a device (prefer booted, else newest iOS runtime >= 26.2), bring it up, and print a machine-readable status object")
	flags.StringVar(&f.readyApp, "app", "",
		"with --ready: an app to launch on the resolved device — a bundle id, or a .app/.apk path to install first")
	if err := flags.Parse(args); err != nil {
		return nil, err
	}
	return f, nil
}

// runServe starts the DeviceDeck HTTP server and blocks until SIGINT/SIGTERM.
//
// Coverage waiver: runServe is process-lifecycle wiring (real listener,
// signals, real backends) verified by running the server; unit tests cover
// parseServeFlags, resolveBinary, prepareHome, bringReady, waitForExit,
// powerOffAndroid, and everything behind the injected interfaces.
func runServe(args []string) error {
	opts, err := parseServeFlags(args)
	if err != nil {
		return err
	}
	// DEVICEDECK_LOG=debug surfaces per-event diagnostics (input
	// translation, engine internals) that are too chatty for normal runs.
	if os.Getenv("DEVICEDECK_LOG") == "debug" {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}
	hidBin, err := resolveBinary("devicedeck-hid", opts.hidPath)
	if err != nil {
		return err
	}
	videoBin, err := resolveBinary("devicedeck-video", opts.videoPath)
	if err != nil {
		return err
	}
	if err := prepareHome(); err != nil {
		return err
	}

	st := buildStack(hidBin, videoBin, opts.fps)
	httpServer := &http.Server{Addr: opts.addr, Handler: st.srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()
	slog.Info("devicedeck serving", "addr", opts.addr, "console", "http://"+opts.addr,
		"hid", hidBin, "video", videoBin)

	if opts.ready {
		bringReady(context.Background(), st.devices, st.boots, st.launches, "http://"+opts.addr, opts.readyApp, os.Stdout)
	}
	if err := waitForExit(errCh); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	st.inputs.CloseAll()
	st.videos.CloseAll()
	// Capture the driven devices before StopAll clears them; the Android
	// ones are DeviceDeck's to power off.
	driven := st.engines.ActiveUDIDs()
	st.engines.StopAll(ctx)
	if !opts.keepDevices {
		powerOffAndroid(ctx, driven, st.emu)
	}
	return nil
}

// stack is everything serve wires together and must tear down again: the
// sidecar managers, the tree engines, the platform clients and the routers
// that pick a client per device. --ready reuses the routers so it drives a
// device exactly the way the HTTP API would.
type stack struct {
	inputs   *input.Manager
	videos   *video.Manager
	engines  *runner.Engines
	emu      *emu.Client
	devices  server.MultiLister
	boots    server.BootRouter
	launches server.LaunchRouter
	srv      *server.Server
}

// androidInjector adapts the engines' concrete Android engine to the input
// router's injector interface; Go will not convert the method value itself.
//
// Coverage waiver: the only way through is starting a real Android engine
// (adb + the devicelab driver), which is exercised end-to-end, not in unit
// tests.
func (st *stack) androidInjector(ctx context.Context, udid string) (input.AndroidInjector, error) {
	return st.engines.AndroidInjector(ctx, udid)
}

// buildStack constructs the stack without starting any process: the
// managers spawn their sidecars lazily, on the first device that needs one.
func buildStack(hidBin, videoBin string, fps int) *stack {
	st := &stack{
		inputs:  input.NewManager(hidBin),
		videos:  video.NewManager(videoBin, fps),
		engines: runner.NewEngines(),
		emu:     emu.NewClient(),
	}
	simClient := sim.NewClient()
	st.devices = server.MultiLister{simClient, st.emu}
	st.boots = server.BootRouter{IOS: simClient, Android: st.emu}
	st.launches = server.LaunchRouter{IOS: simClient, Android: st.emu}
	// Android input rides the tree engine's driver session; the router
	// picks the backend per device.
	frames := input.NewRouter(st.inputs, st.androidInjector)
	st.srv = server.New(st.devices, st.boots, st.launches,
		server.ScreenshotRouter{IOS: simClient, Android: st.emu},
		frames, st.engines, st.videos, capture.NewService(st.engines))
	st.srv.SetConsole(web.Handler(st.srv.FirstTree))
	return st
}

// waitForExit blocks until the listener fails or SIGINT/SIGTERM arrives. Only
// the listener failure is an error: a signal is the normal way out.
func waitForExit(errCh <-chan error) error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)
	select {
	case err := <-errCh:
		return err
	case sig := <-stop:
		slog.Info("shutting down", "signal", sig.String())
		return nil
	}
}

// androidKiller powers off an emulator DeviceDeck started.
type androidKiller interface {
	Kill(ctx context.Context, serial string) error
}

// powerOffAndroid kills the Android emulators among the driven devices.
// Stopping an iOS engine shuts its simulator down — killing xcodebuild tears
// the test session down with it — so the runner already cleans those up.
// Android emulators are detached and outlive that, so they are the ones
// DeviceDeck must power off itself. --keep-devices skips this; iOS is the
// runner's either way.
func powerOffAndroid(ctx context.Context, driven []string, emus androidKiller) {
	for _, udid := range driven {
		if !platform.IsAndroidSerial(udid) {
			continue
		}
		if err := emus.Kill(ctx, udid); err != nil {
			slog.Warn("emulator shutdown on exit", "serial", udid, "err", err)
		}
	}
}

// prepareHome gives the runner packages DeviceDeck's own home folder, with
// the Android driver written into it, before anything resolves a driver or a
// build cache. DeviceDeck never borrows another tool's install.
func prepareHome() error {
	dir, err := home.Dir()
	if err != nil {
		return err
	}
	if err := home.Prepare(dir); err != nil {
		return fmt.Errorf("prepare %s: %w", dir, err)
	}
	slog.Debug("devicedeck home", "dir", dir)
	return nil
}

// resolveBinary finds a sidecar binary: explicit flag value, then the
// DEVICEDECK_<NAME> env var, then next to this executable, then the local
// Swift build output (developer setup).
func resolveBinary(name, explicit string) (string, error) {
	envVar := strings.ToUpper(strings.ReplaceAll(name, "-", "_")) // devicedeck-hid → DEVICEDECK_HID
	// A path given deliberately must be honoured or refused, never
	// quietly replaced: falling through to a different binary than the
	// one someone named turns a typo into a mystery, and it would run
	// the wrong build without ever saying so.
	for _, named := range []struct{ source, path string }{
		{"--sidecar/--video-sidecar", explicit},
		{envVar, os.Getenv(envVar)},
	} {
		if named.path == "" {
			continue
		}
		if info, err := os.Stat(named.path); err != nil || info.IsDir() {
			return "", fmt.Errorf("%s points at %q, which is not an executable file (%s)",
				named.source, named.path, name)
		}
		return named.path, nil
	}
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), name))
	}
	candidates = append(candidates, filepath.Join("sidecar/.build/release", name))
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("%s not found; build it with `make sidecar` or pass a flag (env %s also works)", name, envVar)
}

// bringReady resolves the device `--ready` should serve, boots it if needed,
// optionally installs and launches an app, and writes one status object an
// agent can parse to decide it may start. The decision is ready.ResolveDevice;
// this carries it out through the same platform routers the server uses, so
// a booted emulator is driven by adb and a simulator by simctl.
func bringReady(ctx context.Context, devices server.DeviceLister, boots server.DeviceBooter,
	launches server.AppLauncher, baseURL, app string, out io.Writer,
) {
	chosen, reason, ok := ready.ResolveDevice(readyDevices(ctx, devices))
	var warnings []string
	appInstalled := false
	if ok && !chosen.Booted {
		if err := boots.Boot(ctx, chosen.UDID); err != nil {
			warnings = append(warnings, "boot: "+err.Error())
		}
	}
	if ok && app != "" {
		appInstalled = readyLaunch(ctx, launches, chosen.UDID, app, &warnings)
	}
	status := ready.BuildStatus(baseURL, chosen, reason, ok, readyAppID(app), appInstalled, warnings)
	b, _ := json.Marshal(status) // plain strings and bools: cannot fail
	_, _ = fmt.Fprintln(out, string(b))
}

// readyDevices lists every simulator and emulator as the resolver sees them.
// A listing failure is not fatal: the resolver then reports that nothing
// qualifies, which is the honest status when no source answered.
func readyDevices(ctx context.Context, devices server.DeviceLister) []ready.Device {
	all, err := devices.All(ctx)
	if err != nil {
		return nil
	}
	out := make([]ready.Device, 0, len(all))
	for _, d := range all {
		out = append(out, ready.Device{UDID: d.UDID, Name: d.Name, OS: d.OS, Booted: d.Booted})
	}
	return out
}

// readyLaunch carries out the --app step: a file path is installed (the
// build's own bundle id is not known here, so launch it separately by id), a
// bundle id is launched. Errors become warnings so serve still comes up with
// the device even when the app step fails. Returns whether the app is now on
// the device.
func readyLaunch(ctx context.Context, launches server.AppLauncher, udid, app string, warnings *[]string) bool {
	if isAppFile(app) {
		if err := launches.Install(ctx, udid, app); err != nil {
			*warnings = append(*warnings, "install: "+err.Error())
			return false
		}
		*warnings = append(*warnings, "installed the build; launch it by bundle id (--app <id>) to bring it to the foreground")
		return true
	}
	if err := launches.LaunchApp(ctx, udid, app); err != nil {
		*warnings = append(*warnings, "launch: "+err.Error())
	}
	return true
}

// isAppFile reports whether --app names a build to install rather than a
// bundle id to launch.
func isAppFile(app string) bool {
	return strings.HasSuffix(app, ".app") || strings.HasSuffix(app, ".apk") || strings.Contains(app, "/")
}

// readyAppID is the bundle id label for the status: a real bundle id passes
// through, a file path has none to report.
func readyAppID(app string) string {
	if isAppFile(app) {
		return ""
	}
	return app
}
