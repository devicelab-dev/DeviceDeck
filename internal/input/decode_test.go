package input

import "testing"

func TestDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		frame []byte
		want  Event
	}{
		{"touch down", Touch(TouchDown, 0.5, 0.25, EdgeBottom),
			Event{Kind: EventTouch, Phase: TouchDown, X: 0.5, Y: 0.25, Edge: EdgeBottom}},
		{"touch move", Touch(TouchMove, 0.125, 0.75, EdgeNone),
			Event{Kind: EventTouch, Phase: TouchMove, X: 0.125, Y: 0.75}},
		{"touch up", Touch(TouchUp, 1, 0, EdgeNone),
			Event{Kind: EventTouch, Phase: TouchUp, X: 1, Y: 0}},
		{"two finger", TwoFinger(TouchMove, 0.125, 0.25, 0.375, 0.5),
			Event{Kind: EventTwoFinger, Phase: TouchMove, X: 0.125, Y: 0.25, X2: 0.375, Y2: 0.5}},
		{"button press", ButtonPress(0x0C, 0xE9),
			Event{Kind: EventButtonPress, Page: 0x0C, Usage: 0xE9}},
		{"button down", ButtonDown(1, 2), Event{Kind: EventButtonDown, Page: 1, Usage: 2}},
		{"button up", ButtonUp(1, 2), Event{Kind: EventButtonUp, Page: 1, Usage: 2}},
		{"legacy", LegacyButton(1), Event{Kind: EventLegacyButton, Code: 1}},
		{"key", Key(0x02, 0x04), Event{Kind: EventKey, Mod: 0x02, Usage: 0x04}},
		{"gesture", SystemGesture(GestureAppSwitcher),
			Event{Kind: EventGesture, Gesture: GestureAppSwitcher}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Decode(tt.frame)
			if !ok {
				t.Fatalf("Decode rejected %x", tt.frame)
			}
			if got != tt.want {
				t.Errorf("Decode = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodeRejectsMalformed(t *testing.T) {
	for _, frame := range [][]byte{nil, {}, {0xFF, 1}, {0x01, 1, 2}} {
		if _, ok := Decode(frame); ok {
			t.Errorf("Decode accepted %x", frame)
		}
	}
}
