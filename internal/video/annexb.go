package video

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

// H.264 NAL unit types (nal_unit_type, low 5 bits of the header byte).
const (
	nalNonIDR = 1
	nalIDR    = 5
	nalSPS    = 7
	nalPPS    = 8
)

// WriteFrame emits one framed protocol message ([type:u8][len:u32BE]
// [payload]) — the write-side counterpart of ReadFrames, shared by the
// Android capture path (the iOS sidecar frames its output in Swift).
// One Write call per frame, so a mutex-guarded writer keeps concurrent
// frame producers (video repack vs. still snapshots) from interleaving.
func WriteFrame(w io.Writer, frameType byte, payload []byte) error {
	frame := make([]byte, 5+len(payload))
	frame[0] = frameType
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	_, err := w.Write(frame)
	return err
}

// repackIdleFlush is how long the stream must stay quiet before the
// pending access unit is flushed. An Annex-B NAL is only provably
// complete when the next start code arrives — but screenrecord pauses
// output whenever pixels stop changing, which would hold the burst's
// last frame (often the IDR) hostage indefinitely. screenrecord writes
// whole access units, so a quiet pipe means the buffered tail is a
// complete NAL.
const repackIdleFlush = 100 * time.Millisecond

// RepackAnnexB converts an H.264 Annex-B elementary stream (what
// `adb screenrecord --output-format=h264` emits) into the sidecar frame
// protocol: an avcC decoder description once SPS+PPS are seen, then one
// AVCC-formatted sample per access unit, typed keyframe or delta. Runs
// until r is exhausted; returns nil on EOF.
func RepackAnnexB(r io.Reader, w io.Writer) error {
	rp := &repacker{w: w}
	done := make(chan struct{})
	defer close(done)
	chunks, readErr := readChunks(r, done)

	var buf []byte
	quiet := false // one idle tick of grace before flushing
	idle := time.NewTicker(repackIdleFlush)
	defer idle.Stop()
	for {
		select {
		case chunk := <-chunks:
			quiet = false
			var err error
			if buf, err = rp.consume(append(buf, chunk...)); err != nil {
				return err
			}
		case err := <-readErr:
			// All chunk sends happen before the error send; drain what's
			// still buffered or the stream's tail frames are lost.
			buf, _ = drainChunks(rp, chunks, buf)
			ferr := rp.finish(buf)
			if err == io.EOF {
				return ferr
			}
			return err
		case <-idle.C:
			if !quiet {
				quiet = true
				continue
			}
			var err error
			if buf, err = rp.flushPending(buf); err != nil {
				return err
			}
		}
	}
}

// drainChunks consumes every already-buffered chunk without blocking.
func drainChunks(rp *repacker, chunks <-chan []byte, buf []byte) ([]byte, error) {
	for {
		select {
		case chunk := <-chunks:
			var err error
			if buf, err = rp.consume(append(buf, chunk...)); err != nil {
				return nil, err
			}
		default:
			return buf, nil
		}
	}
}

// readChunks pumps r into a channel; the terminal read error (io.EOF
// included) arrives on the second channel. done releases the goroutine
// if the consumer returns early.
func readChunks(r io.Reader, done <-chan struct{}) (<-chan []byte, <-chan error) {
	chunks := make(chan []byte, 8)
	readErr := make(chan error, 1)
	go func() {
		for {
			buf := make([]byte, 32*1024)
			n, err := r.Read(buf)
			if n > 0 {
				select {
				case chunks <- buf[:n]:
				case <-done:
					return
				}
			}
			if err != nil {
				readErr <- err
				return
			}
		}
	}()
	return chunks, readErr
}

// repacker accumulates NAL units into access units and writes protocol
// frames. A new AU starts at a slice whose first_mb_in_slice is 0; the
// pending AU is flushed when the next one begins (or at stream end).
type repacker struct {
	w        io.Writer
	sps, pps []byte
	sentDesc bool
	au       []byte // AVCC-concatenated slices of the pending access unit
	auKey    bool
}

func (rp *repacker) handleNAL(nal []byte) error {
	switch nal[0] & 0x1F {
	case nalSPS:
		rp.sps = append([]byte(nil), nal...)
	case nalPPS:
		rp.pps = append([]byte(nil), nal...)
	case nalIDR, nalNonIDR:
		if err := rp.maybeDescription(); err != nil {
			return err
		}
		// first_mb_in_slice is the first ue(v) field after the NAL
		// header; a leading 1 bit decodes to 0 — the start of a frame.
		if len(nal) > 1 && nal[1]&0x80 != 0 {
			if err := rp.flush(); err != nil {
				return err
			}
		}
		rp.au = appendAVCC(rp.au, nal)
		rp.auKey = rp.auKey || nal[0]&0x1F == nalIDR
	default:
		// SEI, AUD, filler — nothing the decoder needs from us.
	}
	return nil
}

// flush writes the pending access unit as one sample, if any.
func (rp *repacker) flush() error {
	if len(rp.au) == 0 {
		return nil
	}
	frameType := TypeDelta
	if rp.auKey {
		frameType = TypeKeyframe
	}
	err := WriteFrame(rp.w, frameType, rp.au)
	rp.au = nil
	rp.auKey = false
	return err
}

func (rp *repacker) maybeDescription() error {
	if rp.sentDesc {
		return nil
	}
	if rp.sps == nil || rp.pps == nil {
		return fmt.Errorf("slice NAL before SPS/PPS — stream misaligned")
	}
	if err := WriteFrame(rp.w, TypeDescription, buildAVCC(rp.sps, rp.pps)); err != nil {
		return err
	}
	rp.sentDesc = true
	return nil
}

// appendAVCC appends nal to buf in AVCC framing (4-byte BE length prefix).
func appendAVCC(buf, nal []byte) []byte {
	var lenBE [4]byte
	binary.BigEndian.PutUint32(lenBE[:], uint32(len(nal)))
	return append(append(buf, lenBE[:]...), nal...)
}

// buildAVCC assembles an AVCDecoderConfigurationRecord from one SPS and
// one PPS — the `description` WebCodecs needs to decode AVCC samples.
func buildAVCC(sps, pps []byte) []byte {
	out := []byte{
		1,      // configurationVersion
		sps[1], // AVCProfileIndication
		sps[2], // profile_compatibility
		sps[3], // AVCLevelIndication
		0xFF,   // 4-byte NAL length size
		0xE1,   // one SPS
	}
	out = append(out, byte(len(sps)>>8), byte(len(sps)))
	out = append(out, sps...)
	out = append(out, 1) // one PPS
	out = append(out, byte(len(pps)>>8), byte(len(pps)))
	return append(out, pps...)
}

// consume parses every complete NAL out of buf, returning the unparsed
// remainder. Bytes before the first start code (possible after an idle
// flush consumed a NAL without its terminator) are dropped to resync.
func (rp *repacker) consume(buf []byte) ([]byte, error) {
	for {
		if trimStartCode(buf) == nil && len(buf) >= 4 {
			if idx := bytes.Index(buf, startCode); idx >= 0 {
				buf = buf[idx:]
			} else {
				buf = buf[len(buf)-3:] // keep a possible partial start code
			}
		}
		nal, rest, ok := splitNAL(buf)
		if !ok {
			return buf, nil
		}
		buf = rest
		if err := rp.handleNAL(nal); err != nil {
			return nil, err
		}
	}
}

// flushPending treats a quiet stream's buffered tail as a complete NAL
// (screenrecord writes whole access units) and emits the pending AU.
func (rp *repacker) flushPending(buf []byte) ([]byte, error) {
	if nal := trimStartCode(buf); len(nal) > 0 {
		if err := rp.handleNAL(nal); err != nil {
			return nil, err
		}
		buf = nil
	}
	return buf, rp.flush()
}

// finish drains the final buffered NAL at stream end.
func (rp *repacker) finish(buf []byte) error {
	if _, err := rp.flushPending(buf); err != nil {
		return err
	}
	return nil
}

var startCode = []byte{0, 0, 1}

// splitNAL extracts the first complete NAL from buf: the bytes between
// the leading start code and the next one.
func splitNAL(buf []byte) (nal, rest []byte, ok bool) {
	body := trimStartCode(buf)
	if body == nil {
		return nil, buf, false
	}
	idx := bytes.Index(body, startCode)
	if idx < 0 {
		return nil, buf, false
	}
	end := idx
	if end > 0 && body[end-1] == 0 { // 4-byte start code's leading zero
		end--
	}
	return body[:end], body[idx:], true
}

// trimStartCode strips one leading 3- or 4-byte start code; nil if buf
// does not begin with one (or is too short to tell).
func trimStartCode(buf []byte) []byte {
	if len(buf) >= 4 && buf[0] == 0 && buf[1] == 0 && buf[2] == 0 && buf[3] == 1 {
		return buf[4:]
	}
	if len(buf) >= 3 && buf[0] == 0 && buf[1] == 0 && buf[2] == 1 {
		return buf[3:]
	}
	return nil
}
