package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/devicelab-dev/DeviceDeck/internal/input"
)

type fakeVideo struct {
	frames chan []byte
	err    error
	udid   string
}

func (f *fakeVideo) Subscribe(_ context.Context, udid string) (<-chan []byte, func(), error) {
	f.udid = udid
	if f.err != nil {
		return nil, nil, f.err
	}
	return f.frames, func() {}, nil
}

func wsServer(t *testing.T, backend *fakeBackend, videoSrc VideoSource) *httptest.Server {
	t.Helper()
	s := New(backend, backend, backend, backend, videoSrc, &fakeCapture{})
	s.sleep = func(time.Duration) {}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

func wsAddr(srv *httptest.Server, path string) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + path
}

func TestVideoWSStreamsFrames(t *testing.T) {
	fv := &fakeVideo{frames: make(chan []byte, 4)}
	fv.frames <- []byte{1, 0xAA}
	fv.frames <- []byte{2, 0xBB}
	srv := wsServer(t, &fakeBackend{}, fv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsAddr(srv, "/api/devices/AAA/video"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	for _, want := range [][]byte{{1, 0xAA}, {2, 0xBB}} {
		kind, msg, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if kind != websocket.MessageBinary || string(msg) != string(want) {
			t.Errorf("got %v %x, want binary %x", kind, msg, want)
		}
	}
	if fv.udid != "AAA" {
		t.Errorf("subscribed udid = %q", fv.udid)
	}

	// Closing the source ends the socket cleanly.
	close(fv.frames)
	if _, _, err := conn.Read(ctx); err == nil {
		t.Error("expected close after source ended")
	}
}

func TestVideoWSSubscribeError(t *testing.T) {
	srv := wsServer(t, &fakeBackend{}, &fakeVideo{err: errors.New("no sim")})
	res, err := http.Get(srv.URL + "/api/devices/AAA/video")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d", res.StatusCode)
	}
}

func TestInputWSForwardsValidFramesOnly(t *testing.T) {
	backend := &fakeBackend{}
	srv := wsServer(t, backend, &fakeVideo{frames: make(chan []byte)})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsAddr(srv, "/api/devices/AAA/input"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	valid := input.Touch(input.TouchDown, 0.5, 0.5, input.EdgeNone)
	if err := conn.Write(ctx, websocket.MessageBinary, []byte{0xFF, 0x01}); err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(ctx, websocket.MessageText, []byte("junk")); err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(ctx, websocket.MessageBinary, valid); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	for len(backend.sentFrames()) < 1 {
		select {
		case <-deadline:
			t.Fatalf("frame never forwarded; got %x", backend.sentFrames())
		case <-time.After(10 * time.Millisecond):
		}
	}
	frames := backend.sentFrames()
	if len(frames) != 1 || string(frames[0]) != string(valid) {
		t.Errorf("forwarded = %x, want only %x", frames, valid)
	}
	if backend.sentUDID() != "AAA" {
		t.Errorf("udid = %q", backend.sentUDID())
	}
}

func TestInputWSClosesOnSidecarFailure(t *testing.T) {
	backend := &fakeBackend{framesErr: errors.New("sidecar dead")}
	srv := wsServer(t, backend, &fakeVideo{frames: make(chan []byte)})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsAddr(srv, "/api/devices/AAA/input"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	frame := input.Touch(input.TouchDown, 0.5, 0.5, input.EdgeNone)
	if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.Read(ctx); err == nil {
		t.Error("expected close after sidecar failure")
	}
}
