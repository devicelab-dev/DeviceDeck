package runner

import (
	"context"
	"errors"
	"fmt"
	"testing"

	dlios "github.com/devicelab-dev/maestro-runner/pkg/driver/devicelab_ios"
)

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
				err: fmt.Errorf("runner snapshot: %w", &dlios.AgentError{Code: "APP_NOT_RUNNING", Message: "nope"}),
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

func TestWarmStartsOnceAndReportsFailure(t *testing.T) {
	started := 0
	s := &Engines{start: startSequence(&started, &fakeEngine{}), engines: make(map[string]engineAPI)}
	for range 2 {
		if err := s.Warm(context.Background(), "AAA"); err != nil {
			t.Fatal(err)
		}
	}
	if started != 1 {
		t.Errorf("engine started %d times, want 1", started)
	}
	failing := &Engines{
		start:   func(context.Context, string) (engineAPI, error) { return nil, errors.New("no device") },
		engines: make(map[string]engineAPI),
	}
	if err := failing.Warm(context.Background(), "BBB"); err == nil {
		t.Error("a failed start must be reported")
	}
}

func TestNotBooted(t *testing.T) {
	for msg, want := range map[string]bool{
		"simctl install: exit status 149 (…):\nUnable to lookup in current state: Shutdown":      true,
		"simctl install: exit status 149 (…):\nUnable to lookup in current state: Shutting Down": true,
		"xcodebuild: test runner exited early":                                                   false,
	} {
		if got := notBooted(errors.New(msg)); got != want {
			t.Errorf("notBooted(%q) = %v, want %v", msg, got, want)
		}
	}
}

// A simulator that is not running is expected (a tab asking before boot):
// the start still fails, and is not cached.
func TestEnginesStartOnShutdownDevice(t *testing.T) {
	s := &Engines{
		start: func(context.Context, string) (engineAPI, error) {
			return nil, errors.New("Unable to lookup in current state: Shutdown")
		},
		engines: make(map[string]engineAPI),
	}
	if _, err := s.Snapshot(context.Background(), "AAA", ""); err == nil || !notBooted(err) {
		t.Fatalf("want the not-booted error back, got %v", err)
	}
	if len(s.engines) != 0 {
		t.Error("failed start must not be cached")
	}
}

// Stop ends one device's engine and leaves the others running.
func TestEnginesStop(t *testing.T) {
	a, b := &fakeEngine{stopErr: errors.New("xcodebuild already gone")}, &fakeEngine{}
	s := &Engines{engines: map[string]engineAPI{"AAA": a, "BBB": b}}
	s.Stop(context.Background(), "AAA")
	s.Stop(context.Background(), "CCC") // nothing running there: a no-op
	if !a.stopped || b.stopped {
		t.Errorf("stopped AAA=%v BBB=%v, want only AAA", a.stopped, b.stopped)
	}
	if _, ok := s.engines["AAA"]; ok || len(s.engines) != 1 {
		t.Errorf("engines = %v, want only BBB left", s.engines)
	}
}

// idlingFake is an engine that can wait for its app (iOS).
type idlingFake struct {
	fakeEngine
	idled []string
}

func (f *idlingFake) Idle(_ context.Context, app string) error {
	f.idled = append(f.idled, app)
	return nil
}

func TestEnginesIdle(t *testing.T) {
	ios := &idlingFake{}
	s := &Engines{engines: map[string]engineAPI{"ios": ios, "android": &fakeEngine{}},
		start: func(context.Context, string) (engineAPI, error) { return nil, errors.New("no device") }}
	if err := s.Idle(context.Background(), "ios", "com.example"); err != nil || len(ios.idled) != 1 {
		t.Errorf("ios idle: err %v, idled %v", err, ios.idled)
	}
	if err := s.Idle(context.Background(), "android", "com.example"); err != nil {
		t.Errorf("an engine without idle returns at once: %v", err)
	}
	if err := s.Idle(context.Background(), "gone", "com.example"); err == nil {
		t.Error("a start failure must surface")
	}
}

// releasingFake is an engine that can be released (the iOS agent).
type releasingFake struct {
	fakeEngine
	released bool
}

func (f *releasingFake) Release(context.Context) { f.released = true }

// On exit a releasable engine is released, not stopped; others are stopped.
func TestStopAllReleasesWhereItCan(t *testing.T) {
	ios, android := &releasingFake{}, &fakeEngine{}
	s := &Engines{engines: map[string]engineAPI{"ios": ios, "android": android}}
	s.StopAll(context.Background())
	if !ios.released || ios.stopped {
		t.Errorf("iOS engine: released=%v stopped=%v, want released only", ios.released, ios.stopped)
	}
	if !android.stopped {
		t.Error("an engine that cannot be released must be stopped")
	}
	if len(s.engines) != 0 {
		t.Errorf("cache not cleared: %v", s.engines)
	}
}
