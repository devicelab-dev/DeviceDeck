package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/input"
	"github.com/devicelab-dev/DeviceDeck/internal/runner"
	"github.com/devicelab-dev/DeviceDeck/internal/sim"
)

type fakeBackend struct {
	devices    []sim.Device
	devicesErr error
	png        []byte
	pngErr     error
	nodes      []runner.Node
	nodesErr   error
	treeApp    string
	framesErr  error
	// failAfter, when > 0, makes SendFrame fail once that many frames
	// have been accepted — exercises mid-gesture sidecar death.
	failAfter int

	// mu guards frames/frameUDID: WebSocket handlers call SendFrame from
	// server goroutines while tests poll the captured frames.
	mu        sync.Mutex
	frames    [][]byte
	frameUDID string
}

func (f *fakeBackend) sentFrames() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]byte(nil), f.frames...)
}

func (f *fakeBackend) sentUDID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.frameUDID
}

func (f *fakeBackend) Booted(context.Context) ([]sim.Device, error) {
	return f.devices, f.devicesErr
}

func (f *fakeBackend) Screenshot(_ context.Context, udid string) ([]byte, error) {
	return f.png, f.pngErr
}

func (f *fakeBackend) Snapshot(_ context.Context, udid, app string) ([]runner.Node, error) {
	f.treeApp = app
	return f.nodes, f.nodesErr
}

func (f *fakeBackend) SendFrame(_ context.Context, udid string, frame []byte) error {
	if f.framesErr != nil {
		return f.framesErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAfter > 0 && len(f.frames) >= f.failAfter {
		return errors.New("sidecar died mid-gesture")
	}
	f.frameUDID = udid
	f.frames = append(f.frames, frame)
	return nil
}

func newTestServer(f *fakeBackend) *Server {
	s := New(f, f, f, f, &fakeVideo{frames: make(chan []byte)})
	s.sleep = func(time.Duration) {}
	return s
}

func do(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestDevices(t *testing.T) {
	f := &fakeBackend{devices: []sim.Device{{UDID: "AAA", Name: "iPhone 16 Pro", OS: "iOS 18.6", Booted: true}}}
	rec := do(t, newTestServer(f), "GET", "/api/devices", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var got struct{ Devices []sim.Device }
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Devices) != 1 || got.Devices[0].UDID != "AAA" {
		t.Errorf("devices = %+v", got.Devices)
	}
}

func TestDevicesEmptyAndError(t *testing.T) {
	rec := do(t, newTestServer(&fakeBackend{}), "GET", "/api/devices", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"devices":[]`) {
		t.Errorf("empty list: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, newTestServer(&fakeBackend{devicesErr: errors.New("boom")}), "GET", "/api/devices", "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("error status = %d", rec.Code)
	}
}

func TestScreenshot(t *testing.T) {
	f := &fakeBackend{png: []byte{0x89, 'P', 'N', 'G'}}
	rec := do(t, newTestServer(f), "GET", "/api/devices/AAA/screenshot", "")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("status %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	rec = do(t, newTestServer(&fakeBackend{pngErr: errors.New("no sim")}), "GET", "/api/devices/AAA/screenshot", "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("error status = %d", rec.Code)
	}
}

func TestTree(t *testing.T) {
	f := &fakeBackend{nodes: []runner.Node{{Index: 0, Type: "Application"}}}
	rec := do(t, newTestServer(f), "GET", "/api/devices/AAA/tree?app=com.example", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if f.treeApp != "com.example" {
		t.Errorf("app passed = %q", f.treeApp)
	}
	rec = do(t, newTestServer(&fakeBackend{}), "GET", "/api/devices/AAA/tree", "")
	if !strings.Contains(rec.Body.String(), `"nodes":[]`) {
		t.Errorf("empty tree: %s", rec.Body)
	}
	rec = do(t, newTestServer(&fakeBackend{nodesErr: errors.New("runner down")}), "GET", "/api/devices/AAA/tree", "")
	if rec.Code != http.StatusBadGateway {
		t.Errorf("error status = %d", rec.Code)
	}
}

func TestTap(t *testing.T) {
	f := &fakeBackend{}
	rec := do(t, newTestServer(f), "POST", "/api/devices/AAA/tap", `{"x":0.5,"y":0.25}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if f.frameUDID != "AAA" || len(f.frames) != 2 {
		t.Fatalf("frames = %d to %q", len(f.frames), f.frameUDID)
	}
	wantDown := input.Touch(input.TouchDown, 0.5, 0.25, input.EdgeNone)
	wantUp := input.Touch(input.TouchUp, 0.5, 0.25, input.EdgeNone)
	if string(f.frames[0]) != string(wantDown) || string(f.frames[1]) != string(wantUp) {
		t.Errorf("frames = %x / %x", f.frames[0], f.frames[1])
	}
}

func TestTapValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"bad json", "{", http.StatusBadRequest},
		{"x out of range", `{"x":1.5,"y":0.5}`, http.StatusBadRequest},
		{"y negative", `{"x":0.5,"y":-0.1}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, newTestServer(&fakeBackend{}), "POST", "/api/devices/AAA/tap", tt.body)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestTapSidecarFailure(t *testing.T) {
	f := &fakeBackend{framesErr: errors.New("sidecar dead")}
	rec := do(t, newTestServer(f), "POST", "/api/devices/AAA/tap", `{"x":0.5,"y":0.5}`)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestSwipe(t *testing.T) {
	f := &fakeBackend{}
	rec := do(t, newTestServer(f), "POST", "/api/devices/AAA/swipe",
		`{"x1":0.5,"y1":0.8,"x2":0.5,"y2":0.2,"durationMs":100}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	// down + 10 moves + up
	if len(f.frames) != 12 {
		t.Fatalf("frames = %d, want 12", len(f.frames))
	}
	if f.frames[0][0] != 0x01 || f.frames[11][0] != 0x03 {
		t.Errorf("first/last frame types = %x / %x", f.frames[0][0], f.frames[11][0])
	}
	for i := 1; i <= 10; i++ {
		if f.frames[i][0] != 0x02 {
			t.Errorf("frame %d type = %x, want move", i, f.frames[i][0])
		}
	}
}

func TestTapUpFrameFailure(t *testing.T) {
	f := &fakeBackend{failAfter: 1} // down succeeds, up fails
	rec := do(t, newTestServer(f), "POST", "/api/devices/AAA/tap", `{"x":0.5,"y":0.5}`)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestSwipeMidGestureFailures(t *testing.T) {
	// Move frame fails (down + 2 moves accepted, third move dies).
	f := &fakeBackend{failAfter: 3}
	rec := do(t, newTestServer(f), "POST", "/api/devices/AAA/swipe", `{"x1":0.1,"y1":0.1,"x2":0.9,"y2":0.9}`)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("move failure status = %d", rec.Code)
	}
	// Up frame fails (down + 10 moves accepted).
	f = &fakeBackend{failAfter: 11}
	rec = do(t, newTestServer(f), "POST", "/api/devices/AAA/swipe", `{"x1":0.1,"y1":0.1,"x2":0.9,"y2":0.9}`)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("up failure status = %d", rec.Code)
	}
}

func TestSwipeValidation(t *testing.T) {
	rec := do(t, newTestServer(&fakeBackend{}), "POST", "/api/devices/AAA/swipe", `{"x1":2,"y1":0,"x2":0,"y2":0}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d", rec.Code)
	}
	rec = do(t, newTestServer(&fakeBackend{framesErr: errors.New("dead")}), "POST", "/api/devices/AAA/swipe", `{"x1":0.1,"y1":0.1,"x2":0.9,"y2":0.9}`)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("sidecar failure status = %d", rec.Code)
	}
}

func TestGesture(t *testing.T) {
	for kind, g := range gestureKinds {
		f := &fakeBackend{}
		rec := do(t, newTestServer(f), "POST", "/api/devices/AAA/gesture", fmt.Sprintf(`{"kind":%q}`, kind))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", kind, rec.Code)
		}
		if len(f.frames) != 1 || string(f.frames[0]) != string(input.SystemGesture(g)) {
			t.Errorf("%s: frame = %x", kind, f.frames)
		}
	}
	rec := do(t, newTestServer(&fakeBackend{}), "POST", "/api/devices/AAA/gesture", `{"kind":"backflip"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown gesture status = %d", rec.Code)
	}
	rec = do(t, newTestServer(&fakeBackend{framesErr: errors.New("dead")}), "POST", "/api/devices/AAA/gesture", `{"kind":"home"}`)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("sidecar failure status = %d", rec.Code)
	}
}

func TestKey(t *testing.T) {
	f := &fakeBackend{}
	rec := do(t, newTestServer(f), "POST", "/api/devices/AAA/key", `{"usage":4,"modifiers":2}`)
	if rec.Code != http.StatusOK || string(f.frames[0]) != string(input.Key(2, 4)) {
		t.Errorf("status %d frame %x", rec.Code, f.frames)
	}
	rec = do(t, newTestServer(&fakeBackend{}), "POST", "/api/devices/AAA/key", `{"modifiers":2}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing usage status = %d", rec.Code)
	}
	rec = do(t, newTestServer(&fakeBackend{framesErr: errors.New("dead")}), "POST", "/api/devices/AAA/key", `{"usage":4}`)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("sidecar failure status = %d", rec.Code)
	}
}

func TestButton(t *testing.T) {
	f := &fakeBackend{}
	rec := do(t, newTestServer(f), "POST", "/api/devices/AAA/button", `{"button":"home"}`)
	if rec.Code != http.StatusOK || string(f.frames[0]) != string(input.LegacyButton(0)) {
		t.Errorf("home: status %d frame %x", rec.Code, f.frames)
	}

	f = &fakeBackend{}
	rec = do(t, newTestServer(f), "POST", "/api/devices/AAA/button", `{"page":12,"usage":233}`)
	if rec.Code != http.StatusOK || string(f.frames[0]) != string(input.ButtonPress(12, 233)) {
		t.Errorf("hid: status %d frame %x", rec.Code, f.frames)
	}

	for _, body := range []string{`{"button":"snooze"}`, `{}`, `{"page":12}`} {
		rec = do(t, newTestServer(&fakeBackend{}), "POST", "/api/devices/AAA/button", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s status = %d", body, rec.Code)
		}
	}
	rec = do(t, newTestServer(&fakeBackend{framesErr: errors.New("dead")}), "POST", "/api/devices/AAA/button", `{"button":"lock"}`)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("sidecar failure status = %d", rec.Code)
	}
}

func TestBadBodiesRejectedEverywhere(t *testing.T) {
	for _, path := range []string{"swipe", "gesture", "key", "button"} {
		rec := do(t, newTestServer(&fakeBackend{}), "POST", "/api/devices/AAA/"+path, "{")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d", path, rec.Code)
		}
	}
}

func TestHelpers(t *testing.T) {
	if durationOrDefault(0, 60) != 60*time.Millisecond {
		t.Error("default duration")
	}
	if durationOrDefault(120, 60) != 120*time.Millisecond {
		t.Error("explicit duration")
	}
	if validNorm(-0.01) || validNorm(1.01) || !validNorm(0) || !validNorm(1) {
		t.Error("validNorm bounds")
	}
}
