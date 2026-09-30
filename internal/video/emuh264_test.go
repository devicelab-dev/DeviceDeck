package video

import (
	"bytes"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/devicelab-dev/DeviceDeck/internal/emugrpc"
)

// fakeEncoder installs a stand-in for devicedeck-video --stdin-frames: it
// writes one framed keyframe to stdout and records everything on stdin.
func fakeEncoder(t *testing.T) (path, stdinLog string) {
	t.Helper()
	dir := t.TempDir()
	stdinLog = filepath.Join(dir, "stdin")
	path = filepath.Join(dir, "encoder")
	// Frame: type 2 (keyframe), length 3, payload "avc".
	script := "#!/bin/sh\nprintf '\\002\\000\\000\\000\\003avc'\ncat > " + stdinLog + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, stdinLog
}

// rawEmulator streams canned raw frames, the empty one first.
type rawEmulator struct {
	emugrpc.UnimplementedEmulatorControllerServer
	gotFormat chan emugrpc.ImageFormat_ImgFormat
}

func (f *rawEmulator) StreamScreenshot(req *emugrpc.ImageFormat, stream emugrpc.EmulatorController_StreamScreenshotServer) error {
	f.gotFormat <- req.GetFormat()
	_ = stream.Send(&emugrpc.Image{}) // no pixels yet: skipped
	_ = stream.Send(&emugrpc.Image{
		Format: &emugrpc.ImageFormat{Width: 2, Height: 1},
		Image:  []byte{1, 2, 3, 4, 5, 6},
	})
	<-stream.Context().Done()
	return stream.Context().Err()
}

// Raw frames from the emulator reach the encoder as 'F' messages, a
// keyframe request reaches it as 'K', and its framed output is relayed.
func TestStreamH264(t *testing.T) {
	encPath, stdinLog := fakeEncoder(t)
	t.Setenv(EncoderEnv, encPath)
	t.Setenv(EncoderFPSEnv, "15")
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fake := &rawEmulator{gotFormat: make(chan emugrpc.ImageFormat_ImgFormat, 4)}
	srv := grpc.NewServer()
	emugrpc.RegisterEmulatorControllerServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	out := &syncBuffer{}
	c := &androidCapture{serial: "emulator-0000", out: &syncWriter{w: out}}
	done := make(chan error, 1)
	go func() { done <- c.streamH264(emulatorEndpoint{addr: lis.Addr().String(), token: "t"}) }()

	if got := <-fake.gotFormat; got != emugrpc.ImageFormat_RGB888 {
		t.Errorf("requested format = %v, want RGB888", got)
	}
	waitFor(t, func() bool { return bytes.Contains(out.Bytes(), []byte("avc")) })
	waitFor(t, func() bool { data, _ := os.ReadFile(stdinLog); return len(data) >= 19 })
	c.requestKeyframe()

	c.mu.Lock()
	c.closed = true
	cancel := c.cancelStream
	c.mu.Unlock()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("streamH264: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not end")
	}
	data, _ := os.ReadFile(stdinLog)
	want := []byte{'F'}
	want = binary.BigEndian.AppendUint32(want, 2)
	want = binary.BigEndian.AppendUint32(want, 1)
	want = binary.BigEndian.AppendUint32(want, 6)
	want = append(want, 1, 2, 3, 4, 5, 6, 'K')
	if !bytes.Equal(data, want) {
		t.Errorf("encoder stdin = %v, want %v", data, want)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStartEncoderFailures(t *testing.T) {
	if _, err := startEncoder("", 30); err == nil || !strings.Contains(err.Error(), EncoderEnv) {
		t.Errorf("no path: %v", err)
	}
	if _, err := startEncoder(filepath.Join(t.TempDir(), "missing"), 30); err == nil {
		t.Error("a missing encoder must fail to start")
	}
	t.Setenv(EncoderEnv, "")
	c := &androidCapture{serial: "emulator-0000", out: &syncWriter{w: &syncBuffer{}}}
	if err := c.streamH264(emulatorEndpoint{addr: "127.0.0.1:1"}); err == nil {
		t.Error("streamH264 without an encoder must fail")
	}
}

// An encoder that has exited makes the next frame fail, which ends the
// stream with the encoder named.
func TestEncoderGoneFailsTheStream(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "encoder")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	enc, err := startEncoder(path, 30)
	if err != nil {
		t.Fatal(err)
	}
	_ = enc.cmd.Wait()
	big := make([]byte, 1<<20)
	var werr error
	for i := 0; i < 20 && werr == nil; i++ {
		werr = enc.frame(512, 512, big)
	}
	if werr == nil {
		t.Error("writing to an exited encoder should fail")
	}
	if err := enc.keyframe(); err == nil {
		t.Error("keyframe to an exited encoder should fail")
	}
}

// DEVICEDECK_ANDROID_CAPTURE=h264 takes the H.264 path with no fallback.
func TestRunAndroidCaptureH264Forced(t *testing.T) {
	t.Setenv("DEVICEDECK_ANDROID_CAPTURE", "h264")
	old := discoverEndpoint
	defer func() { discoverEndpoint = old }()
	discoverEndpoint = func(string) (emulatorEndpoint, error) { return emulatorEndpoint{}, os.ErrNotExist }
	if err := RunAndroidCapture("emulator-0000", strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Error("a missing emulator endpoint must fail on the forced h264 path")
	}
	discoverEndpoint = func(string) (emulatorEndpoint, error) { return emulatorEndpoint{addr: "127.0.0.1:1"}, nil }
	t.Setenv(EncoderEnv, "")
	if err := RunAndroidCapture("emulator-0000", strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Error("no encoder must fail on the forced h264 path")
	}
}

// A stream that cannot open, and an encoder that has gone, both end the pump
// with an error the retry loop can act on.
func TestPumpRawStreamErrors(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fake := &rawEmulator{gotFormat: make(chan emugrpc.ImageFormat_ImgFormat, 4)}
	srv := grpc.NewServer()
	emugrpc.RegisterEmulatorControllerServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	client := emugrpc.NewEmulatorControllerClient(conn)
	c := &androidCapture{serial: "emulator-0000", out: &syncWriter{w: &syncBuffer{}}}

	dir := t.TempDir()
	path := filepath.Join(dir, "encoder")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	enc, err := startEncoder(path, 30)
	if err != nil {
		t.Fatal(err)
	}
	_ = enc.cmd.Wait() // gone before the first frame
	if err := c.pumpRawStream(client, enc); err == nil || !strings.Contains(err.Error(), "video encoder") {
		t.Errorf("gone encoder: %v", err)
	}

	_ = conn.Close()
	if err := c.pumpRawStream(client, enc); err == nil {
		t.Error("a closed connection must fail to open the stream")
	}
}
