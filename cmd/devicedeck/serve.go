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
	"syscall"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/input"
	"github.com/devicelab-dev/DeviceDeck/internal/runner"
	"github.com/devicelab-dev/DeviceDeck/internal/server"
	"github.com/devicelab-dev/DeviceDeck/internal/sim"
)

// runServe starts the DeviceDeck HTTP server and blocks until SIGINT/SIGTERM.
//
// Coverage waiver: runServe is process-lifecycle wiring (real listener,
// signals, real backends) verified by running the server; unit tests cover
// resolveSidecar and everything behind the injected interfaces.
func runServe(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := flags.String("addr", "127.0.0.1:8787", "listen address")
	sidecarPath := flags.String("sidecar", "", "path to devicedeck-hid (default: auto-discover)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	binPath, err := resolveSidecar(*sidecarPath)
	if err != nil {
		return err
	}
	defaultRunnerHome()

	inputs := input.NewManager(binPath)
	engines := runner.NewEngines()
	srv := server.New(sim.NewClient(), sim.NewClient(), inputs, engines)
	httpServer := &http.Server{Addr: *addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}

	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()
	slog.Info("devicedeck serving", "addr", *addr, "sidecar", binPath)

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
	engines.StopAll(ctx)
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

// resolveSidecar finds the devicedeck-hid binary: explicit flag, then the
// DEVICEDECK_HID env var, then next to this executable, then the local
// Swift build output (developer setup).
func resolveSidecar(explicit string) (string, error) {
	candidates := []string{explicit, os.Getenv("DEVICEDECK_HID")}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "devicedeck-hid"))
	}
	candidates = append(candidates, "sidecar/.build/release/devicedeck-hid")
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("devicedeck-hid not found; build it with `make sidecar` or pass --sidecar")
}
