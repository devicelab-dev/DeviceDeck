package logging

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// startRun starts a run in a temp home and restores the default logger and
// active run afterwards, so tests cannot leak state into each other.
func startRun(t *testing.T, term io.Writer, level slog.Level) (*Run, string) {
	t.Helper()
	prev := slog.Default()
	home := t.TempDir()
	r, err := Start(home, "serve", term, level)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		slog.SetDefault(prev)
	})
	return r, home
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestStartFileKeepsEverythingTerminalKeepsItsLevel(t *testing.T) {
	var term bytes.Buffer
	r, home := startRun(t, &term, slog.LevelInfo)
	if !strings.HasPrefix(r.Dir, filepath.Join(home, "logs", "serve-")) {
		t.Fatalf("run dir = %s", r.Dir)
	}
	slog.Debug("engine detail", "udid", "AAA")
	slog.Info("device booted", "udid", "AAA")
	file := read(t, r.Path("devicedeck.log"))
	for _, want := range []string{"engine detail", "device booted", "udid=AAA"} {
		if !strings.Contains(file, want) {
			t.Errorf("file log missing %q:\n%s", want, file)
		}
	}
	if strings.Contains(term.String(), "engine detail") || !strings.Contains(term.String(), "device booted") {
		t.Errorf("terminal should hold info but not debug:\n%s", term.String())
	}
	if _, err := os.Stat(r.Path("crash.log")); err != nil {
		t.Errorf("crash log not created: %v", err)
	}
}

func TestStartWithoutTerminalLogsToFileOnly(t *testing.T) {
	r, _ := startRun(t, nil, slog.LevelInfo)
	slog.Warn("tool failed")
	if !strings.Contains(read(t, r.Path("devicedeck.log")), "tool failed") {
		t.Error("file-only run lost a record")
	}
}

func TestStartErrors(t *testing.T) {
	t.Run("home is a file", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(home, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Start(home, "serve", nil, slog.LevelInfo); err == nil ||
			!strings.Contains(err.Error(), "create log folder") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("main log cannot be opened", func(t *testing.T) {
		fixed := time.Date(2026, 9, 22, 15, 0, 0, 0, time.Local)
		now = func() time.Time { return fixed }
		t.Cleanup(func() { now = time.Now })
		home := t.TempDir()
		blocker := filepath.Join(home, "logs", "serve-"+fixed.Format("20060102-150405.000"), "devicedeck.log")
		if err := os.MkdirAll(blocker, 0o750); err != nil { // a folder where the file should go
			t.Fatal(err)
		}
		if _, err := Start(home, "serve", nil, slog.LevelInfo); err == nil ||
			!strings.Contains(err.Error(), "open log devicedeck.log") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestComponent(t *testing.T) {
	t.Run("no active run falls back to stderr", func(t *testing.T) {
		setActive(nil)
		if w := Component("hid-AAA"); w != os.Stderr {
			t.Errorf("Component = %T, want os.Stderr", w)
		}
	})
	t.Run("active run appends to one shared file per name", func(t *testing.T) {
		r, _ := startRun(t, nil, slog.LevelInfo)
		_, _ = io.WriteString(Component("video-avd:Pixel/9"), "first\n")
		_, _ = io.WriteString(Component("video-avd:Pixel/9"), "second\n")
		got := read(t, r.Path("video-avd_Pixel_9.log"))
		if got != "first\nsecond\n" {
			t.Errorf("component log = %q", got)
		}
	})
	t.Run("unopenable file falls back to stderr", func(t *testing.T) {
		r, _ := startRun(t, nil, slog.LevelInfo)
		if err := os.Mkdir(r.Path("hid-BBB.log"), 0o750); err != nil {
			t.Fatal(err)
		}
		if w := Component("hid-BBB"); w != os.Stderr {
			t.Errorf("Component = %T, want os.Stderr", w)
		}
	})
}

func TestCrashLogUnavailableIsNotFatal(t *testing.T) {
	r, _ := startRun(t, nil, slog.LevelInfo)
	r.Dir = filepath.Join(r.Dir, "missing") // crash.log can no longer be created
	r.captureCrashes()
	if _, ok := r.files["crash"]; ok && r.files["crash"] == nil {
		t.Error("nil crash file recorded")
	}
}

func TestCloseDeactivates(t *testing.T) {
	r, _ := startRun(t, nil, slog.LevelInfo)
	_ = Component("hid-AAA")
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if getActive() != nil || len(r.files) != 0 {
		t.Error("Close left the run active or files open")
	}
}

func TestLevel(t *testing.T) {
	for in, want := range map[string]slog.Level{
		"debug": slog.LevelDebug, "INFO": slog.LevelInfo, "warn": slog.LevelWarn,
		"error": slog.LevelError, "": slog.LevelWarn, "loud": slog.LevelWarn,
	} {
		if got := Level(in); got != want {
			t.Errorf("Level(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestPruneKeepsNewest(t *testing.T) {
	logs := t.TempDir()
	base := time.Now().Add(-time.Hour)
	for i := range 5 {
		dir := filepath.Join(logs, "serve-"+string(rune('a'+i)))
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		mod := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(dir, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(logs, "stray.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	prune(logs, 2)
	entries, _ := os.ReadDir(logs)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if got := strings.Join(names, ","); got != "serve-d,serve-e,stray.txt" {
		t.Errorf("after prune: %s", got)
	}
	prune(filepath.Join(logs, "missing"), 2) // a missing folder is a no-op
}

func TestCaptureConsole(t *testing.T) {
	r, _ := startRun(t, nil, slog.LevelInfo)
	origOut, origErr := os.Stdout, os.Stderr
	if err := r.CaptureConsole(); err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(os.Stdout, "printed to stdout")
	fmt.Fprintln(os.Stderr, "printed to stderr")
	_, _ = io.WriteString(Component("hid-AAA"), "sidecar line\n")
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if os.Stdout != origOut || os.Stderr != origErr {
		t.Fatal("Close did not restore stdout and stderr")
	}
	console := read(t, r.Path("console.log"))
	for _, want := range []string{"printed to stdout", "printed to stderr"} {
		if !strings.Contains(console, want) {
			t.Errorf("console.log missing %q: %q", want, console)
		}
	}
	if strings.Contains(console, "sidecar line") {
		t.Error("child-process output was copied into console.log as well as its own log")
	}
}

func TestCaptureConsoleErrors(t *testing.T) {
	t.Run("console log cannot be opened", func(t *testing.T) {
		r, _ := startRun(t, nil, slog.LevelInfo)
		if err := os.Mkdir(r.Path("console.log"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := r.CaptureConsole(); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("second pipe fails and the first is undone", func(t *testing.T) {
		r, _ := startRun(t, nil, slog.LevelInfo)
		origOut, origErr := os.Stdout, os.Stderr
		calls := 0
		pipe = func() (*os.File, *os.File, error) {
			if calls++; calls == 2 {
				return nil, nil, errors.New("too many open files")
			}
			return os.Pipe()
		}
		t.Cleanup(func() { pipe = os.Pipe })
		if err := r.CaptureConsole(); err == nil || !strings.Contains(err.Error(), "capture console") {
			t.Fatalf("err = %v", err)
		}
		if os.Stdout != origOut || os.Stderr != origErr || r.console != nil {
			t.Error("a failed capture left the streams redirected")
		}
	})
}
