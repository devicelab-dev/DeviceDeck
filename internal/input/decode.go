package input

import (
	"encoding/binary"
	"math"
)

// EventKind classifies a decoded frame.
type EventKind int

const (
	EventTouch EventKind = iota + 1
	EventTwoFinger
	EventButtonPress
	EventButtonDown
	EventButtonUp
	EventLegacyButton
	EventKey
	EventGesture
)

// Event is the decoded form of one wire frame — what Flow Capture consumes.
type Event struct {
	Kind    EventKind
	Phase   TouchPhase
	X, Y    float64
	X2, Y2  float64
	Edge    Edge
	Page    uint32
	Usage   uint32
	Code    uint32
	Mod     byte
	Gesture Gesture
}

// Decode parses one well-formed frame into an Event. Returns false for
// malformed or unknown frames (callers already gate on ValidFrame; this
// re-checks so Decode is safe standalone).
func Decode(frame []byte) (Event, bool) {
	if !ValidFrame(frame) {
		return Event{}, false
	}
	switch t := frame[0]; {
	case t >= 0x01 && t <= 0x03:
		return Event{
			Kind:  EventTouch,
			Phase: TouchPhase(t - 0x01),
			X:     readF32BE(frame[1:]),
			Y:     readF32BE(frame[5:]),
			Edge:  Edge(frame[9]),
		}, true
	case t >= 0x04 && t <= 0x06:
		return Event{
			Kind:  EventTwoFinger,
			Phase: TouchPhase(t - 0x04),
			X:     readF32BE(frame[1:]),
			Y:     readF32BE(frame[5:]),
			X2:    readF32BE(frame[9:]),
			Y2:    readF32BE(frame[13:]),
		}, true
	case t >= 0x07 && t <= 0x09:
		kinds := map[byte]EventKind{0x07: EventButtonPress, 0x08: EventButtonDown, 0x09: EventButtonUp}
		return Event{
			Kind:  kinds[t],
			Page:  binary.BigEndian.Uint32(frame[1:]),
			Usage: binary.BigEndian.Uint32(frame[5:]),
		}, true
	case t == typeLegacyButton:
		return Event{Kind: EventLegacyButton, Code: binary.BigEndian.Uint32(frame[1:])}, true
	case t == typeKey:
		return Event{Kind: EventKey, Mod: frame[1], Usage: binary.BigEndian.Uint32(frame[2:])}, true
	case t == typeGesture:
		return Event{Kind: EventGesture, Gesture: Gesture(frame[1])}, true
	default:
		return Event{}, false
	}
}

func readF32BE(b []byte) float64 {
	return float64(math.Float32frombits(binary.BigEndian.Uint32(b)))
}
