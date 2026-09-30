package runner

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	dlios "github.com/devicelab-dev/maestro-runner/pkg/driver/devicelab_ios"
)

// agentReq is one command as the devicelab iOS agent receives it.
type agentReq struct {
	Cmd  string         `json:"cmd"`
	Args map[string]any `json:"args"`
}

// stubAgent mimics the agent's HTTP endpoint: respond answers each command
// with a raw response envelope.
func stubAgent(t *testing.T, respond func(req agentReq) string) *TreeClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req agentReq
		_ = json.Unmarshal(body, &req)
		_, _ = w.Write([]byte(respond(req)))
	}))
	t.Cleanup(server.Close)
	_, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)
	return NewTreeClient(port)
}

// recordingAgent is a stubAgent that keeps every command it was sent.
func recordingAgent(t *testing.T, respond func(req agentReq) string) (*Engine, func() []agentReq) {
	t.Helper()
	var mu sync.Mutex
	var sent []agentReq
	tree := stubAgent(t, func(req agentReq) string {
		mu.Lock()
		sent = append(sent, req)
		mu.Unlock()
		return respond(req)
	})
	return &Engine{tree: tree}, func() []agentReq {
		mu.Lock()
		defer mu.Unlock()
		return append([]agentReq(nil), sent...)
	}
}

const twoNodes = `{"ok": true, "data": {"appState": "foreground", "nodes": [
	{"i": 0, "p": -1, "t": "Application", "x": 0, "y": 0, "w": 402, "h": 874, "en": true, "vis": 1},
	{"i": 1, "p": 0, "t": "Button", "label": "Log in", "id": "loginButton", "value": "v",
	 "placeholder": "ph", "x": 10, "y": 20, "w": 100, "h": 44, "en": true, "foc": true, "sel": true, "vis": 1}
]}}`

func TestSnapshotConvertsNodes(t *testing.T) {
	var got agentReq
	tree := stubAgent(t, func(req agentReq) string { got = req; return twoNodes })
	snap, err := tree.SnapshotState(context.Background(), "com.example.app")
	if err != nil {
		t.Fatalf("SnapshotState: %v", err)
	}
	if got.Cmd != "snapshot" || got.Args["app"] != "com.example.app" || got.Args["alerts"] != true {
		t.Errorf("agent received %+v", got)
	}
	if snap.AppState != appForeground || len(snap.Nodes) != 2 {
		t.Fatalf("snap = %+v", snap)
	}
	root, btn := snap.Nodes[0], snap.Nodes[1]
	if root.ParentIndex != nil || root.Depth != 0 {
		t.Errorf("root = %+v", root)
	}
	if btn.Type != "Button" || btn.Label != "Log in" || btn.Identifier != "loginButton" || btn.Value != "v" ||
		btn.Placeholder != "ph" || !btn.Enabled || !btn.Focused || !btn.Selected || !btn.Hittable || btn.Depth != 1 {
		t.Errorf("button = %+v", btn)
	}
	if btn.Frame != (Rect{X: 10, Y: 20, Width: 100, Height: 44}) {
		t.Errorf("frame = %+v", btn.Frame)
	}
	if btn.ParentIndex == nil || *btn.ParentIndex != 0 {
		t.Errorf("parentIndex = %v", btn.ParentIndex)
	}
}

func TestConvertNodes(t *testing.T) {
	if got := convertNodes(nil); len(got) != 0 {
		t.Errorf("convertNodes(nil) = %v", got)
	}
	// Off screen is not hittable; a parent listed after its child (never
	// sent by the agent) is dropped rather than read out of range.
	got := convertNodes([]dlios.Node{{I: 0, P: -1}, {I: 1, P: 0, Vis: 0}, {I: 2, P: 5, Vis: 0.5}})
	if got[1].Hittable || got[1].Depth != 1 || !got[2].Hittable || got[2].ParentIndex != nil {
		t.Errorf("nodes = %+v", got)
	}
}

// A title stands in for a missing label; a label wins over a title.
func TestConvertNodesTitle(t *testing.T) {
	got := convertNodes([]dlios.Node{{I: 0, P: -1, Title: "Settings"}, {I: 1, P: 0, Label: "Back", Title: "Settings"}})
	if got[0].Label != "Settings" || got[1].Label != "Back" {
		t.Errorf("labels = %q, %q", got[0].Label, got[1].Label)
	}
}

func TestSnapshotFailures(t *testing.T) {
	tests := []struct{ name, reply string }{
		{"agent error", `{"ok": false, "error": {"code": "APP_NOT_RUNNING", "message": "nope"}}`},
		{"empty data", `{"ok": true}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := stubAgent(t, func(agentReq) string { return tt.reply })
			if _, err := tree.Snapshot(context.Background(), ""); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	unreachable := NewTreeClient(1) // nothing listens on port 1
	if _, err := unreachable.Snapshot(context.Background(), ""); err == nil {
		t.Fatal("expected connection error")
	}
}

// An app that is not in front is not read: the answer is its state and no
// tree, whether the capture succeeded or the agent could not take it.
func TestSnapshotStateNotFrontmost(t *testing.T) {
	failed := `{"ok": false, "error": {"code": "SNAPSHOT_FAILED", "message": "timed out"}}`
	stateIs := func(s string) string { return `{"ok": true, "data": {"appState": "` + s + `"}}` }
	tests := []struct {
		name, app, snapshot, state, wantState string
		wantErr                               bool
	}{
		{"captured in background", "com.example", `{"ok": true, "data": {"appState": "background", "nodes": [{"i": 0, "p": -1}]}}`, "", "runningBackground", false},
		{"capture failed, backgrounded", "com.example", failed, stateIs("background"), "runningBackground", false},
		{"capture failed, not running", "com.example", failed, stateIs("notRunning"), "notRunning", false},
		{"frontmost but unreadable", "com.example", failed, stateIs("foreground"), "", true},
		{"state unknown", "com.example", failed, stateIs("unknown"), "", true},
		{"state call fails", "com.example", failed, `{"ok": false, "error": {"code": "EXCEPTION", "message": "x"}}`, "", true},
		{"state call empty", "com.example", failed, `{"ok": true}`, "", true},
		{"no app named", "", failed, stateIs("background"), "", true},
		{"another error, app in front", "com.example", `{"ok": false, "error": {"code": "EXCEPTION", "message": "x"}}`, stateIs("foreground"), "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := stubAgent(t, func(req agentReq) string {
				if req.Cmd == "app" {
					return tt.state
				}
				return tt.snapshot
			})
			snap, err := tree.SnapshotState(context.Background(), tt.app)
			if (err != nil) != tt.wantErr || snap.AppState != tt.wantState || len(snap.Nodes) != 0 {
				t.Errorf("snap = %+v, err = %v; want state %q, err %v", snap, err, tt.wantState, tt.wantErr)
			}
		})
	}
}

// A named app already known to be out of front is never captured: the
// agent's first capture of one after it starts hangs.
func TestSnapshotStateAsksStateFirst(t *testing.T) {
	var cmds []string
	tree := stubAgent(t, func(req agentReq) string {
		cmds = append(cmds, req.Cmd)
		if req.Cmd == "app" {
			return `{"ok": true, "data": {"appState": "background"}}`
		}
		return twoNodes
	})
	snap, err := tree.SnapshotState(context.Background(), "com.example")
	if err != nil || snap.AppState != "runningBackground" || len(snap.Nodes) != 0 || strings.Join(cmds, ",") != "app" {
		t.Errorf("snap = %+v, err = %v, commands = %v", snap, err, cmds)
	}
}

// An app that leaves the front between the state check and the capture is
// reported by its new state, not as an error.
func TestSnapshotStateAppLeavesMidCapture(t *testing.T) {
	calls := 0
	tree := stubAgent(t, func(req agentReq) string {
		if req.Cmd != "app" {
			return `{"ok": false, "error": {"code": "SNAPSHOT_FAILED", "message": "timed out"}}`
		}
		calls++
		if calls == 1 {
			return `{"ok": true, "data": {"appState": "foreground"}}`
		}
		return `{"ok": true, "data": {"appState": "background"}}`
	})
	snap, err := tree.SnapshotState(context.Background(), "com.example")
	if err != nil || snap.AppState != "runningBackground" {
		t.Errorf("snap = %+v, err = %v", snap, err)
	}
}

// An unknown state still returns the tree: "cannot tell" is not "gone".
func TestSnapshotUnknownStateKeepsTree(t *testing.T) {
	tree := stubAgent(t, func(agentReq) string {
		return strings.Replace(twoNodes, `"foreground"`, `"unknown"`, 1)
	})
	snap, err := tree.SnapshotState(context.Background(), "com.example")
	if err != nil || snap.AppState != "" || len(snap.Nodes) != 2 {
		t.Errorf("snap = %+v, err = %v", snap, err)
	}
}

func TestEngineDelegatesToAgent(t *testing.T) {
	e, sent := recordingAgent(t, func(req agentReq) string {
		if req.Cmd == "snapshot" {
			return twoNodes
		}
		return `{"ok": true, "data": {"settled": true}}`
	})
	if nodes, err := e.Snapshot(context.Background(), "com.example"); err != nil || len(nodes) != 2 {
		t.Fatalf("Snapshot = %d nodes, %v", len(nodes), err)
	}
	if snap, err := e.SnapshotState(context.Background(), "com.example"); err != nil || snap.AppState != appForeground {
		t.Fatalf("SnapshotState = %+v, %v", snap, err)
	}
	if err := e.Idle(context.Background(), "com.example"); err != nil {
		t.Fatalf("Idle: %v", err)
	}
	all := sent()
	idle := all[len(all)-1]
	if idle.Cmd != "settle" || idle.Args["app"] != "com.example" || idle.Args["timeoutMs"] != float64(idleCapMs) ||
		idle.Args["quiescence"] != "all" {
		t.Errorf("idle = %+v", idle)
	}
}

func TestStartEngineWithoutAgent(t *testing.T) {
	t.Setenv("DEVICELAB_IOS_AGENT_DIR", t.TempDir()) // no manifest: nothing to start
	_, err := StartEngine(context.Background(), "DD000000-0000-4000-8000-000000000001")
	if err == nil || !strings.Contains(err.Error(), "start devicelab iOS agent") {
		t.Fatalf("err = %v", err)
	}
}

// typed answers a type command with the field's value as read back.
func typed(value string) string {
	return `{"ok": true, "data": {"typed": true, "verified": true, "text": ` + strconv.Quote(value) + `}}`
}

const agentOK = `{"ok": true, "data": {"settled": true}}`

func TestIOSFill(t *testing.T) {
	e, sent := recordingAgent(t, func(req agentReq) string {
		if req.Cmd == "type" {
			return typed("devicelab")
		}
		return agentOK
	})
	req := FillRequest{App: "com.example", Identifier: "username-input", X: 201, Y: 450, Text: "devicelab", PrevLen: 30}
	if err := e.Fill(context.Background(), req); err != nil {
		t.Fatalf("Fill: %v", err)
	}
	cmds := sent()
	if len(cmds) != 2 {
		t.Fatalf("commands = %+v", cmds)
	}
	tap, typ := cmds[0], cmds[1]
	if tap.Cmd != "act" || tap.Args["kind"] != "tap" || tap.Args["x"] != 201.0 || tap.Args["y"] != 450.0 || tap.Args["settle"] != true {
		t.Errorf("tap = %+v", tap)
	}
	if typ.Cmd != "type" || typ.Args["app"] != "com.example" || typ.Args["text"] != "devicelab" || typ.Args["erase"] != 38.0 {
		t.Errorf("type = %+v", typ)
	}
}

func TestIOSFillOutcomes(t *testing.T) {
	tests := []struct {
		name, text, tapReply, typeReply, wantErr string
	}{
		{"device holds another value", "devicelab", agentOK, typed("devicelabX"), `device holds "devicelabX"`},
		{"secure field reads as bullets", "abc", agentOK, typed("•••"), ""},
		{"no read-back", "abc", agentOK, `{"ok": true, "data": {"typed": true}}`, ""},
		{"empty answer", "abc", agentOK, `{"ok": true}`, ""},
		{"clear to empty", "", agentOK, `{"ok": true, "data": {"typed": true}}`, ""},
		{"tap fails", "abc", `{"ok": false, "error": {"code": "INPUT_FAILED", "message": "x"}}`, typed("abc"), "focus"},
		{"type fails", "abc", agentOK, `{"ok": false, "error": {"code": "EXCEPTION", "message": "x"}}`, "fill field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, _ := recordingAgent(t, func(req agentReq) string {
				if req.Cmd == "type" {
					return tt.typeReply
				}
				return tt.tapReply
			})
			err := e.Fill(context.Background(), FillRequest{X: 1, Y: 2, Text: tt.text})
			if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// A tap that did not take focus is retried once: tap, type, tap, type.
func TestIOSFillRetapsWithoutFocus(t *testing.T) {
	types := 0
	e, sent := recordingAgent(t, func(req agentReq) string {
		if req.Cmd != "type" {
			return agentOK
		}
		if types++; types == 1 {
			return `{"ok": false, "error": {"code": "NO_FOCUS", "message": "no field has keyboard focus"}}`
		}
		return typed("a")
	})
	if err := e.Fill(context.Background(), FillRequest{X: 1, Y: 2, Text: "a"}); err != nil {
		t.Fatalf("Fill: %v", err)
	}
	var cmds []string
	for _, c := range sent() {
		cmds = append(cmds, c.Cmd)
	}
	if strings.Join(cmds, ",") != "act,type,act,type" {
		t.Errorf("commands = %v", cmds)
	}
}

func TestIOSFillMinimumErase(t *testing.T) {
	e, sent := recordingAgent(t, func(agentReq) string { return agentOK })
	_ = e.Fill(context.Background(), FillRequest{X: 1, Y: 2})
	if got := sent()[1].Args["erase"]; got != float64(clearMinimum) {
		t.Errorf("erase = %v, want %d", got, clearMinimum)
	}
}
