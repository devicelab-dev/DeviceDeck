package input

import (
	"context"
	"errors"
	"testing"
)

// Flush pushes typed text out of the Android batch at once, so the server
// can record an action as delivered only when it is on the device.
func TestRouterFlush(t *testing.T) {
	ctx := context.Background()
	inj := &fakeInjector{w: 100, h: 100}
	resolveErr := error(nil)
	r := NewRouter(nil, func(context.Context, string) (AndroidInjector, error) {
		return inj, resolveErr
	})

	// iOS frames are never buffered, and a device with no input yet has
	// nothing to flush.
	if err := r.Flush(ctx, "IOS-UDID-1"); err != nil {
		t.Errorf("iOS flush: %v", err)
	}
	if err := r.Flush(ctx, "emulator-5554"); err != nil {
		t.Errorf("flush before any input: %v", err)
	}

	for _, usage := range []uint32{0x04, 0x05, 0x06} { // a b c
		if err := r.SendFrame(ctx, "emulator-5554", Key(0, usage)); err != nil {
			t.Fatalf("key: %v", err)
		}
	}
	if len(inj.calls) != 0 {
		t.Fatalf("text went out before the flush: %v", inj.calls)
	}
	if err := r.Flush(ctx, "emulator-5554"); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if len(inj.calls) != 1 || inj.calls[0] != "text abc" {
		t.Errorf("calls = %v, want the batch sent as one text call", inj.calls)
	}

	resolveErr = errors.New("engine gone")
	if err := r.Flush(ctx, "emulator-5554"); err == nil {
		t.Error("a resolver failure must surface")
	}
}
