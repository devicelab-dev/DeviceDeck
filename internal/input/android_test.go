package input

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeInjector records translated driver calls.
type fakeInjector struct {
	calls   []string
	w, h    int
	sizeErr error
	textErr error
}

func (f *fakeInjector) Click(x, y int) error {
	f.calls = append(f.calls, fmt.Sprintf("click %d,%d", x, y))
	return nil
}

func (f *fakeInjector) Swipe(x1, y1, x2, y2, durationMs int) error {
	f.calls = append(f.calls, fmt.Sprintf("swipe %d,%d->%d,%d %dms", x1, y1, x2, y2, durationMs))
	return nil
}

func (f *fakeInjector) Text(text string) error {
	if f.textErr != nil {
		return f.textErr
	}
	f.calls = append(f.calls, "text "+text)
	return nil
}

func (f *fakeInjector) KeyCode(code int) error {
	f.calls = append(f.calls, fmt.Sprintf("key %d", code))
	return nil
}

func (f *fakeInjector) ScreenSize() (int, int, error) {
	if f.sizeErr != nil {
		return 0, 0, f.sizeErr
	}
	return f.w, f.h, nil
}

// send pushes frames through a translator with a fixed clock advancing
// stepMs per call.
func send(t *testing.T, tr *androidTranslator, inj AndroidInjector, frames ...[]byte) {
	t.Helper()
	for _, fr := range frames {
		if err := tr.handle(inj, fr); err != nil {
			t.Fatalf("handle: %v", err)
		}
	}
}

func fixedClock(stepMs int) func() time.Time {
	t0 := time.Unix(0, 0)
	n := 0
	return func() time.Time {
		n++
		return t0.Add(time.Duration(n*stepMs) * time.Millisecond)
	}
}

func TestAndroidTranslatorGestures(t *testing.T) {
	tests := []struct {
		name   string
		frames [][]byte
		want   []string
	}{
		{
			name: "down-up in place is a tap",
			frames: [][]byte{
				Touch(TouchDown, 0.5, 0.5, EdgeNone),
				Touch(TouchUp, 0.5, 0.5, EdgeNone),
			},
			want: []string{"click 540,1170"},
		},
		{
			name: "small travel stays a tap",
			frames: [][]byte{
				Touch(TouchDown, 0.5, 0.5, EdgeNone),
				Touch(TouchMove, 0.505, 0.5, EdgeNone),
				Touch(TouchUp, 0.505, 0.5, EdgeNone),
			},
			want: []string{"click 545,1170"},
		},
		{
			name: "long travel becomes a swipe with measured duration",
			frames: [][]byte{
				Touch(TouchDown, 0.5, 0.8, EdgeNone),
				Touch(TouchMove, 0.5, 0.5, EdgeNone),
				Touch(TouchUp, 0.5, 0.2, EdgeNone),
			},
			// fixedClock steps 100ms per now() call: down stamps t=100,
			// up reads t=200 → 100ms.
			want: []string{"swipe 540,1872->540,468 100ms"},
		},
		{
			name: "typing batches until a control key flushes",
			frames: [][]byte{
				Key(0, 0x04),    // a
				Key(0x02, 0x04), // A (shift)
				Key(0, 0x1e),    // 1
				Key(0, 0x28),    // Enter flushes the batch first
				Key(0, 0x2a),    // Backspace
			},
			want: []string{"text aA1", "key 66", "key 67"},
		},
		{
			name: "touch flushes pending text before the gesture",
			frames: [][]byte{
				Key(0, 0x0b), // h
				Key(0, 0x0c), // i
				Touch(TouchDown, 0.5, 0.5, EdgeNone),
				Touch(TouchUp, 0.5, 0.5, EdgeNone),
			},
			want: []string{"text hi", "click 540,1170"},
		},
		{
			name: "up without down is dropped",
			frames: [][]byte{
				Touch(TouchUp, 0.5, 0.5, EdgeNone),
			},
			want: nil,
		},
		{
			name: "system gestures map to Android keycodes",
			frames: [][]byte{
				SystemGesture(1), // home
				SystemGesture(2), // app switcher
				SystemGesture(3), // notification center
				SystemGesture(4), // lock
			},
			want: []string{"key 3", "key 187", "key 83", "key 26"},
		},
		{
			name: "unknown system gesture is dropped",
			frames: [][]byte{
				SystemGesture(99),
			},
			want: nil,
		},
		{
			name: "legacy home button maps to keycode home",
			frames: [][]byte{
				LegacyButton(0),
			},
			want: []string{"key 3"},
		},
		{
			name: "legacy lock button maps to keycode power",
			frames: [][]byte{
				LegacyButton(1),
			},
			want: []string{"key 26"},
		},
		{
			name: "shifted digit and shifted symbol type their shifted glyphs",
			frames: [][]byte{
				Key(0x02, 0x1e), // shift+1 -> !
				Key(0x02, 0x2d), // shift+- -> _
				Key(0, 0x60),    // unmapped usage: dropped, buffer intact
				Key(0, 0x28),    // Enter flushes the batch
			},
			want: []string{"text !_", "key 66"},
		},
		{
			name: "unsupported frames are dropped",
			frames: [][]byte{
				{0xFF, 0x00},
			},
			want: nil,
		},
		{
			name: "valid but unmapped frames are dropped",
			frames: [][]byte{
				// Two-finger gestures decode fine but have no Android
				// mapping — they hit handle's default and are dropped.
				TwoFinger(TouchMove, 0.1, 0.2, 0.3, 0.4),
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inj := &fakeInjector{w: 1080, h: 2340}
			tr := &androidTranslator{now: fixedClock(100)}
			send(t, tr, inj, tt.frames...)
			if fmt.Sprint(inj.calls) != fmt.Sprint(tt.want) {
				t.Errorf("calls = %v, want %v", inj.calls, tt.want)
			}
		})
	}
}

func TestAndroidTranslatorScreenSizeError(t *testing.T) {
	inj := &fakeInjector{sizeErr: errors.New("wm size broken")}
	tr := &androidTranslator{now: time.Now}
	if err := tr.handle(inj, Touch(TouchDown, 0.5, 0.5, EdgeNone)); err == nil {
		t.Fatal("expected screen-size error to surface")
	}
}

func TestKeyControlKeyFlushError(t *testing.T) {
	inj := &fakeInjector{w: 1080, h: 2340, textErr: errors.New("driver text failed")}
	tr := &androidTranslator{now: fixedClock(100)}
	// Buffer a character, then a control key: flushing the buffer before the
	// keycode must surface the driver error.
	if err := tr.handle(inj, Key(0, 0x04)); err != nil {
		t.Fatalf("buffering key: %v", err)
	}
	if err := tr.handle(inj, Key(0, 0x28)); err == nil {
		t.Fatal("expected flush error on control key")
	}
}

func TestTouchFlushError(t *testing.T) {
	inj := &fakeInjector{w: 1080, h: 2340, textErr: errors.New("driver text failed")}
	tr := &androidTranslator{now: fixedClock(100)}
	if err := tr.handle(inj, Key(0, 0x04)); err != nil {
		t.Fatalf("buffering key: %v", err)
	}
	// A touch flushes pending text first; that flush error must propagate.
	if err := tr.handle(inj, Touch(TouchDown, 0.5, 0.5, EdgeNone)); err == nil {
		t.Fatal("expected flush error before touch")
	}
}

func TestKeyIdleFlushTimer(t *testing.T) {
	inj := &fakeInjector{w: 1080, h: 2340}
	tr := &androidTranslator{now: fixedClock(100)}
	if err := tr.handle(inj, Key(0, 0x04)); err != nil { // 'a'
		t.Fatalf("buffering key: %v", err)
	}
	// No control key or touch follows: the idle timer must flush on its own.
	time.Sleep(textFlushDelay + 200*time.Millisecond)
	tr.mu.Lock() // synchronize with the timer goroutine before reading calls
	defer tr.mu.Unlock()
	if fmt.Sprint(inj.calls) != fmt.Sprint([]string{"text a"}) {
		t.Errorf("calls = %v, want idle-flushed [text a]", inj.calls)
	}
}

func TestFinishGestureClampsDuration(t *testing.T) {
	tests := []struct {
		name   string
		stepMs int
		want   string
	}{
		{"sub-50ms swipe clamps up to 50", 10, "swipe 540,1872->540,468 50ms"},
		{"multi-second swipe clamps down to 2000", 3000, "swipe 540,1872->540,468 2000ms"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inj := &fakeInjector{w: 1080, h: 2340}
			tr := &androidTranslator{now: fixedClock(tt.stepMs)}
			send(t, tr, inj,
				Touch(TouchDown, 0.5, 0.8, EdgeNone),
				Touch(TouchMove, 0.5, 0.5, EdgeNone),
				Touch(TouchUp, 0.5, 0.2, EdgeNone),
			)
			if fmt.Sprint(inj.calls) != fmt.Sprint([]string{tt.want}) {
				t.Errorf("calls = %v, want [%s]", inj.calls, tt.want)
			}
		})
	}
}

func TestRouterRoutesIOSFrameToSidecar(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "frames.bin")
	m := NewManager(stubSidecar(t, capture))
	defer m.CloseAll()
	r := NewRouter(m, func(context.Context, string) (AndroidInjector, error) {
		return nil, errors.New("android resolver must not be reached")
	})
	frame := Touch(TouchDown, 0.5, 0.5, EdgeNone)
	// A non-emulator serial is an iOS UDID: it must go straight to the sidecar.
	if err := r.SendFrame(context.Background(), "IOS-UDID-1", frame); err != nil {
		t.Fatalf("iOS frame: %v", err)
	}
	m.CloseAll()
	got, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, frame) {
		t.Errorf("sidecar captured %x, want %x", got, frame)
	}
}

func TestRouterRoutesByPlatform(t *testing.T) {
	inj := &fakeInjector{w: 100, h: 100}
	r := NewRouter(nil, func(context.Context, string) (AndroidInjector, error) {
		return inj, nil
	})
	ctx := context.Background()
	if err := r.SendFrame(ctx, "emulator-5554", Touch(TouchDown, 0.1, 0.1, EdgeNone)); err != nil {
		t.Fatalf("android frame: %v", err)
	}
	if err := r.SendFrame(ctx, "emulator-5554", Touch(TouchUp, 0.1, 0.1, EdgeNone)); err != nil {
		t.Fatalf("android frame: %v", err)
	}
	if len(inj.calls) != 1 || inj.calls[0] != "click 10,10" {
		t.Errorf("calls = %v, want one click", inj.calls)
	}
}

func TestRouterResolveFailure(t *testing.T) {
	r := NewRouter(nil, func(context.Context, string) (AndroidInjector, error) {
		return nil, errors.New("no engine")
	})
	if err := r.SendFrame(context.Background(), "emulator-5554", Touch(TouchDown, 0, 0, EdgeNone)); err == nil {
		t.Fatal("expected resolver error")
	}
}
