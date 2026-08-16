package capture

import (
	"context"
	"errors"
	"strings"
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

	yaml, steps, err := svc.Stop("UDID-1")
	if err != nil || len(steps) != 1 {
		t.Fatalf("Stop: %v %+v", err, steps)
	}
	if !strings.Contains(yaml, `id: "loginButton"`) {
		t.Errorf("yaml missing selector:\n%s", yaml)
	}
	if _, err := runner.ValidateFlow([]byte(yaml)); err != nil {
		t.Errorf("exported yaml invalid: %v", err)
	}

	if _, _, err := svc.Stop("UDID-1"); err == nil {
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
