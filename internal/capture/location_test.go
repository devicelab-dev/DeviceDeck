package capture

import (
	"context"
	"strings"
	"testing"
)

// A location set while recording is a setLocation step that the real runner
// parses on both platforms, so it replays on any device.
func TestServiceRecordsLocation(t *testing.T) {
	svc := NewService(&fakeTrees{tree: testTree()})
	svc.OnLocation("UDID-1", 1, 2) // not recording: ignored
	if err := svc.Start(context.Background(), "UDID-1", "com.example.app"); err != nil {
		t.Fatal(err)
	}
	svc.OnLocation("UDID-1", 51.5074, -0.1278)
	yaml, _, steps, err := svc.Stop("UDID-1")
	if err != nil || len(steps) != 1 || steps[0].Kind != "setLocation" {
		t.Fatalf("Stop: %v %+v", err, steps)
	}
	if !strings.Contains(yaml, "- setLocation:\n    latitude: 51.5074\n    longitude: -0.1278\n") {
		t.Errorf("yaml:\n%s", yaml)
	}
	if !strings.Contains(yaml, "# devicedeck: setLocation") {
		t.Errorf("provenance line missing:\n%s", yaml)
	}
}
