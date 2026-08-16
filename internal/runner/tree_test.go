package runner

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// stubRunner mimics devicelab-ios-runner's /command endpoint.
func stubRunner(t *testing.T, respond func(cmd map[string]any) string) *TreeClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var cmd map[string]any
		_ = json.Unmarshal(body, &cmd)
		_, _ = w.Write([]byte(respond(cmd)))
	}))
	t.Cleanup(server.Close)
	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)
	return NewTreeClient(host, port)
}

func TestSnapshotConvertsNodes(t *testing.T) {
	var gotCmd map[string]any
	tree := stubRunner(t, func(cmd map[string]any) string {
		gotCmd = cmd
		return `{"ok": true, "data": {"nodes": [
			{"index": 0, "type": "Application", "rect": {"x":0,"y":0,"width":402,"height":874},
			 "enabled": true, "hittable": true, "depth": 0},
			{"index": 1, "type": "Button", "label": "Log in", "identifier": "loginButton",
			 "value": "v", "placeholderValue": "ph",
			 "rect": {"x":10,"y":20,"width":100,"height":44},
			 "enabled": true, "focused": true, "selected": true, "hittable": true,
			 "depth": 1, "parentIndex": 0}
		]}}`
	})

	nodes, err := tree.Snapshot(context.Background(), "com.example.app")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if gotCmd["command"] != "snapshot" || gotCmd["appBundleId"] != "com.example.app" {
		t.Errorf("runner received %v", gotCmd)
	}
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(nodes))
	}
	btn := nodes[1]
	if btn.Type != "Button" || btn.Label != "Log in" || btn.Identifier != "loginButton" ||
		btn.Value != "v" || btn.Placeholder != "ph" || !btn.Focused || !btn.Selected {
		t.Errorf("button = %+v", btn)
	}
	if btn.Frame != (Rect{X: 10, Y: 20, Width: 100, Height: 44}) {
		t.Errorf("frame = %+v", btn.Frame)
	}
	if btn.ParentIndex == nil || *btn.ParentIndex != 0 {
		t.Errorf("parentIndex = %v", btn.ParentIndex)
	}
}

func TestSnapshotRunnerError(t *testing.T) {
	tree := stubRunner(t, func(map[string]any) string {
		return `{"ok": false, "error": {"code": "APP_NOT_RUNNING", "message": "nope"}}`
	})
	if _, err := tree.Snapshot(context.Background(), ""); err == nil {
		t.Fatal("expected runner error")
	}
}

func TestSnapshotEmptyData(t *testing.T) {
	tree := stubRunner(t, func(map[string]any) string { return `{"ok": true}` })
	if _, err := tree.Snapshot(context.Background(), ""); err == nil {
		t.Fatal("expected empty-response error")
	}
}

func TestSnapshotUnreachableRunner(t *testing.T) {
	tree := NewTreeClient("127.0.0.1", 1) // nothing listens on port 1
	if _, err := tree.Snapshot(context.Background(), ""); err == nil {
		t.Fatal("expected connection error")
	}
}

func TestConvertNodesEmpty(t *testing.T) {
	if got := convertNodes(nil); len(got) != 0 {
		t.Errorf("convertNodes(nil) = %v", got)
	}
}

type fakeEngine struct {
	nodes   []Node
	stopped bool
}

func (f *fakeEngine) Snapshot(context.Context, string) ([]Node, error) { return f.nodes, nil }
func (f *fakeEngine) Stop(context.Context) error                       { f.stopped = true; return nil }

func TestEnginesCachesPerUDID(t *testing.T) {
	started := map[string]int{}
	engines := map[string]*fakeEngine{
		"AAA": {nodes: []Node{{Type: "Application"}}},
		"BBB": {nodes: []Node{{Type: "Button"}}},
	}
	s := &Engines{
		start: func(_ context.Context, udid string) (engineAPI, error) {
			started[udid]++
			return engines[udid], nil
		},
		engines: make(map[string]engineAPI),
	}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		nodes, err := s.Snapshot(ctx, "AAA", "")
		if err != nil || nodes[0].Type != "Application" {
			t.Fatalf("AAA snapshot: %v %v", nodes, err)
		}
	}
	if nodes, _ := s.Snapshot(ctx, "BBB", ""); nodes[0].Type != "Button" {
		t.Fatalf("BBB snapshot routed wrong")
	}
	if started["AAA"] != 1 || started["BBB"] != 1 {
		t.Errorf("start counts = %v, want one each", started)
	}

	s.StopAll(ctx)
	if !engines["AAA"].stopped || !engines["BBB"].stopped {
		t.Error("StopAll did not stop cached engines")
	}
	if _, err := s.Snapshot(ctx, "AAA", ""); err != nil || started["AAA"] != 2 {
		t.Errorf("expected fresh start after StopAll, starts=%v err=%v", started, err)
	}
}

func TestEnginesStartFailure(t *testing.T) {
	s := &Engines{
		start: func(context.Context, string) (engineAPI, error) {
			return nil, context.DeadlineExceeded
		},
		engines: make(map[string]engineAPI),
	}
	if _, err := s.Snapshot(context.Background(), "AAA", ""); err == nil {
		t.Fatal("expected start error")
	}
	if len(s.engines) != 0 {
		t.Error("failed start must not be cached")
	}
}
