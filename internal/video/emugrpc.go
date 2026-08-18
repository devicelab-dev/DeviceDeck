package video

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/devicelab-dev/DeviceDeck/internal/emugrpc"
)

// emulatorEndpoint is a running emulator's gRPC address and access token,
// read from its discovery file.
type emulatorEndpoint struct {
	addr  string
	token string
}

// discoverEndpoint resolves a serial to its gRPC endpoint. Var so tests
// can point capture at an in-process fake emulator.
var discoverEndpoint = discoverEmulator

// discoverEmulator finds the gRPC endpoint for an adb serial by matching
// the console port against the emulator's discovery files — the same
// files Android Studio's device streaming reads. The static grpc.token
// from the file authorizes even allowlist-protected methods like
// streamScreenshot.
func discoverEmulator(serial string) (emulatorEndpoint, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return emulatorEndpoint{}, err
	}
	dirs := []string{filepath.Join(home, "Library", "Caches", "TemporaryItems", "avd", "running")}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		dirs = append(dirs, filepath.Join(xdg, "avd", "running"))
	}
	return discoverEmulatorIn(dirs, serial)
}

// discoverEmulatorIn is discoverEmulator over explicit discovery dirs.
func discoverEmulatorIn(dirs []string, serial string) (emulatorEndpoint, error) {
	consolePort := strings.TrimPrefix(serial, "emulator-")
	for _, dir := range dirs {
		files, _ := filepath.Glob(filepath.Join(dir, "pid_*.ini"))
		for _, f := range files {
			kv := parseDiscoveryFile(f)
			if kv["port.serial"] != consolePort {
				continue
			}
			if kv["grpc.port"] == "" || kv["grpc.token"] == "" {
				return emulatorEndpoint{}, fmt.Errorf("discovery file %s lacks grpc.port/grpc.token", f)
			}
			return emulatorEndpoint{addr: "127.0.0.1:" + kv["grpc.port"], token: kv["grpc.token"]}, nil
		}
	}
	return emulatorEndpoint{}, fmt.Errorf("no emulator discovery file for %s", serial)
}

// parseDiscoveryFile reads the flat key=value format of pid_<n>.ini.
func parseDiscoveryFile(path string) map[string]string {
	kv := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return kv
	}
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			kv[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return kv
}

// bearerCreds injects the emulator's token as per-RPC metadata.
type bearerCreds string

// GetRequestMetadata satisfies credentials.PerRPCCredentials.
func (b bearerCreds) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + string(b)}, nil
}

// RequireTransportSecurity is false: the emulator serves loopback plaintext.
func (b bearerCreds) RequireTransportSecurity() bool { return false }

// streamViaGRPC captures via the emulator's streamScreenshot — the
// host-side path (Cuttlefish-style): no adb in the loop, no
// screenrecord 3-minute cap, and the current frame arrives immediately
// on subscribe, so static screens and late joiners need no special
// handling. Every frame ships as a self-contained TypeStill; keyframe
// requests are inherently satisfied. Returns nil on deliberate shutdown,
// an error if streaming cannot (re)start so the caller can fall back.
func (c *androidCapture) streamViaGRPC(ep emulatorEndpoint) error {
	conn, err := grpc.NewClient(ep.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(bearerCreds(ep.token)))
	if err != nil {
		return fmt.Errorf("dial emulator grpc: %w", err)
	}
	defer func() { _ = conn.Close() }()
	client := emugrpc.NewEmulatorControllerClient(conn)

	failures := 0
	for {
		err := c.pumpScreenshotStream(client)
		if c.isClosed() {
			return nil
		}
		failures++
		if failures >= 3 {
			return fmt.Errorf("emulator screenshot stream: %w", err)
		}
		time.Sleep(time.Second)
	}
}

// pumpScreenshotStream forwards one streamScreenshot session's frames.
func (c *androidCapture) pumpScreenshotStream(client emugrpc.EmulatorControllerClient) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.setCancel(cancel)
	stream, err := client.StreamScreenshot(ctx, &emugrpc.ImageFormat{Format: emugrpc.ImageFormat_PNG})
	if err != nil {
		return err
	}
	for {
		img, err := stream.Recv()
		if err != nil {
			return err
		}
		if png := img.GetImage(); len(png) > 0 {
			if err := WriteFrame(c.out, TypeStill, png); err != nil {
				return err
			}
		}
	}
}

func (c *androidCapture) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// setCancel registers how to abort the in-flight stream so a stdin EOF
// unblocks Recv immediately. Shutdown may already have happened — then
// its cancel found nothing registered, so fire it here.
func (c *androidCapture) setCancel(cancel context.CancelFunc) {
	c.mu.Lock()
	c.cancelStream = cancel
	closed := c.closed
	c.mu.Unlock()
	if closed {
		cancel()
	}
}
