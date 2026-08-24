package video

import (
	"bytes"
	"io"
	"testing"
	"time"
)

// nal builds a synthetic NAL unit of the given type with payload bytes.
func nal(nalType byte, payload ...byte) []byte {
	return append([]byte{nalType}, payload...)
}

// annexb joins NAL units with the given start codes into one stream.
func annexb(code4 bool, nals ...[]byte) []byte {
	var out []byte
	for _, n := range nals {
		if code4 {
			out = append(out, 0, 0, 0, 1)
		} else {
			out = append(out, 0, 0, 1)
		}
		out = append(out, n...)
	}
	return out
}

// collectFrames parses framed protocol output into (type, payload) pairs.
func collectFrames(t *testing.T, data []byte) []struct {
	Type    byte
	Payload []byte
} {
	t.Helper()
	var frames []struct {
		Type    byte
		Payload []byte
	}
	err := ReadFrames(bytes.NewReader(data), func(ft byte, p []byte) {
		frames = append(frames, struct {
			Type    byte
			Payload []byte
		}{ft, append([]byte(nil), p...)})
	})
	if err != io.EOF && err != nil {
		t.Fatalf("ReadFrames: %v", err)
	}
	return frames
}

// Slice payload first byte 0x80: leading 1 bit = first_mb_in_slice 0
// (frame start); 0x40 decodes to a non-zero first_mb (continuation).
const frameStart, continuation = 0x80, 0x40

func TestRepackAnnexB(t *testing.T) {
	sps := nal(0x67, 0x42, 0xC0, 0x32, 0xAA)
	pps := nal(0x68, 0xCE, 0x01)
	idr := nal(0x65, frameStart, 0x11)
	delta := nal(0x41, frameStart, 0x22)
	sei := nal(0x06, 0x99)

	tests := []struct {
		name      string
		stream    []byte
		wantTypes []byte
	}{
		{
			name:      "sps pps idr delta with 4-byte codes",
			stream:    annexb(true, sps, pps, idr, delta),
			wantTypes: []byte{TypeDescription, TypeKeyframe, TypeDelta},
		},
		{
			name:      "3-byte start codes",
			stream:    annexb(false, sps, pps, idr),
			wantTypes: []byte{TypeDescription, TypeKeyframe},
		},
		{
			name:      "sei is skipped",
			stream:    annexb(true, sps, pps, sei, idr),
			wantTypes: []byte{TypeDescription, TypeKeyframe},
		},
		{
			name:      "multi-slice access unit groups into one sample",
			stream:    annexb(true, sps, pps, idr, nal(0x65, continuation, 0x12), delta),
			wantTypes: []byte{TypeDescription, TypeKeyframe, TypeDelta},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := RepackAnnexB(bytes.NewReader(tt.stream), &out); err != nil {
				t.Fatalf("RepackAnnexB: %v", err)
			}
			frames := collectFrames(t, out.Bytes())
			var gotTypes []byte
			for _, f := range frames {
				gotTypes = append(gotTypes, f.Type)
			}
			if !bytes.Equal(gotTypes, tt.wantTypes) {
				t.Errorf("frame types = %v, want %v", gotTypes, tt.wantTypes)
			}
		})
	}
}

func TestRepackAVCCFraming(t *testing.T) {
	sps := nal(0x67, 0x42, 0xC0, 0x32)
	pps := nal(0x68, 0xCE)
	idr := nal(0x65, frameStart, 0x11)

	var out bytes.Buffer
	if err := RepackAnnexB(bytes.NewReader(annexb(true, sps, pps, idr)), &out); err != nil {
		t.Fatal(err)
	}
	frames := collectFrames(t, out.Bytes())
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want description + keyframe", len(frames))
	}

	wantAVCC := []byte{
		1, 0x42, 0xC0, 0x32, // version + profile/compat/level from SPS
		0xFF, 0xE1, // 4-byte lengths, one SPS
		0, 4, 0x67, 0x42, 0xC0, 0x32,
		1, 0, 2, 0x68, 0xCE,
	}
	if !bytes.Equal(frames[0].Payload, wantAVCC) {
		t.Errorf("avcC = % x, want % x", frames[0].Payload, wantAVCC)
	}

	wantSample := []byte{0, 0, 0, 3, 0x65, frameStart, 0x11}
	if !bytes.Equal(frames[1].Payload, wantSample) {
		t.Errorf("sample = % x, want % x", frames[1].Payload, wantSample)
	}
}

func TestRepackSliceBeforeSPSErrors(t *testing.T) {
	stream := annexb(true, nal(0x65, frameStart, 0x11))
	if err := RepackAnnexB(bytes.NewReader(stream), io.Discard); err == nil {
		t.Fatal("expected misalignment error")
	}
}

// oneByteReader forces the scanner to reassemble across reads.
type oneByteReader struct{ data []byte }

func (r *oneByteReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	p[0] = r.data[0]
	r.data = r.data[1:]
	return 1, nil
}

func TestRepackSurvivesFragmentedReads(t *testing.T) {
	sps := nal(0x67, 0x42, 0xC0, 0x32)
	pps := nal(0x68, 0xCE)
	stream := annexb(true, sps, pps, nal(0x65, frameStart, 0x11), nal(0x41, frameStart, 0x22))

	var out bytes.Buffer
	if err := RepackAnnexB(&oneByteReader{data: stream}, &out); err != nil {
		t.Fatalf("RepackAnnexB: %v", err)
	}
	frames := collectFrames(t, out.Bytes())
	if len(frames) != 3 {
		t.Fatalf("got %d frames, want 3 (description, keyframe, trailing delta flushed at EOF)", len(frames))
	}
}

func TestRepackFlushesWhenStreamPauses(t *testing.T) {
	// screenrecord's real shape: a burst of frames, then silence while
	// pixels are static. The burst's last access unit has no terminating
	// start code — the idle flush must deliver it anyway.
	pr, pw := io.Pipe()
	defer pw.Close()
	out := &syncBuffer{}
	done := make(chan error, 1)
	go func() { done <- RepackAnnexB(pr, out) }()

	sps := nal(0x67, 0x42, 0xC0, 0x32)
	pps := nal(0x68, 0xCE)
	if _, err := pw.Write(annexb(true, sps, pps, nal(0x65, frameStart, 0x11))); err != nil {
		t.Fatal(err)
	}
	// No more writes: the IDR must arrive without waiting for stream end.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		counts := map[byte]int{}
		_ = ReadFrames(bytes.NewReader(out.Bytes()), func(ft byte, _ []byte) { counts[ft]++ })
		if counts[TypeKeyframe] >= 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("paused stream's keyframe never flushed")
}

func TestIsAndroidSerial(t *testing.T) {
	if !IsAndroidSerial("emulator-5554") {
		t.Error("emulator serial not recognized")
	}
	if IsAndroidSerial("EB69B42A-4763-4A33-AF0F-CD233F721951") {
		t.Error("simulator UDID misclassified as Android")
	}
}

// TestRepackIdleFlushError drives the idle-flush branch to an error: a
// paused stream whose buffered tail is a slice NAL with no SPS/PPS ahead
// of it. The idle ticker flushes the tail, which fails misalignment.
func TestRepackIdleFlushError(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	done := make(chan error, 1)
	go func() { done <- RepackAnnexB(pr, io.Discard) }()

	// A lone slice NAL, no terminating start code and no SPS/PPS. The
	// stream then goes quiet, so only the idle flush can surface it.
	if _, err := pw.Write(annexb(true, nal(0x65, frameStart, 0x11))); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected misalignment error from the idle flush")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("idle flush never fired")
	}
}

// TestDrainChunksConsumeError exercises drainChunks' error path: a chunk
// still buffered when the reader errors contains a slice NAL that parses
// before any SPS/PPS, so consume rejects it mid-drain.
func TestDrainChunksConsumeError(t *testing.T) {
	rp := &repacker{w: io.Discard}
	chunks := make(chan []byte, 1)
	// Two slices so the first is fully delimited and reaches handleNAL.
	chunks <- annexb(true, nal(0x65, frameStart, 0x11), nal(0x41, frameStart, 0x22))
	if _, err := drainChunks(rp, chunks, nil); err == nil {
		t.Fatal("expected consume error while draining a slice before SPS/PPS")
	}
}

// constReader returns a full buffer of zeros on every read and never
// ends — it keeps readChunks producing so its channel fills.
type constReader struct{}

func (constReader) Read(p []byte) (int, error) { return len(p), nil }

// TestReadChunksReleasesOnDone fills readChunks' buffered channel with an
// unbounded reader and never drains it, so the pump blocks mid-send;
// closing done must release the goroutine through its <-done branch.
func TestReadChunksReleasesOnDone(t *testing.T) {
	done := make(chan struct{})
	chunks, readErr := readChunks(constReader{}, done)
	time.Sleep(50 * time.Millisecond) // let the 8-deep buffer fill and block
	close(done)
	time.Sleep(50 * time.Millisecond) // let the goroutine take the <-done exit
	// Nothing more should arrive on readErr: the goroutine left via done.
	select {
	case err := <-readErr:
		t.Fatalf("readErr = %v, want silent release on done", err)
	default:
	}
	_ = chunks
}

// TestConsumeResyncsPastGarbage covers consume's resync: bytes before the
// first start code (left behind after an idle flush ate a NAL without its
// terminator) are dropped, and pure garbage keeps only a partial-code tail.
func TestConsumeResyncsPastGarbage(t *testing.T) {
	rp := &repacker{w: io.Discard, sps: nal(0x67, 0x42), pps: nal(0x68, 0xCE), sentDesc: true}

	// Leading garbage, then a delimited slice: resync jumps to the code.
	buf := append([]byte{0xAB, 0xCD, 0xEF},
		annexb(true, nal(0x41, frameStart, 0x11), nal(0x41, continuation, 0x22))...)
	if _, err := rp.consume(buf); err != nil {
		t.Fatalf("consume past leading garbage: %v", err)
	}

	// Pure garbage, no start code anywhere: keep only the last 3 bytes.
	rest, err := rp.consume([]byte{1, 2, 3, 4, 5})
	if err != nil {
		t.Fatalf("consume of codeless garbage: %v", err)
	}
	if len(rest) != 3 {
		t.Errorf("remainder = % x, want a 3-byte partial-start-code tail", rest)
	}
}
