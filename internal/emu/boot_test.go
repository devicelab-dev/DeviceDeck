package emu

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Boot spawns whatever `emulator` is first on PATH. These tests put a
// stand-in there — or nothing at all — so no real AVD is ever launched.

func TestBootLaunchesTheEmulator(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	script := "#!/bin/sh\necho \"$*\" > " + calls + "\n"
	if err := os.WriteFile(filepath.Join(dir, "emulator"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/bin")

	if err := NewClient().Boot(context.Background(), AVDPrefix+"Pixel_8"); err != nil {
		t.Fatalf("Boot: %v", err)
	}
	// The process is detached; wait for the stand-in to record its args.
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, _ := os.ReadFile(calls)
		if strings.HasPrefix(string(got), "-avd Pixel_8 ") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("emulator args = %q", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestBootWithoutAnEmulator(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := NewClient().Boot(context.Background(), "Pixel_8")
	if err == nil || !strings.Contains(err.Error(), "launch emulator Pixel_8") {
		t.Fatalf("err = %v", err)
	}
}
