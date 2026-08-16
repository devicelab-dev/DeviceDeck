package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeStub(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveBinaryExplicit(t *testing.T) {
	bin := writeStub(t, t.TempDir(), "devicedeck-hid")
	got, err := resolveBinary("devicedeck-hid", bin)
	if err != nil || got != bin {
		t.Fatalf("resolveBinary = %q, %v", got, err)
	}
}

func TestResolveBinaryEnv(t *testing.T) {
	bin := writeStub(t, t.TempDir(), "devicedeck-video")
	t.Setenv("DEVICEDECK_VIDEO", bin)
	got, err := resolveBinary("devicedeck-video", "")
	if err != nil || got != bin {
		t.Fatalf("resolveBinary via env = %q, %v", got, err)
	}
}

func TestResolveBinaryDevBuildPath(t *testing.T) {
	root := t.TempDir()
	release := filepath.Join(root, "sidecar/.build/release")
	if err := os.MkdirAll(release, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := writeStub(t, release, "devicedeck-hid")
	t.Chdir(root)
	got, err := resolveBinary("devicedeck-hid", "")
	if err != nil || got != filepath.Join("sidecar/.build/release", "devicedeck-hid") {
		t.Fatalf("resolveBinary dev path = %q, %v (stub at %s)", got, err, bin)
	}
}

func TestResolveBinarySkipsDirsAndMissing(t *testing.T) {
	// A directory with the right name must not be accepted as the binary.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "devicedeck-hid"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEVICEDECK_HID", filepath.Join(dir, "devicedeck-hid"))
	t.Chdir(t.TempDir()) // ensure the relative dev-build path also misses
	if _, err := resolveBinary("devicedeck-hid", ""); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestDefaultRunnerHome(t *testing.T) {
	t.Run("respects explicit value", func(t *testing.T) {
		t.Setenv("MAESTRO_RUNNER_HOME", "/explicit")
		defaultRunnerHome()
		if got := os.Getenv("MAESTRO_RUNNER_HOME"); got != "/explicit" {
			t.Errorf("MAESTRO_RUNNER_HOME = %q", got)
		}
	})
	t.Run("defaults to install dir when present", func(t *testing.T) {
		home := t.TempDir()
		if err := os.Mkdir(filepath.Join(home, ".maestro-runner"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HOME", home)
		t.Setenv("MAESTRO_RUNNER_HOME", "")
		os.Unsetenv("MAESTRO_RUNNER_HOME")
		defaultRunnerHome()
		if got := os.Getenv("MAESTRO_RUNNER_HOME"); got != filepath.Join(home, ".maestro-runner") {
			t.Errorf("MAESTRO_RUNNER_HOME = %q", got)
		}
	})
	t.Run("leaves unset when install dir missing", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("MAESTRO_RUNNER_HOME", "")
		os.Unsetenv("MAESTRO_RUNNER_HOME")
		defaultRunnerHome()
		if got := os.Getenv("MAESTRO_RUNNER_HOME"); got != "" {
			t.Errorf("MAESTRO_RUNNER_HOME = %q, want unset", got)
		}
	})
}
