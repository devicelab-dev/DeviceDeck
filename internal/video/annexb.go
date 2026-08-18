package video

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
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

// RepackAnnexB converts an H.264 Annex-B elementary stream (what
// `adb screenrecord --output-format=h264` emits) into the sidecar frame
// protocol: an avcC decoder description once SPS+PPS are seen, then one
// AVCC-formatted sample per access unit, typed keyframe or delta. Runs
// until r is exhausted; returns nil on EOF.
func RepackAnnexB(r io.Reader, w io.Writer) error {
	rp := &repacker{w: w}
	sc := newAnnexBScanner(r)
	for {
		nal, err := sc.next()
		if nal != nil {
			if werr := rp.handleNAL(nal); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			return rp.flush()
		}
		if err != nil {
			return err
		}
	}
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

// annexBScanner splits a byte stream on Annex-B start codes (00 00 01,
// optionally preceded by an extra 00), yielding complete NAL units. A
// NAL is only known complete once the next start code (or EOF) arrives.
type annexBScanner struct {
	r   *bufio.Reader
	buf []byte
}

func newAnnexBScanner(r io.Reader) *annexBScanner {
	return &annexBScanner{r: bufio.NewReaderSize(r, 64*1024)}
}

var startCode = []byte{0, 0, 1}

// next returns the next complete NAL unit. It returns a final NAL
// together with io.EOF when the stream ends.
func (s *annexBScanner) next() ([]byte, error) {
	for {
		if nal, rest, ok := splitNAL(s.buf); ok {
			s.buf = rest
			return nal, nil
		}
		chunk := make([]byte, 32*1024)
		n, err := s.r.Read(chunk)
		s.buf = append(s.buf, chunk[:n]...)
		if err != nil {
			nal := trimStartCode(s.buf)
			s.buf = nil
			if err == io.EOF && len(nal) == 0 {
				return nil, io.EOF
			}
			return nal, err
		}
	}
}

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
