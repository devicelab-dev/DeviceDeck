package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// mainArgsEnv carries the argument list for a re-executed test binary that
// should run main() instead of the tests.
const mainArgsEnv = "DEVICEDECK_TEST_MAIN_ARGS"

// TestMain lets a test run the real entry point — os.Args, os.Exit, signal
// handling and all — in a child process. Coverage from the child is merged
// into the parent's profile by `go test -cover`.
func TestMain(m *testing.M) {
	if raw := os.Getenv(mainArgsEnv); raw != "" {
		var args []string
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			os.Exit(99)
		}
		os.Args = append([]string{"devicedeck"}, args...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// child is a devicedeck process running the real main. Its PATH holds no
// simctl, adb or emulator, so it can see and touch no device; its home is a
// temp dir; and its update check is pointed at a dead proxy.
type child struct {
	cmd  *exec.Cmd
	out  bytes.Buffer
	home string
}

func newChild(t *testing.T, args ...string) *child {
	t.Helper()
	raw, _ := json.Marshal(args)
	c := &child{home: t.TempDir()}
	c.cmd = exec.Command(os.Args[0], os.Args[1:]...)
	c.cmd.Env = append(os.Environ(),
		mainArgsEnv+"="+string(raw),
		"PATH="+t.TempDir()+":/bin",
		"HOME="+c.home,
		"DEVICEDECK_HOME="+filepath.Join(c.home, ".devicedeck"),
		"HTTP_PROXY=http://127.0.0.1:9", "HTTPS_PROXY=http://127.0.0.1:9",
	)
	c.cmd.Stdout, c.cmd.Stderr = &c.out, &c.out
	return c
}

// run waits for the child and returns its exit code.
func (c *child) run(t *testing.T) int {
	t.Helper()
	err := c.cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	if err != nil {
		t.Fatalf("run child: %v", err)
	}
	return 0
}

// fakeSidecars makes two executable stand-ins; serve only checks they exist
// until a device asks for a stream.
func fakeSidecars(t *testing.T) (hid, video string) {
	t.Helper()
	dir := t.TempDir()
	hid, video = filepath.Join(dir, "devicedeck-hid"), filepath.Join(dir, "devicedeck-video")
	for _, p := range []string{hid, video} {
		if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return hid, video
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func TestMainCommands(t *testing.T) {
	hid, video := fakeSidecars(t)
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = busy.Close() }()

	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
	}{
		{"version", []string{"version"}, 0, "devicedeck"},
		{"help", []string{"help"}, 0, "devicedeck"},
		{"doctor", []string{"doctor"}, 0, "devicedeck"},
		{"unknown command", []string{"frobnicate"}, 2, `unknown command "frobnicate"`},
		{"mcp serves until stdin closes", []string{"mcp"}, 0, ""},
		{"mcp bad flag", []string{"mcp", "--nope"}, 1, "devicedeck:"},
		{"serve bad flag", []string{"serve", "--nope"}, 1, "devicedeck:"},
		{"serve missing sidecar", []string{"serve", "--addr", freeAddr(t), "--sidecar", "/nonexistent/devicedeck-hid"},
			1, "not an executable file"},
		{"serve missing app build", []string{"serve", "--addr", freeAddr(t), "--sidecar", hid, "--video-sidecar", video,
			"--app", "/nonexistent/My.app"}, 1, "devicedeck:"},
		{"serve port in use", []string{"serve", "--addr", busy.Addr().String(), "--sidecar", hid, "--video-sidecar", video},
			1, "listen on"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newChild(t, tt.args...)
			if code := c.run(t); code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; output:\n%s", code, tt.wantCode, c.out.String())
			}
			if !strings.Contains(c.out.String(), tt.wantOut) {
				t.Errorf("output lacks %q:\n%s", tt.wantOut, c.out.String())
			}
		})
	}
}

func TestMainAndroidCaptureExits(t *testing.T) {
	// No adb on PATH: the hidden capture command gives up rather than
	// hanging, whichever way it ends.
	c := newChild(t, "_video-android", "emulator-5554")
	done := make(chan int, 1)
	go func() { done <- c.run(t) }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		_ = c.cmd.Process.Kill()
		t.Fatal("android capture did not exit without adb")
	}
}

func TestServeRunsUntilInterrupted(t *testing.T) {
	hid, video := fakeSidecars(t)
	c := newChild(t, "serve", "--addr", freeAddr(t), "--sidecar", hid, "--video-sidecar", video)
	if err := c.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- c.cmd.Wait() }()

	// "devicedeck serving" is logged as the startup guide begins; once the
	// guide is written the server waits for a signal.
	waitForLog(t, filepath.Join(c.home, ".devicedeck", "logs"), "devicedeck serving", exited)
	time.Sleep(1500 * time.Millisecond)
	if err := c.cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("serve exited with %v; output:\n%s", err, c.out.String())
		}
	case <-time.After(30 * time.Second):
		_ = c.cmd.Process.Kill()
		t.Fatal("serve did not stop on SIGINT")
	}
}

// waitForLog polls the run folders under logs for a line containing want.
func waitForLog(t *testing.T, logs, want string, exited <-chan error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		files, _ := filepath.Glob(filepath.Join(logs, "*", "devicedeck.log"))
		for _, f := range files {
			if data, _ := os.ReadFile(f); bytes.Contains(data, []byte(want)) {
				return
			}
		}
		select {
		case err := <-exited:
			t.Fatalf("serve exited early: %v", err)
		case <-ctx.Done():
			t.Fatalf("no %q in %s", want, logs)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func TestAndroidInjectorStartsTheEngine(t *testing.T) {
	// A fake adb that knows no device, first on a PATH with no real one:
	// the engine start fails fast and the error comes back through.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "adb"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/bin")
	st, err := buildStack("/nonexistent/devicedeck-hid", "/nonexistent/devicedeck-video", 30, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.androidInjector(context.Background(), "emulator-5554"); err == nil {
		t.Fatal("expected the engine start to fail without a device")
	}
}
