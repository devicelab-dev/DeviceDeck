package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSidecarExplicit(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "devicedeck-hid")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveSidecar(bin)
	if err != nil || got != bin {
		t.Fatalf("resolveSidecar = %q, %v", got, err)
	}
}

func TestResolveSidecarEnv(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "devicedeck-hid")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEVICEDECK_HID", bin)
	got, err := resolveSidecar("")
	if err != nil || got != bin {
		t.Fatalf("resolveSidecar via env = %q, %v", got, err)
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

func TestResolveSidecarSkipsDirsAndMissing(t *testing.T) {
	// A directory with the right name must not be accepted as the binary.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "devicedeck-hid"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEVICEDECK_HID", filepath.Join(dir, "devicedeck-hid"))
	t.Chdir(t.TempDir()) // ensure the relative dev-build path also misses
	if _, err := resolveSidecar(""); err == nil {
		t.Fatal("expected not-found error")
	}
}
