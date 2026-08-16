package input

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// stubSidecar writes a shell script that mimics devicedeck-hid: prints
// "ready", then copies stdin to a capture file until EOF.
func stubSidecar(t *testing.T, capture string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "stub-hid")
	script := "#!/bin/sh\necho ready\ncat > " + capture + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSessionSendAndClose(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "frames.bin")
	s, err := StartSidecar(context.Background(), stubSidecar(t, capture), "booted")
	if err != nil {
		t.Fatalf("StartSidecar: %v", err)
	}
	frame := Touch(TouchDown, 0.5, 0.5, EdgeNone)
	if err := s.Send(frame); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	got, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, frame) {
		t.Errorf("sidecar received %x, want %x", got, frame)
	}
}

func TestStartSidecarBadHandshake(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-hid")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho nope\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := StartSidecar(context.Background(), path, "booted"); err == nil {
		t.Fatal("expected handshake error, got nil")
	}
}

func TestStartSidecarMissingBinary(t *testing.T) {
	if _, err := StartSidecar(context.Background(), "/nonexistent/devicedeck-hid", "booted"); err == nil {
		t.Fatal("expected start error, got nil")
	}
}

func TestManagerCachesAndDrops(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "frames.bin")
	m := NewManager(stubSidecar(t, capture))
	defer m.CloseAll()

	ctx := context.Background()
	s1, err := m.Session(ctx, "UDID-1")
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	s2, err := m.Session(ctx, "UDID-1")
	if err != nil {
		t.Fatalf("Session (cached): %v", err)
	}
	if s1 != s2 {
		t.Error("expected cached session on second call")
	}

	m.Drop("UDID-1")
	s3, err := m.Session(ctx, "UDID-1")
	if err != nil {
		t.Fatalf("Session (after drop): %v", err)
	}
	if s3 == s1 {
		t.Error("expected a fresh session after Drop")
	}
}

func TestManagerCloseAll(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "frames.bin")
	m := NewManager(stubSidecar(t, capture))
	if _, err := m.Session(context.Background(), "UDID-1"); err != nil {
		t.Fatalf("Session: %v", err)
	}
	m.CloseAll()
	// After CloseAll the map is empty; a new Session must start fresh.
	start := time.Now()
	if _, err := m.Session(context.Background(), "UDID-1"); err != nil {
		t.Fatalf("Session after CloseAll: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("restart after CloseAll took suspiciously long")
	}
	m.CloseAll()
}

func TestSendFrameDelivers(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "frames.bin")
	m := NewManager(stubSidecar(t, capture))
	frame := SystemGesture(GestureSwipeToHome)
	if err := m.SendFrame(context.Background(), "UDID-1", frame); err != nil {
		t.Fatalf("SendFrame: %v", err)
	}
	m.CloseAll()
	got, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, frame) {
		t.Errorf("captured %x, want %x", got, frame)
	}
}

func TestSendFrameRestartsDeadSidecar(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "frames.bin")
	m := NewManager(stubSidecar(t, capture))
	s, err := m.Session(context.Background(), "UDID-1")
	if err != nil {
		t.Fatal(err)
	}
	// Kill the first sidecar behind the manager's back; SendFrame must
	// notice the dead pipe, restart, and still deliver.
	_ = s.Close()
	frame := LegacyButton(0)
	if err := m.SendFrame(context.Background(), "UDID-1", frame); err != nil {
		t.Fatalf("SendFrame after death: %v", err)
	}
	m.CloseAll()
	got, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, frame) {
		t.Errorf("captured %x, want %x", got, frame)
	}
}

func TestSendFrameStartFailure(t *testing.T) {
	m := NewManager("/nonexistent/devicedeck-hid")
	if err := m.SendFrame(context.Background(), "UDID-1", LegacyButton(0)); err == nil {
		t.Fatal("expected error when sidecar cannot start")
	}
}

func TestSendFrameRestartFailure(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "frames.bin")
	bin := stubSidecar(t, capture)
	m := NewManager(bin)
	s, err := m.Session(context.Background(), "UDID-1")
	if err != nil {
		t.Fatal(err)
	}
	// Kill the session and remove the binary: the restart attempt inside
	// SendFrame must surface the start error.
	_ = s.Close()
	if err := os.Remove(bin); err != nil {
		t.Fatal(err)
	}
	if err := m.SendFrame(context.Background(), "UDID-1", LegacyButton(0)); err == nil {
		t.Fatal("expected restart failure")
	}
}

func TestAwaitReadyTimeout(t *testing.T) {
	orig := readyTimeout
	readyTimeout = 200 * time.Millisecond
	t.Cleanup(func() { readyTimeout = orig })

	dir := t.TempDir()
	path := filepath.Join(dir, "silent-hid")
	// Prints nothing, just waits — must trip the handshake timeout.
	if err := os.WriteFile(path, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := StartSidecar(context.Background(), path, "booted"); err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestCloseKillsStubbornSidecar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stubborn-hid")
	// Ignores stdin EOF and sleeps well past the close grace period.
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho ready\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := StartSidecar(context.Background(), path, "booted")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_ = s.Close() // returns the kill error; the point is that it returns
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("Close took %v; kill fallback did not fire", elapsed)
	}
}

func TestSendAfterCloseFails(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "frames.bin")
	s, err := StartSidecar(context.Background(), stubSidecar(t, capture), "booted")
	if err != nil {
		t.Fatalf("StartSidecar: %v", err)
	}
	_ = s.Close()
	if err := s.Send(Touch(TouchDown, 0, 0, EdgeNone)); err == nil {
		t.Error("expected error sending after Close")
	}
}
