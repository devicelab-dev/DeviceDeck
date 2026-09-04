package video

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/devicelab-dev/DeviceDeck/internal/emugrpc"
)

func writeDiscovery(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverEmulatorIn(t *testing.T) {
	dir := t.TempDir()
	writeDiscovery(t, dir, "pid_100.ini",
		"port.serial=5554\ngrpc.port=8554\ngrpc.token=tok-a\n")
	writeDiscovery(t, dir, "pid_200.ini",
		"port.serial=5556\ngrpc.port=8556\ngrpc.token=tok-b\n")
	writeDiscovery(t, dir, "pid_300.ini",
		"port.serial=5558\n") // no grpc entries

	tests := []struct {
		name     string
		serial   string
		want     emulatorEndpoint
		wantErr  bool
		errmatch string
	}{
		{name: "first emulator", serial: "emulator-5554", want: emulatorEndpoint{addr: "127.0.0.1:8554", token: "tok-a"}},
		{name: "second emulator", serial: "emulator-5556", want: emulatorEndpoint{addr: "127.0.0.1:8556", token: "tok-b"}},
		{name: "missing grpc entries", serial: "emulator-5558", wantErr: true, errmatch: "lacks grpc"},
		{name: "unknown serial", serial: "emulator-9999", wantErr: true, errmatch: "no emulator discovery"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := discoverEmulatorIn([]string{dir}, tt.serial)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), tt.errmatch) {
					t.Fatalf("err = %v, want containing %q", err, tt.errmatch)
				}
				return
			}
			if err != nil {
				t.Fatalf("discover: %v", err)
			}
			if got != tt.want {
				t.Errorf("endpoint = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// fakeEmulator implements the EmulatorController stream: it checks the
// bearer token and pushes canned PNG frames.
type fakeEmulator struct {
	emugrpc.UnimplementedEmulatorControllerServer
	frames  [][]byte
	gotAuth chan string
}

func (f *fakeEmulator) StreamScreenshot(_ *emugrpc.ImageFormat, stream emugrpc.EmulatorController_StreamScreenshotServer) error {
	md, _ := metadata.FromIncomingContext(stream.Context())
	select {
	case f.gotAuth <- strings.Join(md.Get("authorization"), ","):
	default:
	}
	for _, fr := range f.frames {
		if err := stream.Send(&emugrpc.Image{Image: fr}); err != nil {
			return err
		}
	}
	<-stream.Context().Done() // hold the stream open like the emulator does
	return stream.Context().Err()
}

func TestStreamViaGRPC(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeEmulator{frames: [][]byte{[]byte("png-one"), []byte("png-two")}, gotAuth: make(chan string, 1)}
	srv := grpc.NewServer()
	emugrpc.RegisterEmulatorControllerServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	out := &syncBuffer{}
	c := &androidCapture{serial: "emulator-0000", out: &syncWriter{w: out}}

	done := make(chan error, 1)
	go func() {
		done <- c.streamViaGRPC(emulatorEndpoint{addr: lis.Addr().String(), token: "sekret"})
	}()

	// Wait for both frames to land, then simulate stdin EOF.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if bytes.Contains(out.Bytes(), []byte("png-two")) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	c.mu.Lock()
	c.closed = true
	cancel := c.cancelStream
	c.mu.Unlock()
	if cancel == nil {
		t.Fatal("stream cancel never registered")
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("streamViaGRPC: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not end after close")
	}

	if auth := <-fake.gotAuth; auth != "Bearer sekret" {
		t.Errorf("authorization = %q, want bearer token", auth)
	}
	var stills [][]byte
	err = ReadFrames(bytes.NewReader(out.Bytes()), func(ft byte, p []byte) {
		if ft == TypeStill {
			stills = append(stills, append([]byte(nil), p...))
		}
	})
	if err != io.EOF {
		t.Fatalf("framing: %v", err)
	}
	if len(stills) < 2 || !bytes.Equal(stills[0], []byte("png-one")) || !bytes.Equal(stills[1], []byte("png-two")) {
		t.Errorf("stills = %q, want the two streamed frames in order", stills)
	}
}

func TestStreamViaGRPCFailsWithoutServer(t *testing.T) {
	c := &androidCapture{serial: "emulator-0000", out: &syncWriter{w: io.Discard}}
	err := c.streamViaGRPC(emulatorEndpoint{addr: "127.0.0.1:1", token: "x"})
	if err == nil {
		t.Fatal("expected failure against dead endpoint")
	}
	netErr := err
	_ = errors.Unwrap(netErr) // shape not asserted — only that fallback would trigger
}
