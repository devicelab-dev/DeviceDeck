package main

import (
	"context"
	"flag"
	"fmt"
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
	"github.com/devicelab-dev/DeviceDeck/internal/input"
	"github.com/devicelab-dev/DeviceDeck/internal/platform"
	"github.com/devicelab-dev/DeviceDeck/internal/runner"
	"github.com/devicelab-dev/DeviceDeck/internal/server"
	"github.com/devicelab-dev/DeviceDeck/internal/sim"
	"github.com/devicelab-dev/DeviceDeck/internal/video"
	"github.com/devicelab-dev/DeviceDeck/internal/web"
)

// runServe starts the DeviceDeck HTTP server and blocks until SIGINT/SIGTERM.
//
// Coverage waiver: runServe is process-lifecycle wiring (real listener,
// signals, real backends) verified by running the server; unit tests cover
// resolveBinary, defaultRunnerHome, and everything behind the injected
// interfaces.
func runServe(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := flags.String("addr", "127.0.0.1:8787", "listen address")
	hidPath := flags.String("sidecar", "", "path to devicedeck-hid (default: auto-discover)")
	videoPath := flags.String("video-sidecar", "", "path to devicedeck-video (default: auto-discover)")
	fps := flags.Int("fps", 30, "video capture frame rate")
	keepDevices := flags.Bool("keep-devices", false,
		"leave the Android emulators DeviceDeck started running on exit (iOS simulators are shut down by the test runner regardless)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	// DEVICEDECK_LOG=debug surfaces per-event diagnostics (input
	// translation, engine internals) that are too chatty for normal runs.
	if os.Getenv("DEVICEDECK_LOG") == "debug" {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}
	hidBin, err := resolveBinary("devicedeck-hid", *hidPath)
	if err != nil {
		return err
	}
	videoBin, err := resolveBinary("devicedeck-video", *videoPath)
	if err != nil {
		return err
	}
	defaultRunnerHome()

	inputs := input.NewManager(hidBin)
	videos := video.NewManager(videoBin, *fps)
	engines := runner.NewEngines()
	captures := capture.NewService(engines)
	// Android input rides the tree engine's driver session; the router
	// picks the backend per device.
	frames := input.NewRouter(inputs, func(ctx context.Context, udid string) (input.AndroidInjector, error) {
		return engines.AndroidInjector(ctx, udid)
	})
	simClient, emuClient := sim.NewClient(), emu.NewClient()
	srv := server.New(
		server.MultiLister{simClient, emuClient},
		server.BootRouter{IOS: simClient, Android: emuClient},
		server.LaunchRouter{IOS: simClient, Android: emuClient},
		server.ScreenshotRouter{IOS: simClient, Android: emuClient},
		frames, engines, videos, captures)
	srv.SetConsole(web.Handler(srv.FirstTree))
	httpServer := &http.Server{Addr: *addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}

	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()
	slog.Info("devicedeck serving", "addr", *addr, "console", "http://"+*addr,
		"hid", hidBin, "video", videoBin)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case sig := <-stop:
		slog.Info("shutting down", "signal", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	inputs.CloseAll()
	videos.CloseAll()
	// Stopping an iOS engine shuts its simulator down — killing xcodebuild
	// tears the test session down with it — so the runner already cleans
	// those up. Android emulators are detached and outlive that, so they
	// are the ones DeviceDeck must power off itself: capture the driven
	// devices before StopAll clears them, then kill the emulators among
	// them. --keep-devices leaves those running; iOS is the runner's either
	// way.
	driven := engines.ActiveUDIDs()
	engines.StopAll(ctx)
	if !*keepDevices {
		for _, udid := range driven {
			if !platform.IsAndroidSerial(udid) {
				continue
			}
			if err := emuClient.Kill(ctx, udid); err != nil {
				slog.Warn("emulator shutdown on exit", "serial", udid, "err", err)
			}
		}
	}
	return nil
}

// defaultRunnerHome points the imported maestro-runner packages at the
// user's maestro-runner install (~/.maestro-runner) when the env var is
// unset. Without it, the runner's home resolution falls back to our cwd
// and the vendored XCUITest runner source is never found.
func defaultRunnerHome() {
	if os.Getenv("MAESTRO_RUNNER_HOME") != "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	candidate := filepath.Join(home, ".maestro-runner")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		os.Setenv("MAESTRO_RUNNER_HOME", candidate)
	}
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
