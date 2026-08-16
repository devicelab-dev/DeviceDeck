// Package input encodes DeviceDeck's sidecar wire protocol and manages
// devicedeck-hid sidecar processes. The frame layout is defined by
// sidecar/Sources/HIDProtocol/Frame.swift; the two must stay in lockstep.
package input

import (
	"encoding/binary"
	"math"
)

// TouchPhase selects the down/move/up variant of a touch frame type.
type TouchPhase byte

const (
	TouchDown TouchPhase = 0
	TouchMove TouchPhase = 1
	TouchUp   TouchPhase = 2
)

// Edge flags a touch as starting on a screen edge, routing it to iOS's
// system gesture recognizers instead of the app's pan handlers.
type Edge byte

const (
	EdgeNone   Edge = 0
	EdgeLeft   Edge = 1
	EdgeTop    Edge = 2
	EdgeRight  Edge = 3
	EdgeBottom Edge = 4
)

// Gesture identifies a canned system gesture recipe executed sidecar-side.
type Gesture byte

const (
	GestureSwipeToHome        Gesture = 1
	GestureAppSwitcher        Gesture = 2
	GestureNotificationCenter Gesture = 3
	GestureLockScreen         Gesture = 4
)

// Frame type bytes — must match Frame.parse in the Swift sidecar.
const (
	typeTouchBase     byte = 0x01 // +phase → 0x01..0x03
	typeTwoFingerBase byte = 0x04 // +phase → 0x04..0x06
	typeButtonPress   byte = 0x07
	typeButtonDown    byte = 0x08
	typeButtonUp      byte = 0x09
	typeLegacyButton  byte = 0x0A
	typeKey           byte = 0x0B
	typeGesture       byte = 0x0C
)

// Touch encodes a single-finger touch frame. Coordinates are normalized 0–1.
func Touch(phase TouchPhase, x, y float64, edge Edge) []byte {
	buf := make([]byte, 10)
	buf[0] = typeTouchBase + byte(phase)
	putF32(buf[1:], x)
	putF32(buf[5:], y)
	buf[9] = byte(edge)
	return buf
}

// TwoFinger encodes a two-finger touch frame (pinch / two-finger pan).
func TwoFinger(phase TouchPhase, x1, y1, x2, y2 float64) []byte {
	buf := make([]byte, 17)
	buf[0] = typeTwoFingerBase + byte(phase)
	putF32(buf[1:], x1)
	putF32(buf[5:], y1)
	putF32(buf[9:], x2)
	putF32(buf[13:], y2)
	return buf
}

// ButtonPress encodes a press-and-release of a HID (page, usage) button.
func ButtonPress(page, usage uint32) []byte { return buttonFrame(typeButtonPress, page, usage) }

// ButtonDown encodes the down half of a real-time button hold.
func ButtonDown(page, usage uint32) []byte { return buttonFrame(typeButtonDown, page, usage) }

// ButtonUp encodes the up half of a real-time button hold.
func ButtonUp(page, usage uint32) []byte { return buttonFrame(typeButtonUp, page, usage) }

// LegacyButton encodes a home (0) or lock (1) press via the legacy service.
func LegacyButton(code uint32) []byte {
	buf := make([]byte, 5)
	buf[0] = typeLegacyButton
	binary.BigEndian.PutUint32(buf[1:], code)
	return buf
}

// Key encodes a key press. modifiers is the USB HID modifier bitmap
// (bit0=LeftCtrl, bit1=LeftShift, bit2=LeftAlt, bit3=LeftGUI, …); usage is
// the HID keyboard usage on page 0x07.
func Key(modifiers byte, usage uint32) []byte {
	buf := make([]byte, 6)
	buf[0] = typeKey
	buf[1] = modifiers
	binary.BigEndian.PutUint32(buf[2:], usage)
	return buf
}

// SystemGesture encodes a canned system gesture frame.
func SystemGesture(g Gesture) []byte {
	return []byte{typeGesture, byte(g)}
}

// FrameLength returns the total wire length (type byte included) for a
// frame starting with frameType, or 0 for an unknown type. Mirrors
// Frame.payloadLength in the Swift sidecar.
func FrameLength(frameType byte) int {
	switch {
	case frameType >= 0x01 && frameType <= 0x03:
		return 10
	case frameType >= 0x04 && frameType <= 0x06:
		return 17
	case frameType >= 0x07 && frameType <= 0x09:
		return 9
	case frameType == typeLegacyButton:
		return 5
	case frameType == typeKey:
		return 6
	case frameType == typeGesture:
		return 2
	default:
		return 0
	}
}

// ValidFrame reports whether raw is exactly one well-formed frame. The
// live input WebSocket uses it to reject malformed client frames before
// they can desync the sidecar's stdin stream.
func ValidFrame(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	return FrameLength(raw[0]) == len(raw)
}

func buttonFrame(frameType byte, page, usage uint32) []byte {
	buf := make([]byte, 9)
	buf[0] = frameType
	binary.BigEndian.PutUint32(buf[1:], page)
	binary.BigEndian.PutUint32(buf[5:], usage)
	return buf
}

func putF32(dst []byte, v float64) {
	binary.BigEndian.PutUint32(dst, math.Float32bits(float32(v)))
}
