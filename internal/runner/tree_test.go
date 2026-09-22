package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	dlios "github.com/devicelab-dev/maestro-runner/pkg/driver/devicelab_ios"
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
	stopErr error
	nodes   []Node
	err     error
	stopped bool
}

func (f *fakeEngine) Snapshot(context.Context, string) ([]Node, error) { return f.nodes, f.err }
func (f *fakeEngine) SnapshotState(context.Context, string) (Snapshot, error) {
	return Snapshot{Nodes: f.nodes}, f.err
}
func (f *fakeEngine) Stop(context.Context) error { f.stopped = true; return f.stopErr }

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

// startSequence returns a start func handing out the given engines in order,
// counting calls; starts beyond the sequence fail.
func startSequence(started *int, engines ...engineAPI) func(context.Context, string) (engineAPI, error) {
	return func(context.Context, string) (engineAPI, error) {
		*started++
		if *started > len(engines) {
			return nil, errors.New("no more engines")
		}
		return engines[*started-1], nil
	}
}

func TestEnginesSnapshotRecovery(t *testing.T) {
	snapErr := errors.New("connection refused")
	tests := []struct {
		name        string
		engines     []engineAPI
		ctx         func() context.Context
		wantErr     bool
		wantStarts  int
		wantStopped bool // first engine stopped by eviction
	}{
		{
			name:        "dead engine evicted and retried on fresh one",
			engines:     []engineAPI{&fakeEngine{err: snapErr}, &fakeEngine{nodes: []Node{{Type: "Application"}}}},
			ctx:         context.Background,
			wantStarts:  2,
			wantStopped: true,
		},
		{
			name:        "retry on fresh engine also fails",
			engines:     []engineAPI{&fakeEngine{err: snapErr}, &fakeEngine{err: snapErr}},
			ctx:         context.Background,
			wantErr:     true,
			wantStarts:  2,
			wantStopped: true,
		},
		{
			name:        "restart failure is reported",
			engines:     []engineAPI{&fakeEngine{err: snapErr}},
			ctx:         context.Background,
			wantErr:     true,
			wantStarts:  2,
			wantStopped: true,
		},
		{
			name: "runner-level error does not trigger a restart",
			engines: []engineAPI{&fakeEngine{
				err: fmt.Errorf("runner snapshot: %w", &dlios.RunnerError{Code: "APP_NOT_RUNNING", Message: "nope"}),
			}},
			ctx:        context.Background,
			wantErr:    true,
			wantStarts: 1,
		},
		{
			name:    "cancelled context does not trigger a restart",
			engines: []engineAPI{&fakeEngine{err: snapErr}},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr:    true,
			wantStarts: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			started := 0
			s := &Engines{start: startSequence(&started, tt.engines...), engines: make(map[string]engineAPI)}
			nodes, err := s.Snapshot(tt.ctx(), "AAA", "")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Snapshot err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && nodes[0].Type != "Application" {
				t.Errorf("nodes = %v, want recovered snapshot", nodes)
			}
			if started != tt.wantStarts {
				t.Errorf("starts = %d, want %d", started, tt.wantStarts)
			}
			if got := tt.engines[0].(*fakeEngine).stopped; got != tt.wantStopped {
				t.Errorf("first engine stopped = %v, want %v", got, tt.wantStopped)
			}
		})
	}
}

func TestEvictSkipsReplacedEngine(t *testing.T) {
	dead := &fakeEngine{err: errors.New("dead")}
	replacement := &fakeEngine{}
	s := &Engines{engines: map[string]engineAPI{"AAA": replacement}}
	s.evict(context.Background(), "AAA", dead)
	if dead.stopped || replacement.stopped {
		t.Errorf("stopped: dead=%v replacement=%v, want neither", dead.stopped, replacement.stopped)
	}
	if s.engines["AAA"] != engineAPI(replacement) {
		t.Error("replacement engine must stay cached")
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

// ActiveUDIDs lists exactly the devices an engine has been started on.
func TestEnginesActiveUDIDs(t *testing.T) {
	s := &Engines{
		start: func(_ context.Context, udid string) (engineAPI, error) {
			return &fakeEngine{nodes: []Node{{Type: "Application"}}}, nil
		},
		engines: make(map[string]engineAPI),
	}
	ctx := context.Background()
	if got := s.ActiveUDIDs(); len(got) != 0 {
		t.Fatalf("fresh engines has %v, want none", got)
	}
	_, _ = s.Snapshot(ctx, "AAA", "")
	_, _ = s.Snapshot(ctx, "BBB", "")
	got := s.ActiveUDIDs()
	set := map[string]bool{}
	for _, u := range got {
		set[u] = true
	}
	if len(got) != 2 || !set["AAA"] || !set["BBB"] {
		t.Errorf("ActiveUDIDs = %v, want AAA and BBB", got)
	}
}

// A failing Stop is logged, never allowed to keep a dead engine cached.
func TestStopFailuresStillClearTheCache(t *testing.T) {
	failing := func() *fakeEngine { return &fakeEngine{stopErr: errors.New("xcodebuild already gone")} }
	a, b := failing(), failing()
	s := &Engines{engines: map[string]engineAPI{"AAA": a, "BBB": b}}
	s.evict(context.Background(), "AAA", a)
	s.StopAll(context.Background())
	if len(s.engines) != 0 || !a.stopped || !b.stopped {
		t.Fatalf("engines left = %d, stopped a=%v b=%v", len(s.engines), a.stopped, b.stopped)
	}
}
