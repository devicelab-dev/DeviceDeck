package video

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// stubADB installs a fake adb on PATH: screencap emits marker bytes,
// screenrecord emits one synthetic Annex-B GOP then lingers so the cycle
// looks healthy until killed.
func stubADB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/bash
case "$*" in
*screencap*) printf 'PNGBYTES'; exit 0 ;;
*screenrecord*)
  printf '\x00\x00\x00\x01\x67\x42\xc0\x32'
  printf '\x00\x00\x00\x01\x68\xce'
  printf '\x00\x00\x00\x01\x65\x80\x11'
  printf '\x00\x00\x00\x01\x41\x80\x22'
  exec sleep 30 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "adb"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// syncBuffer is a mutex-guarded bytes.Buffer for cross-goroutine capture.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}

func TestAndroidCaptureLifecycle(t *testing.T) {
	stubADB(t)
	stdinR, stdinW := io.Pipe()
	out := &syncBuffer{}

	done := make(chan error, 1)
	go func() { done <- RunAndroidCapture("emulator-0000", stdinR, out) }()

	// Give the first cycle time to emit the still and the GOP, then ask
	// for a keyframe: the restart is debounced, but a fresh still must
	// still go out.
	time.Sleep(600 * time.Millisecond)
	if _, err := stdinW.Write([]byte{'K'}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	_ = stdinW.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunAndroidCapture: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("capture did not exit after stdin close")
	}

	counts := map[byte]int{}
	err := ReadFrames(bytes.NewReader(out.Bytes()), func(ft byte, p []byte) {
		counts[ft]++
		if ft == TypeStill && string(p) != "PNGBYTES" {
			t.Errorf("still payload = %q", p)
		}
	})
	if err != io.EOF {
		t.Fatalf("output framing broken: %v", err)
	}
	if counts[TypeStill] < 2 {
		t.Errorf("stills = %d, want one on start plus one per keyframe request", counts[TypeStill])
	}
	if counts[TypeDescription] < 1 || counts[TypeKeyframe] < 1 {
		t.Errorf("frame counts = %v, want description and keyframe from the GOP", counts)
	}
}

func TestAndroidCaptureGivesUpWhenADBFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "adb"), []byte("#!/bin/bash\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	err := RunAndroidCapture("emulator-0000", stdinR, io.Discard)
	if err == nil {
		t.Fatal("expected failure after repeated fast adb exits")
	}
}
