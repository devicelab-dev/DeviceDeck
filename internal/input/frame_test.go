package input

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func f32be(v float64) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, math.Float32bits(float32(v)))
	return b
}

func u32be(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

func concat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

func TestTouch(t *testing.T) {
	tests := []struct {
		name     string
		phase    TouchPhase
		edge     Edge
		wantType byte
	}{
		{"down interior", TouchDown, EdgeNone, 0x01},
		{"move interior", TouchMove, EdgeNone, 0x02},
		{"up interior", TouchUp, EdgeNone, 0x03},
		{"down bottom edge", TouchDown, EdgeBottom, 0x01},
		{"down left edge", TouchDown, EdgeLeft, 0x01},
		{"down top edge", TouchDown, EdgeTop, 0x01},
		{"down right edge", TouchDown, EdgeRight, 0x01},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Touch(tt.phase, 0.5, 0.25, tt.edge)
			want := concat([]byte{tt.wantType}, f32be(0.5), f32be(0.25), []byte{byte(tt.edge)})
			if !bytes.Equal(got, want) {
				t.Errorf("Touch() = %x, want %x", got, want)
			}
		})
	}
}

func TestTwoFinger(t *testing.T) {
	tests := []struct {
		phase    TouchPhase
		wantType byte
	}{
		{TouchDown, 0x04},
		{TouchMove, 0x05},
		{TouchUp, 0x06},
	}
	for _, tt := range tests {
		got := TwoFinger(tt.phase, 0.125, 0.25, 0.375, 0.5)
		want := concat([]byte{tt.wantType}, f32be(0.125), f32be(0.25), f32be(0.375), f32be(0.5))
		if !bytes.Equal(got, want) {
			t.Errorf("TwoFinger(phase=%d) = %x, want %x", tt.phase, got, want)
		}
	}
}

func TestButtons(t *testing.T) {
	tests := []struct {
		name     string
		got      []byte
		wantType byte
	}{
		{"press", ButtonPress(0x0C, 0xE9), 0x07},
		{"down", ButtonDown(0x0C, 0xE9), 0x08},
		{"up", ButtonUp(0x0C, 0xE9), 0x09},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := concat([]byte{tt.wantType}, u32be(0x0C), u32be(0xE9))
			if !bytes.Equal(tt.got, want) {
				t.Errorf("= %x, want %x", tt.got, want)
			}
		})
	}
}

func TestLegacyButton(t *testing.T) {
	if got, want := LegacyButton(1), concat([]byte{0x0A}, u32be(1)); !bytes.Equal(got, want) {
		t.Errorf("LegacyButton() = %x, want %x", got, want)
	}
}

func TestKey(t *testing.T) {
	got := Key(0x02, 0x04) // LeftShift + 'a'
	want := concat([]byte{0x0B, 0x02}, u32be(0x04))
	if !bytes.Equal(got, want) {
		t.Errorf("Key() = %x, want %x", got, want)
	}
}

func TestSystemGesture(t *testing.T) {
	for _, g := range []Gesture{GestureSwipeToHome, GestureAppSwitcher, GestureNotificationCenter, GestureLockScreen} {
		got := SystemGesture(g)
		if !bytes.Equal(got, []byte{0x0C, byte(g)}) {
			t.Errorf("SystemGesture(%d) = %x", g, got)
		}
	}
}
