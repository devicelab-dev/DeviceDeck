package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/devicelab-dev/DeviceDeck/internal/doctor"
	"github.com/devicelab-dev/DeviceDeck/internal/sim"
	"github.com/devicelab-dev/DeviceDeck/internal/version"
)

// noTools is a machine where every tool check fails, so the welcome must
// list them all with their fixes.
func noTools() doctor.Env {
	return doctor.Env{
		Run:      func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("absent") },
		LookPath: func(string) (string, error) { return "", errors.New("absent") },
		Exists:   func(string) bool { return false },
	}
}

func renderWelcome(t *testing.T, f *fakePlatform, fancy bool) string {
	t.Helper()
	var out bytes.Buffer
	newWelcome(context.Background(), f, noTools(), "http://127.0.0.1:8787",
		[]string{"http://10.0.4.21:8787"}, "/home/.devicedeck/logs/serve-1", fancy).write(&out)
	return out.String()
}

func TestWelcomeListsBootedDevicesWithLinks(t *testing.T) {
	out := renderWelcome(t, &fakePlatform{devices: []sim.Device{
		{UDID: "EMU", Name: "Pixel 9", OS: "android", Booted: true},
		{UDID: "SIM", Name: "iPhone 17 Pro", OS: "iOS 26.2", Booted: true},
		{UDID: "OFF", Name: "iPhone 16", OS: "iOS 18.6"},
	}}, false)
	for _, want := range []string{
		"● Running",
		"OPEN", "Console    http://127.0.0.1:8787",
		"Network    http://10.0.4.21:8787",
		"DEVICES", "2 of 3 booted",
		"http://127.0.0.1:8787/device/SIM",
		"http://127.0.0.1:8787/device/EMU",
		"/device/booted",
		"USE WITH CLAUDE CODE", "1. Add the browser tool", "$ " + claudePlaywright,
		"$ " + claudeSkills, "$ " + claudeMCP,
		"USE WITH TESTS", "baseURL    http://127.0.0.1:8787/device/<udid>",
		"/home/.devicedeck/logs/serve-1",
		"Ctrl-C",
		"TOOLS", "✗ Xcode", "→ install Xcode from the App Store",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("welcome missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "Pixel 9") > strings.Index(out, "iPhone 17 Pro") {
		t.Error("booted devices should be listed by name")
	}
	if strings.Contains(out, "/device/OFF") || strings.Contains(out, "\x1b") {
		t.Errorf("unbooted device linked or escape codes in plain output:\n%s", out)
	}
}

func TestWelcomeWithoutBootedDevices(t *testing.T) {
	out := renderWelcome(t, &fakePlatform{devices: []sim.Device{{UDID: "OFF", Name: "iPhone 16"}}}, false)
	if !strings.Contains(out, "None booted yet. Pick one of 1 in the console.") {
		t.Errorf("welcome = %s", out)
	}
	failed := renderWelcome(t, &fakePlatform{listErr: errors.New("simctl down")}, false)
	if !strings.Contains(failed, "Pick one of 0 in the console.") || !strings.Contains(failed, claudePlaywright) {
		t.Errorf("a failed listing must still print the setup:\n%s", failed)
	}
}

func TestWelcomeOnATerminalHasClickableLinks(t *testing.T) {
	out := renderWelcome(t, &fakePlatform{}, true)
	if !strings.Contains(out, "\x1b]8;;http://127.0.0.1:8787\x1b\\") {
		t.Errorf("console link is not clickable: %q", out)
	}
}

func TestListen(t *testing.T) {
	srv := &http.Server{Addr: "127.0.0.1:0", Handler: http.NotFoundHandler()}
	errCh, err := listen(srv)
	if err != nil {
		t.Fatal(err)
	}
	_ = srv.Close()
	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		t.Errorf("serve ended with %v", err)
	}

	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = taken.Close() }()
	if _, err := listen(&http.Server{Addr: taken.Addr().String()}); err == nil ||
		!strings.Contains(err.Error(), "another devicedeck may be running") {
		t.Fatalf("taken port: err = %v", err)
	}
}

func TestCommand(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		wantCmd  string
		wantRest []string
	}{
		{nil, "serve", nil},
		{[]string{"--addr", ":9000"}, "serve", []string{"--addr", ":9000"}},
		{[]string{"serve", "--ready"}, "serve", []string{"--ready"}},
		{[]string{"mcp"}, "mcp", []string{}},
		{[]string{"doctor"}, "doctor", []string{}},
		{[]string{"--version"}, "version", nil},
		{[]string{"-h"}, "help", nil},
		{[]string{"help"}, "help", nil},
		{[]string{"frobnicate"}, "frobnicate", []string{}},
	} {
		cmd, rest := command(tc.args)
		if cmd != tc.wantCmd || len(rest) != len(tc.wantRest) {
			t.Errorf("command(%q) = %q %q, want %q %q", tc.args, cmd, rest, tc.wantCmd, tc.wantRest)
		}
	}
}

func TestRunDoctor(t *testing.T) {
	var out bytes.Buffer
	runDoctor(context.Background(), noTools(), &out)
	for _, want := range []string{version.Line(), "TOOLS", "✗ Xcode", "maestro-runner"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("doctor output missing %q:\n%s", want, out.String())
		}
	}
}

func TestExitOn(t *testing.T) {
	code := -1
	exit = func(c int) { code = c }
	t.Cleanup(func() { exit = os.Exit })
	exitOn("devicedeck", nil)
	if code != -1 {
		t.Fatal("a nil error must not exit")
	}
	exitOn("devicedeck", errors.New("port in use"))
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestWelcomeAllToolsFound(t *testing.T) {
	var out bytes.Buffer
	w := welcome{local: "http://127.0.0.1:8787", tools: []doctor.Result{{Name: "Xcode"}, {Name: "adb"}}}
	w.write(&out)
	if !strings.Contains(out.String(), "✓ All 2 found") {
		t.Errorf("welcome = %s", out.String())
	}
}

func TestTildeHome(t *testing.T) {
	t.Setenv("HOME", "/Users/dev")
	for in, want := range map[string]string{
		"/Users/dev/.devicedeck/logs/x": "~/.devicedeck/logs/x",
		"/Users/devx/logs":              "/Users/devx/logs",
		"/tmp/logs":                     "/tmp/logs",
	} {
		if got := tildeHome(in); got != want {
			t.Errorf("tildeHome(%q) = %q, want %q", in, got, want)
		}
	}
	t.Setenv("HOME", "")
	if got := tildeHome("/tmp/x"); got != "/tmp/x" {
		t.Errorf("no home: %q", got)
	}
}
