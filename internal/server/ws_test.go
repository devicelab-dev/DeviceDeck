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
	s := New(backend, backend, backend, backend, backend, backend, videoSrc, &fakeCapture{})
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

// A connection that vanishes mid-drag must not leave the device holding
// a finger down: the sidecar would have a touch-down with no matching up
// and every later interaction would land on a wedged input stack.
func TestInputWSReleasesHeldTouchOnDisconnect(t *testing.T) {
	backend := &fakeBackend{}
	srv := wsServer(t, backend, &fakeVideo{frames: make(chan []byte)})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsAddr(srv, "/api/devices/AAA/input"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	down := input.Touch(input.TouchDown, 0.25, 0.75, input.EdgeNone)
	if err := conn.Write(ctx, websocket.MessageBinary, down); err != nil {
		t.Fatal(err)
	}
	for len(backend.sentFrames()) < 1 {
		time.Sleep(5 * time.Millisecond)
	}
	// Vanish mid-gesture, without ever sending the up.
	conn.CloseNow()

	want := input.Touch(input.TouchUp, 0.25, 0.75, input.EdgeNone)
	deadline := time.After(5 * time.Second)
	for {
		frames := backend.sentFrames()
		if len(frames) >= 2 && string(frames[len(frames)-1]) == string(want) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("held touch never released; frames=%x", frames)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// A completed gesture needs no cleanup — a spurious extra touch-up would
// register as a second tap.
func TestInputWSDoesNotReleaseCompletedGesture(t *testing.T) {
	backend := &fakeBackend{}
	srv := wsServer(t, backend, &fakeVideo{frames: make(chan []byte)})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsAddr(srv, "/api/devices/AAA/input"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	for _, f := range [][]byte{
		input.Touch(input.TouchDown, 0.5, 0.5, input.EdgeNone),
		input.Touch(input.TouchUp, 0.5, 0.5, input.EdgeNone),
	} {
		if err := conn.Write(ctx, websocket.MessageBinary, f); err != nil {
			t.Fatal(err)
		}
	}
	for len(backend.sentFrames()) < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	conn.CloseNow()

	// Give any erroneous cleanup a chance to fire before asserting.
	time.Sleep(200 * time.Millisecond)
	if got := len(backend.sentFrames()); got != 2 {
		t.Fatalf("expected exactly the two frames sent, got %d: %x", got, backend.sentFrames())
	}
}

// Two-finger gestures hold two contacts and need the two-finger release.
func TestInputWSReleasesHeldTwoFinger(t *testing.T) {
	backend := &fakeBackend{}
	srv := wsServer(t, backend, &fakeVideo{frames: make(chan []byte)})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsAddr(srv, "/api/devices/AAA/input"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageBinary,
		input.TwoFinger(input.TouchDown, 0.2, 0.3, 0.7, 0.8)); err != nil {
		t.Fatal(err)
	}
	for len(backend.sentFrames()) < 1 {
		time.Sleep(5 * time.Millisecond)
	}
	conn.CloseNow()

	want := input.TwoFinger(input.TouchUp, 0.2, 0.3, 0.7, 0.8)
	deadline := time.After(5 * time.Second)
	for {
		frames := backend.sentFrames()
		if len(frames) >= 2 && string(frames[len(frames)-1]) == string(want) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("two-finger contact never released; frames=%x", frames)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// observe must ignore frames that are not contacts: a key press between
// touch-down and disconnect must not clear the held contact, and an
// undecodable frame must not panic or change state.
func TestHeldTouchObserveIgnoresNonContactFrames(t *testing.T) {
	var h heldTouch
	h.observe(input.Touch(input.TouchDown, 0.1, 0.2, input.EdgeNone))
	h.observe(input.Key(0, 0x04))
	h.observe(input.SystemGesture(input.GestureSwipeToHome))
	h.observe([]byte{0xFF})
	got := h.frames()
	if len(got) != 1 || string(got[0]) != string(input.Touch(input.TouchUp, 0.1, 0.2, input.EdgeNone)) {
		t.Fatalf("held contact lost or corrupted: %x", got)
	}
	// A move updates the release point without releasing.
	h.observe(input.Touch(input.TouchMove, 0.6, 0.7, input.EdgeNone))
	got = h.frames()
	if len(got) != 1 || string(got[0]) != string(input.Touch(input.TouchUp, 0.6, 0.7, input.EdgeNone)) {
		t.Fatalf("release point not tracked through move: %x", got)
	}
}

// A device drives one client at a time. The second connection is refused
// with an explanation rather than silently interleaving its touches with
// the first — which is what test runners produce by default, since they
// parallelise across files.
func TestInputWSRefusesSecondDriver(t *testing.T) {
	backend := &fakeBackend{}
	srv := wsServer(t, backend, &fakeVideo{frames: make(chan []byte)})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	first, _, err := websocket.Dial(ctx, wsAddr(srv, "/api/devices/AAA/input"), nil)
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	defer first.Close(websocket.StatusNormalClosure, "")
	// Drive one frame so the first connection is unambiguously established.
	if err := first.Write(ctx, websocket.MessageBinary,
		input.Touch(input.TouchDown, 0.5, 0.5, input.EdgeNone)); err != nil {
		t.Fatal(err)
	}
	for len(backend.sentFrames()) < 1 {
		time.Sleep(5 * time.Millisecond)
	}

	second, _, err := websocket.Dial(ctx, wsAddr(srv, "/api/devices/AAA/input"), nil)
	if err != nil {
		t.Fatalf("second dial: %v", err)
	}
	defer second.Close(websocket.StatusInternalError, "")
	_, _, readErr := second.Read(ctx)
	if readErr == nil {
		t.Fatal("second driver was accepted")
	}
	if status := websocket.CloseStatus(readErr); status != websocket.StatusPolicyViolation {
		t.Errorf("close status = %v, want policy violation: %v", status, readErr)
	}
	if !strings.Contains(readErr.Error(), "one driver at a time") {
		t.Errorf("refusal does not explain itself: %v", readErr)
	}

	// A different device stays available.
	other, _, err := websocket.Dial(ctx, wsAddr(srv, "/api/devices/BBB/input"), nil)
	if err != nil {
		t.Fatalf("other device refused: %v", err)
	}
	other.Close(websocket.StatusNormalClosure, "")
}

// Releasing the device has to happen on disconnect, or one crashed
// client locks a device until the server restarts.
func TestInputWSReleasesOnDisconnect(t *testing.T) {
	backend := &fakeBackend{}
	srv := wsServer(t, backend, &fakeVideo{frames: make(chan []byte)})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	first, _, err := websocket.Dial(ctx, wsAddr(srv, "/api/devices/AAA/input"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if err := first.Write(ctx, websocket.MessageBinary,
		input.Touch(input.TouchDown, 0.5, 0.5, input.EdgeNone)); err != nil {
		t.Fatal(err)
	}
	for len(backend.sentFrames()) < 1 {
		time.Sleep(5 * time.Millisecond)
	}
	first.CloseNow()

	deadline := time.After(5 * time.Second)
	for {
		next, _, err := websocket.Dial(ctx, wsAddr(srv, "/api/devices/AAA/input"), nil)
		if err == nil {
			// Dial succeeds either way; prove it was not immediately closed.
			werr := next.Write(ctx, websocket.MessageBinary,
				input.Touch(input.TouchDown, 0.1, 0.1, input.EdgeNone))
			if werr == nil {
				if _, _, rerr := next.Read(ctx); rerr == nil ||
					websocket.CloseStatus(rerr) != websocket.StatusPolicyViolation {
					next.Close(websocket.StatusNormalClosure, "")
					return
				}
			}
			next.CloseNow()
		}
		select {
		case <-deadline:
			t.Fatal("device never became available after the driver disconnected")
		case <-time.After(50 * time.Millisecond):
		}
	}
}
