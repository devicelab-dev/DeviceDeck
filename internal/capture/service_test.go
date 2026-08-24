package capture

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/devicelab-dev/DeviceDeck/internal/input"
	"github.com/devicelab-dev/DeviceDeck/internal/runner"
)

type fakeTrees struct {
	tree []runner.Node
	err  error
}

func (f *fakeTrees) Snapshot(context.Context, string, string) ([]runner.Node, error) {
	return f.tree, f.err
}

func TestServiceLifecycle(t *testing.T) {
	svc := NewService(&fakeTrees{tree: testTree()})
	ctx := context.Background()

	if recording, _ := svc.Status("UDID-1"); recording {
		t.Fatal("recording before Start")
	}
	if err := svc.Start(ctx, "UDID-1", "com.example.app"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := svc.Start(ctx, "UDID-1", "com.example.app"); err == nil {
		t.Fatal("double Start must fail")
	}

	// Tap the login button through the frame path.
	svc.OnFrame("UDID-1", input.Touch(input.TouchDown, 0.5, 0.486, input.EdgeNone))
	svc.OnFrame("UDID-1", input.Touch(input.TouchUp, 0.5, 0.486, input.EdgeNone))
	svc.OnFrame("UDID-1", []byte{0xFF, 0xFF})                                     // malformed: ignored
	svc.OnFrame("UDID-2", input.Touch(input.TouchDown, 0.5, 0.5, input.EdgeNone)) // not recording

	recording, steps := svc.Status("UDID-1")
	if !recording || len(steps) != 1 || steps[0].ID != "loginButton" {
		t.Fatalf("status = %v %+v", recording, steps)
	}

	yaml, guard, steps, err := svc.Stop("UDID-1")
	if err != nil || len(steps) != 1 {
		t.Fatalf("Stop: %v %+v", err, steps)
	}
	if !strings.Contains(yaml, `id: "loginButton"`) {
		t.Errorf("yaml missing selector:\n%s", yaml)
	}
	if _, err := runner.ValidateFlow([]byte(yaml)); err != nil {
		t.Errorf("exported yaml invalid: %v", err)
	}
	// The guard is a separate artifact covering the same steps.
	if !strings.Contains(guard, `"version": 1`) || !strings.Contains(guard, `"kind": "tapOn"`) {
		t.Errorf("guard missing or wrong shape:\n%s", guard)
	}

	if _, _, _, err := svc.Stop("UDID-1"); err == nil {
		t.Fatal("Stop without recording must fail")
	}
	if recording, _ := svc.Status("UDID-1"); recording {
		t.Fatal("still recording after Stop")
	}
}

func TestServiceStartFailsWhenTreeUnavailable(t *testing.T) {
	svc := NewService(&fakeTrees{err: errors.New("runner down")})
	if err := svc.Start(context.Background(), "UDID-1", "app"); err == nil {
		t.Fatal("expected error")
	}
}

// Assert is refused when the device is not recording, and when the point
// resolves to nothing durable.
func TestServiceAssert(t *testing.T) {
	svc := NewService(&fakeTrees{tree: []runner.Node{
		{Index: 0, Type: "Application", Frame: runner.Rect{Width: 100, Height: 200}},
		{Index: 1, Type: "Button", Identifier: "ok", Depth: 1,
			Frame: runner.Rect{X: 0, Y: 0, Width: 50, Height: 30}},
	}})
	if err := svc.Assert("UDID-1", 0.25, 0.075); err == nil {
		t.Fatal("assert without a recording must fail")
	}
	if err := svc.Start(context.Background(), "UDID-1", "com.example"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := svc.Assert("UDID-1", 0.25, 0.075); err != nil {
		t.Fatalf("Assert: %v", err)
	}
	if err := svc.Assert("UDID-1", 0.9, 0.9); err == nil {
		t.Fatal("assert on empty space must fail rather than record a coordinate")
	}
	_, _, steps, err := svc.Stop("UDID-1")
	if err != nil || len(steps) != 1 || steps[0].Kind != "assertVisible" {
		t.Fatalf("steps = %+v (err %v)", steps, err)
	}
}

// trapTrees runs a hook during Snapshot so a test can simulate work — such
// as a rival Start winning the device — happening while a Start is fetching
// the initial tree outside the service lock.
type trapTrees struct {
	tree   []runner.Node
	onSnap func()
}

func (f trapTrees) Snapshot(context.Context, string, string) ([]runner.Node, error) {
	if f.onSnap != nil {
		f.onSnap()
	}
	return f.tree, nil
}

// Start snapshots the initial tree outside the lock, so a concurrent Start
// can claim the device in that window. The re-check after the snapshot must
// catch it and refuse rather than clobber the live recording.
func TestStartLosesRaceToConcurrentRecorder(t *testing.T) {
	svc := NewService(nil)
	var once sync.Once
	svc.trees = trapTrees{tree: testTree(), onSnap: func() {
		once.Do(func() {
			svc.mu.Lock()
			svc.recorders["UDID-1"] = &Recorder{} // rival won the device
			svc.mu.Unlock()
		})
	}}
	if err := svc.Start(context.Background(), "UDID-1", "app"); err == nil {
		t.Fatal("Start must lose to the concurrent recorder")
	}
}
