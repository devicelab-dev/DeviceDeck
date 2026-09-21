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

// LaunchApp starts fresh: a caller relying on the app's first screen
// cannot inherit whatever the last session left running.
func TestLaunchAppTerminatesFirst(t *testing.T) {
	var calls []string
	c := &Client{run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil, nil
	}}
	if err := c.LaunchApp(context.Background(), "UDID-1", "com.example"); err != nil {
		t.Fatalf("LaunchApp: %v", err)
	}
	if len(calls) != 2 ||
		!strings.Contains(calls[0], "terminate UDID-1 com.example") ||
		!strings.Contains(calls[1], "launch UDID-1 com.example") {
		t.Fatalf("calls = %v", calls)
	}
}

// A terminate failure means it was not running, which is the state we
// wanted; only the launch itself can fail the call.
func TestLaunchAppIgnoresTerminateFailure(t *testing.T) {
	c := &Client{run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 1 && args[1] == "terminate" {
			return nil, errors.New("not running")
		}
		return nil, nil
	}}
	if err := c.LaunchApp(context.Background(), "UDID-1", "com.example"); err != nil {
		t.Errorf("terminate failure should not fail the launch: %v", err)
	}
	failing := &Client{run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 1 && args[1] == "launch" {
			return nil, errors.New("no such app")
		}
		return nil, nil
	}}
	if err := failing.LaunchApp(context.Background(), "UDID-1", "com.example"); err == nil {
		t.Error("a failed launch must be reported")
	}
}

// ResetApp empties the app's data container so the next launch is a first
// run: it locates the container, then wipes Library, Documents and tmp.
func TestResetApp(t *testing.T) {
	var calls []string
	c := &Client{run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if len(args) > 1 && args[1] == "get_app_container" {
			return []byte("/data/Containers/Data/Application/ABC\n"), nil
		}
		return nil, nil
	}}
	if err := c.ResetApp(context.Background(), "UDID-1", "com.example"); err != nil {
		t.Fatalf("ResetApp: %v", err)
	}
	rm := calls[len(calls)-1]
	for _, want := range []string{
		"rm -rf",
		"/data/Containers/Data/Application/ABC/Library",
		"/data/Containers/Data/Application/ABC/Documents",
		"/data/Containers/Data/Application/ABC/tmp",
	} {
		if !strings.Contains(rm, want) {
			t.Errorf("rm call %q missing %q", rm, want)
		}
	}
}

// A container that cannot be located fails the reset rather than guessing.
func TestResetAppLocateFailure(t *testing.T) {
	c := &Client{run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 1 && args[1] == "get_app_container" {
			return nil, errors.New("no such app")
		}
		return nil, nil
	}}
	err := c.ResetApp(context.Background(), "UDID-1", "com.example")
	if err == nil || !strings.Contains(err.Error(), "locate") {
		t.Fatalf("expected a locate error, got %v", err)
	}
}

// A non-absolute container path would make the wipe `rm -rf /Library`, so
// it is refused outright.
func TestResetAppRefusesRelativePath(t *testing.T) {
	c := &Client{run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 1 && args[1] == "get_app_container" {
			return []byte("\n"), nil // empty path
		}
		return nil, nil
	}}
	err := c.ResetApp(context.Background(), "UDID-1", "com.example")
	if err == nil || !strings.Contains(err.Error(), "no data container") {
		t.Fatalf("expected a no-container error, got %v", err)
	}
}

// The wipe failing fails the reset — stale data is not the clean slate
// that was asked for.
func TestResetAppWipeFailure(t *testing.T) {
	c := &Client{run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if len(args) > 1 && args[1] == "get_app_container" {
			return []byte("/data/App/ABC\n"), nil
		}
		if name == "rm" {
			return nil, errors.New("permission denied")
		}
		return nil, nil
	}}
	err := c.ResetApp(context.Background(), "UDID-1", "com.example")
	if err == nil || !strings.Contains(err.Error(), "reset com.example") {
		t.Fatalf("expected a reset error, got %v", err)
	}
}

func TestInstall(t *testing.T) {
	var got string
	c := &Client{run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		got = name + " " + strings.Join(args, " ")
		return nil, nil
	}}
	if err := c.Install(context.Background(), "UDID-1", "/path/My.app"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !strings.Contains(got, "simctl install UDID-1 /path/My.app") {
		t.Fatalf("call = %q", got)
	}
}

func TestInstallError(t *testing.T) {
	c := &Client{run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, errors.New("no such file")
	}}
	if err := c.Install(context.Background(), "UDID-1", "/x.app"); err == nil {
		t.Fatal("expected an install error")
	}
}

func TestOpenURL(t *testing.T) {
	var got []string
	c := &Client{run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		got = append([]string{name}, args...)
		return nil, nil
	}}
	if err := c.OpenURL(context.Background(), "U1", "myapp://checkout"); err != nil {
		t.Fatal(err)
	}
	want := []string{"xcrun", "simctl", "openurl", "U1", "myapp://checkout"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("cmd = %v, want %v", got, want)
	}
	// A simctl failure is reported.
	fail := &Client{run: func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("simctl down")
	}}
	if err := fail.OpenURL(context.Background(), "U1", "x"); err == nil {
		t.Error("openurl failure must propagate")
	}
}
