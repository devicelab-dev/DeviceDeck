package sim

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const fixture = `{
  "devices": {
    "com.apple.CoreSimulator.SimRuntime.iOS-18-6": [
      {"udid": "AAA", "name": "iPhone 16 Pro", "state": "Booted", "isAvailable": true},
      {"udid": "BBB", "name": "iPhone 16", "state": "Shutdown", "isAvailable": true},
      {"udid": "CCC", "name": "Broken", "state": "Booted", "isAvailable": false}
    ],
    "com.apple.CoreSimulator.SimRuntime.iOS-26-2": [
      {"udid": "DDD", "name": "iPhone 17", "state": "Booted", "isAvailable": true}
    ]
  }
}`

func fixedRun(out []byte, err error) runFunc {
	return func(context.Context, string, ...string) ([]byte, error) { return out, err }
}

func TestBooted(t *testing.T) {
	c := &Client{run: fixedRun([]byte(fixture), nil)}
	devices, err := c.Booted(context.Background())
	if err != nil {
		t.Fatalf("Booted: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2 (unavailable and shutdown excluded): %+v", len(devices), devices)
	}
	byUDID := map[string]Device{}
	for _, d := range devices {
		byUDID[d.UDID] = d
	}
	if d := byUDID["AAA"]; d.Name != "iPhone 16 Pro" || d.OS != "iOS 18.6" || !d.Booted {
		t.Errorf("AAA = %+v", d)
	}
	if d := byUDID["DDD"]; d.OS != "iOS 26.2" {
		t.Errorf("DDD OS = %q, want iOS 26.2", d.OS)
	}
}

func TestBootedErrors(t *testing.T) {
	tests := []struct {
		name string
		run  runFunc
	}{
		{"command failure", fixedRun(nil, errors.New("simctl exploded"))},
		{"bad json", fixedRun([]byte("not json"), nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{run: tt.run}
			if _, err := c.Booted(context.Background()); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestScreenshotPassesThrough(t *testing.T) {
	want := []byte{0x89, 'P', 'N', 'G'}
	var gotArgs string
	c := &Client{run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotArgs = name + " " + strings.Join(args, " ")
		return want, nil
	}}
	out, err := c.Screenshot(context.Background(), "AAA")
	if err != nil || string(out) != string(want) {
		t.Fatalf("Screenshot = %x, %v", out, err)
	}
	if gotArgs != "xcrun simctl io AAA screenshot --type=png -" {
		t.Errorf("command = %q", gotArgs)
	}
}

func TestRuntimeName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"com.apple.CoreSimulator.SimRuntime.iOS-18-6", "iOS 18.6"},
		{"com.apple.CoreSimulator.SimRuntime.iOS-26-2", "iOS 26.2"},
		{"com.apple.CoreSimulator.SimRuntime.watchOS-11-0", "watchOS 11.0"},
		{"com.apple.CoreSimulator.SimRuntime.weird", "weird"},
		{"custom-runtime", "custom-runtime"},
	}
	for _, tt := range tests {
		if got := runtimeName(tt.in); got != tt.want {
			t.Errorf("runtimeName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNewClientRunsRealCommands(t *testing.T) {
	c := NewClient()
	// Use a harmless command through the injected runner to cover the
	// default run implementation, including its error path.
	if out, err := c.run(context.Background(), "echo", "hi"); err != nil || strings.TrimSpace(string(out)) != "hi" {
		t.Fatalf("run echo = %q, %v", out, err)
	}
	if _, err := c.run(context.Background(), "false"); err == nil {
		t.Fatal("expected error from failing command")
	}
	_ = fmt.Sprintf("%v", c) // keep vet happy about unused import styles
}
