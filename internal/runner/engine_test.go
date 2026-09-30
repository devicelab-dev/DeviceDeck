package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/devicelab-dev/maestro-runner/pkg/device"
	"github.com/devicelab-dev/maestro-runner/pkg/maestro"
)

// fakeDriver stands in for the on-device devicelab Android driver: it
// accepts the WebSocket upgrade (which is all the host's health check asks
// for) and answers each JSON request by method.
type fakeDriver struct {
	mu       sync.Mutex
	failing  map[string]bool
	rejectWS bool
	source   string
	methods  []string
	// badSnapshot answers UI.snapshot with a result that is not an object.
	badSnapshot bool
	// field is the text of the one field this fake holds; ignoreKeys makes
	// Input.sendKeys drop the text, and mask reports it as bullets.
	field      string
	ignoreKeys bool
	mask       bool
	// failSecondFind fails every element lookup after the first — a field
	// found and typed into that can no longer be read back.
	failSecondFind bool
	finds          int
}

func (f *fakeDriver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/ws" && f.rejectWS {
		http.Error(w, "refused", http.StatusInternalServerError)
		return
	}
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = c.CloseNow() }()
	ctx := context.Background()
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		var req struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
			Params struct {
				Text string `json:"text"`
			} `json:"params"`
		}
		_ = json.Unmarshal(data, &req)
		if err := c.Write(ctx, websocket.MessageText, f.reply(req.ID, req.Method, req.Params.Text)); err != nil {
			return
		}
	}
}

func (f *fakeDriver) reply(id int64, method, text string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.methods = append(f.methods, method)
	if f.failing[method] {
		return fmt.Appendf(nil, `{"id":%d,"error":{"code":"ERR","message":"%s failed"}}`, id, method)
	}
	result := `{}`
	switch method {
	case "UI.snapshot":
		b, _ := json.Marshal(map[string]string{"xml": f.source})
		result = string(b)
		if f.badSnapshot {
			result = `"not an object"`
		}
	case "Session.create":
		result = `{"sessionId":"s1"}`
	case "UI.findElement", "UI.activeElement":
		if f.finds++; f.failSecondFind && f.finds > 1 {
			return fmt.Appendf(nil, `{"id":%d,"error":{"code":"ERR","message":"gone"}}`, id)
		}
		shown := f.field
		if f.mask {
			shown = strings.Repeat("•", len([]rune(f.field)))
		}
		b, _ := json.Marshal(map[string]any{"elementId": "el1", "text": shown,
			"bounds": map[string]int{"x": 10, "y": 20, "width": 100, "height": 40}})
		result = string(b)
	case "Input.clearElement":
		f.field = ""
	case "Input.sendKeys":
		if !f.ignoreKeys {
			f.field += text
		}
	}
	return fmt.Appendf(nil, `{"id":%d,"result":%s}`, id, result)
}

func (f *fakeDriver) called(method string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.methods {
		if m == method {
			return true
		}
	}
	return false
}

var serialSeq atomic.Int32

// testSerial is an emulator-style serial no real device has, so the driver
// socket path under /tmp is this test's alone.
func testSerial() string {
	return fmt.Sprintf("emulator-dd%d-%d", os.Getpid(), serialSeq.Add(1))
}

// healthyADB answers like an emulator with the driver installed and running.
// forward records each socket forward, which is when real adb would create
// the driver socket the host then dials.
func healthyADB(tools string, overrides ...toolRule) []toolRule {
	return append(overrides,
		toolRule{"forward --remove", "exit 0"},
		toolRule{"forward localfilesystem", fmt.Sprintf(`echo "$*" >> %q; exit 0`, filepath.Join(tools, "forwards"))},
		toolRule{"get-state", "echo device"},
		toolRule{"pm list packages", `printf 'package:dev.devicelab.driver.android\npackage:dev.devicelab.driver.android.test\n'`},
		toolRule{"ps -A", "echo 'u0_a1 1 dev.devicelab.driver.android'"},
		toolRule{"wm size", "echo 'Physical size: 1080x2340'"},
		toolRule{"default_input_method", "echo com.example.keyboard/.Ime"},
	)
}

// serveDriverOnForward listens on the driver socket each time the fake adb
// records a forward to it, and serves h there — what adb forward plus the
// on-device driver amount to from the host's side.
func serveDriverOnForward(t *testing.T, tools, socket string, h http.Handler) {
	t.Helper()
	stop, done := make(chan struct{}), make(chan struct{})
	var lns []net.Listener
	go func() {
		defer close(done)
		seen := 0
		for {
			select {
			case <-stop:
				return
			case <-time.After(10 * time.Millisecond):
			}
			data, _ := os.ReadFile(filepath.Join(tools, "forwards"))
			if n := strings.Count(string(data), "\n"); n != seen {
				seen = n
				_ = os.Remove(socket)
				if ln, err := net.Listen("unix", socket); err == nil {
					lns = append(lns, ln)
					go func() { _ = http.Serve(ln, h) }()
				}
			}
		}
	}()
	t.Cleanup(func() {
		close(stop)
		<-done
		for _, ln := range lns {
			_ = ln.Close()
		}
		_ = os.Remove(socket)
		_ = os.Remove(strings.TrimSuffix(socket, ".sock") + ".pid")
	})
}

// newTestAndroidEngine assembles an engine against a fake adb and a fake
// driver session, without the start-up sequence.
func newTestAndroidEngine(t *testing.T, drv *fakeDriver, overrides ...toolRule) (*AndroidEngine, string) {
	t.Helper()
	tools := isolateTools(t)
	writeTool(t, tools, "adb", healthyADB(tools, overrides...))
	dev, err := device.New(testSerial())
	if err != nil {
		t.Fatalf("device.New: %v", err)
	}
	srv := httptest.NewServer(drv)
	t.Cleanup(srv.Close)
	_, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	client := maestro.NewClientTCP(port)
	if err := client.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return &AndroidEngine{dev: dev, client: client, adapter: maestro.NewAdapter(client)}, tools
}

func adbCalls(t *testing.T, tools string) string {
	t.Helper()
	data, _ := os.ReadFile(filepath.Join(tools, "adb.calls"))
	return string(data)
}

func TestAndroidEngineSnapshot(t *testing.T) {
	e, tools := newTestAndroidEngine(t, &fakeDriver{source: androidPageXML})
	nodes, err := e.Snapshot(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(nodes) != 7 || nodes[0].Frame.Width != 1080 || nodes[0].Frame.Height != 2340 {
		t.Fatalf("nodes = %d, root = %+v", len(nodes), nodes[0].Frame)
	}
	snap, err := e.SnapshotState(context.Background(), "")
	if err != nil || len(snap.Nodes) != 7 || snap.AppState != "" {
		t.Fatalf("SnapshotState = %d nodes, %q, %v", len(snap.Nodes), snap.AppState, err)
	}
	// The screen size is read once per session, then cached.
	if n := strings.Count(adbCalls(t, tools), "wm size"); n != 1 {
		t.Errorf("wm size queried %d times, want 1", n)
	}
}

func TestAndroidEngineSnapshotErrors(t *testing.T) {
	tests := []struct {
		name     string
		drv      *fakeDriver
		override []toolRule
		want     string
	}{
		{"wm size fails", &fakeDriver{source: androidPageXML}, []toolRule{{"wm size", "exit 1"}}, "wm size"},
		{"wm size unreadable", &fakeDriver{source: androidPageXML}, []toolRule{{"wm size", "echo nothing"}}, "unparseable"},
		{"source fails", &fakeDriver{failing: map[string]bool{"UI.snapshot": true}}, nil, "android page source"},
		{"source unparseable", &fakeDriver{source: "<hierarchy"}, nil, "parse android page source"},
		{"snapshot result not an object", &fakeDriver{badSnapshot: true}, nil, "parse snapshot result"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, _ := newTestAndroidEngine(t, tt.drv, tt.override...)
			_, err := e.Snapshot(context.Background(), "")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestAndroidEngineInput(t *testing.T) {
	drv := &fakeDriver{}
	e, tools := newTestAndroidEngine(t, drv)
	if err := e.Click(10, 20); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if err := e.Swipe(1, 2, 3, 4, 100); err != nil {
		t.Fatalf("Swipe: %v", err)
	}
	if err := e.KeyCode(66); err != nil {
		t.Fatalf("KeyCode: %v", err)
	}
	if err := e.Text("it's me"); err != nil {
		t.Fatalf("Text: %v", err)
	}
	for _, m := range []string{"Gesture.click", "Gesture.swipe", "Input.pressKeyCode"} {
		if !drv.called(m) {
			t.Errorf("driver never received %s", m)
		}
	}
	if !strings.Contains(adbCalls(t, tools), `input text 'it'\''s%sme'`) {
		t.Errorf("adb calls = %q", adbCalls(t, tools))
	}
}

func TestAndroidEngineInputErrors(t *testing.T) {
	drv := &fakeDriver{failing: map[string]bool{"Gesture.click": true, "Gesture.swipe": true, "Input.pressKeyCode": true}}
	e, _ := newTestAndroidEngine(t, drv, toolRule{"input text", "exit 1"})
	if e.Click(1, 1) == nil || e.Swipe(1, 1, 2, 2, 10) == nil || e.KeyCode(4) == nil || e.Text("x") == nil {
		t.Fatal("a failing driver or adb must surface as an error")
	}
}

func TestAndroidEngineStop(t *testing.T) {
	drv := &fakeDriver{}
	e, tools := newTestAndroidEngine(t, drv)
	e.imeBefore = "1"
	if err := e.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !drv.called("Session.delete") {
		t.Error("the driver session was not deleted")
	}
	if !strings.Contains(adbCalls(t, tools), "settings put secure show_ime_with_hard_keyboard 1") {
		t.Error("the soft keyboard setting was not restored")
	}
	if !strings.Contains(adbCalls(t, tools), "am force-stop dev.devicelab.driver.android") {
		t.Error("the driver was not stopped on the device")
	}
}

func TestTuneAndroidSession(t *testing.T) {
	t.Run("applies both settings", func(t *testing.T) {
		drv := &fakeDriver{}
		e, tools := newTestAndroidEngine(t, drv, toolRule{"settings get secure show_ime_with_hard_keyboard", "echo null"})
		if before := tuneAndroidSession(e.dev, e.adapter); before != "null" {
			t.Errorf("setting before = %q, want null", before)
		}
		if !drv.called("Settings.update") {
			t.Error("waitForIdleTimeout was not set")
		}
		if !strings.Contains(adbCalls(t, tools), "show_ime_with_hard_keyboard 0") {
			t.Error("the soft keyboard was not disabled")
		}
	})
	t.Run("failures are best-effort", func(t *testing.T) {
		drv := &fakeDriver{failing: map[string]bool{"Settings.update": true}}
		e, _ := newTestAndroidEngine(t, drv, toolRule{"settings put", "exit 1"})
		tuneAndroidSession(e.dev, e.adapter) // logs, does not fail or panic
	})
}

// Stop puts the soft-keyboard setting back as the session found it, and
// never writes back a value it does not recognise.
func TestRestoreIMESetting(t *testing.T) {
	tests := []struct{ before, want, notWant string }{
		{"null", "settings delete secure show_ime_with_hard_keyboard", "settings put"},
		{"1", "settings put secure show_ime_with_hard_keyboard 1", "settings delete"},
		{"0", "", "settings"},
		{"", "", "settings"},
		{"weird; reboot", "", "settings"},
	}
	for _, tt := range tests {
		t.Run(tt.before, func(t *testing.T) {
			e, tools := newTestAndroidEngine(t, &fakeDriver{})
			restoreIMESetting(e.dev, tt.before)
			calls := adbCalls(t, tools)
			if tt.want != "" && !strings.Contains(calls, tt.want) || strings.Contains(calls, tt.notWant) {
				t.Errorf("adb calls = %q", calls)
			}
		})
	}
	t.Run("failure is best-effort", func(t *testing.T) {
		e, _ := newTestAndroidEngine(t, &fakeDriver{}, toolRule{"settings delete", "exit 1"})
		restoreIMESetting(e.dev, "null") // logs, does not fail or panic
	})
}

func TestStartAndroidEngine(t *testing.T) {
	tests := []struct {
		name     string
		drv      *fakeDriver
		override []toolRule
		want     string
	}{
		{"device offline", &fakeDriver{}, []toolRule{{"get-state", "echo offline"}}, "android device"},
		{"driver install fails", &fakeDriver{}, []toolRule{{" install ", "exit 1"}}, "install devicelab android driver"},
		{"driver not on the device", &fakeDriver{}, []toolRule{{"pm list packages", "exit 0"}}, "start devicelab android driver"},
		{"driver socket refused", &fakeDriver{rejectWS: true}, nil, "connect devicelab android driver"},
		{"session refused", &fakeDriver{failing: map[string]bool{"Session.create": true}}, nil, "create driver session"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tools := isolateTools(t)
			writeTool(t, tools, "adb", healthyADB(tools, tt.override...))
			serial := testSerial()
			serveDriverOnForward(t, tools, "/tmp/devicelab-driver-"+serial+".sock", tt.drv)
			_, err := StartAndroidEngine(context.Background(), serial)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestStartAndroidEngineReady(t *testing.T) {
	tools := isolateTools(t)
	writeTool(t, tools, "adb", healthyADB(tools))
	serial := testSerial()
	drv := &fakeDriver{source: androidPageXML}
	serveDriverOnForward(t, tools, "/tmp/devicelab-driver-"+serial+".sock", drv)

	e, err := StartAndroidEngine(context.Background(), serial)
	if err != nil {
		t.Fatalf("StartAndroidEngine: %v", err)
	}
	if !drv.called("Session.create") || !drv.called("Settings.update") {
		t.Error("the session was not created and tuned")
	}
	if nodes, err := e.Snapshot(context.Background(), ""); err != nil || len(nodes) != 7 {
		t.Errorf("Snapshot = %d nodes, %v", len(nodes), err)
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func TestEnginesAndroidInjector(t *testing.T) {
	android := &AndroidEngine{}
	s := &Engines{engines: map[string]engineAPI{}, start: func(_ context.Context, udid string) (engineAPI, error) {
		switch udid {
		case "emulator-5554":
			return android, nil
		case "sim":
			return &fakeEngine{}, nil
		}
		return nil, fmt.Errorf("no device %s", udid)
	}}
	if got, err := s.AndroidInjector(context.Background(), "emulator-5554"); err != nil || got != android {
		t.Errorf("android = %v, %v", got, err)
	}
	if _, err := s.AndroidInjector(context.Background(), "sim"); err == nil ||
		!strings.Contains(err.Error(), "not an Android engine") {
		t.Errorf("simulator: %v", err)
	}
	if _, err := s.AndroidInjector(context.Background(), "gone"); err == nil {
		t.Error("a start failure must surface")
	}
}

func TestNewEnginesRoutesByPlatform(t *testing.T) {
	tools := isolateTools(t)
	t.Setenv("DEVICELAB_IOS_AGENT_DIR", t.TempDir()) // no agent to start
	writeTool(t, tools, "adb", healthyADB(tools, toolRule{" install ", "exit 1"}))
	writeTool(t, tools, "xcrun", nil) // bootstatus: the simulator is up

	s := NewEngines()
	if s.engines == nil {
		t.Fatal("engine cache not initialised")
	}
	if _, err := s.start(context.Background(), testSerial()); err == nil ||
		!strings.Contains(err.Error(), "install devicelab android driver") {
		t.Errorf("android route: %v", err)
	}
	if _, err := s.start(context.Background(), "DD000000-0000-4000-8000-000000000002"); err == nil ||
		!strings.Contains(err.Error(), "start devicelab iOS agent") {
		t.Errorf("iOS route: %v", err)
	}
	// A simulator that never finishes booting stops the start before the
	// agent is tried (a failed first agent start is sticky in the runner).
	writeTool(t, tools, "xcrun", []toolRule{{"bootstatus", "exit 1"}})
	if _, err := s.start(context.Background(), "DD000000-0000-4000-8000-000000000002"); err == nil ||
		!strings.Contains(err.Error(), "finish booting") {
		t.Errorf("boot wait: %v", err)
	}
}
