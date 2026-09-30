package capture

import (
	"context"
	"testing"

	"github.com/devicelab-dev/DeviceDeck/internal/input"
)

// A browser fill() records as the tap that focuses the field and its whole
// value; typing key by key fills the same field once per key, and those
// collapse into one tap and one inputText.
func TestFillRecordsTapAndText(t *testing.T) {
	rec, _ := newTestRecorder(t, testTree())
	rec.OnFill(0.5, 0.486, "de")
	rec.OnFill(0.5, 0.486, "devicelab")
	// Any other input ends the fill: here, a tap on the banner.
	rec.OnEvent(touch(input.TouchDown, 0.5, 0.137))
	rec.OnEvent(touch(input.TouchUp, 0.5, 0.137))
	steps := rec.Steps()
	if len(steps) != 3 || steps[0].Kind != "tapOn" || steps[0].ID != "loginButton" ||
		steps[1].Kind != "inputText" || steps[1].Input != "devicelab" || steps[2].Kind != "tapOn" {
		t.Fatalf("steps = %+v", steps)
	}
	// A fill after other input taps the field again.
	rec.OnFill(0.5, 0.486, "x")
	rec.OnEvent(touch(input.TouchDown, 0.5, 0.137))
	rec.OnEvent(touch(input.TouchUp, 0.5, 0.137))
	if steps := rec.Steps(); len(steps) != 6 || steps[3].Kind != "tapOn" || steps[4].Input != "x" {
		t.Fatalf("steps after a second fill = %+v", steps)
	}
}

func TestServiceOnFill(t *testing.T) {
	svc := NewService(&fakeTrees{tree: testTree()})
	svc.OnFill("UDID-1", 0.5, 0.486, "ignored") // not recording: a no-op
	if err := svc.Start(context.Background(), "UDID-1", "com.example.app"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	svc.OnFill("UDID-1", 0.5, 0.486, "devicelab")
	_, _, steps, err := svc.Stop("UDID-1")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(steps) != 2 || steps[0].Kind != "tapOn" || steps[1].Input != "devicelab" {
		t.Fatalf("steps = %+v", steps)
	}
}
