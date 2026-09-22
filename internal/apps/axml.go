package apps

import (
	"archive/zip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unicode/utf16"
)

// Android's compiled XML (AXML) chunk types this reader needs.
const (
	chunkStringPool = 0x0001
	chunkStartTag   = 0x0102
	flagUTF8        = 1 << 8
	attrSize        = 20
	typeIntDec      = 0x10 // an attribute stored as a decimal integer
)

// manifest is what DeviceDeck reads from an APK's compiled
// AndroidManifest.xml.
type manifest struct {
	Package, VersionName, VersionCode, MinSDK string
}

// readManifest opens an APK and reads its manifest. It needs no Android SDK
// tools: DeviceDeck runs on a Mac that may have none.
func readManifest(zr *zip.Reader) (manifest, error) {
	f, err := zr.Open("AndroidManifest.xml")
	if err != nil {
		return manifest{}, fmt.Errorf("no AndroidManifest.xml: %w", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 16<<20))
	if err != nil {
		return manifest{}, err
	}
	return parseManifest(data)
}

// parseManifest walks the AXML chunks: the string pool, then each start
// tag, keeping the attributes of <manifest> and <uses-sdk>.
func parseManifest(data []byte) (manifest, error) {
	var m manifest
	var pool []string
	for off := 8; off+8 <= len(data); {
		typ := binary.LittleEndian.Uint16(data[off:])
		size := int(binary.LittleEndian.Uint32(data[off+4:]))
		if size < 8 || off+size > len(data) {
			break
		}
		switch chunk := data[off : off+size]; typ {
		case chunkStringPool:
			pool = stringPool(chunk)
		case chunkStartTag:
			m.take(chunk, pool)
		}
		off += size
	}
	if m.Package == "" {
		return m, errors.New("manifest has no package")
	}
	return m, nil
}

// take copies the attributes it knows from one start tag.
func (m *manifest) take(chunk []byte, pool []string) {
	attrs := tagAttrs(chunk, pool)
	switch tagName(chunk, pool) {
	case "manifest":
		m.Package, m.VersionName, m.VersionCode = attrs["package"], attrs["versionName"], attrs["versionCode"]
	case "uses-sdk":
		m.MinSDK = attrs["minSdkVersion"]
	}
}

func tagName(chunk []byte, pool []string) string {
	if len(chunk) < 24 {
		return ""
	}
	return lookup(pool, binary.LittleEndian.Uint32(chunk[20:]))
}

// tagAttrs reads a start tag's attributes: a string value when it has one,
// otherwise a decimal integer (versionCode and minSdkVersion are stored so).
func tagAttrs(chunk []byte, pool []string) map[string]string {
	out := map[string]string{}
	if len(chunk) < 36 {
		return out
	}
	start := int(binary.LittleEndian.Uint16(chunk[2:])) + int(binary.LittleEndian.Uint16(chunk[24:]))
	count := int(binary.LittleEndian.Uint16(chunk[28:]))
	for i := 0; i < count && start+(i+1)*attrSize <= len(chunk); i++ {
		a := chunk[start+i*attrSize:]
		name := lookup(pool, binary.LittleEndian.Uint32(a[4:]))
		if v := lookup(pool, binary.LittleEndian.Uint32(a[8:])); v != "" {
			out[name] = v
		} else if a[15] == typeIntDec {
			out[name] = strconv.Itoa(int(int32(binary.LittleEndian.Uint32(a[16:]))))
		}
	}
	return out
}

func lookup(pool []string, i uint32) string {
	if int(i) < len(pool) {
		return pool[i]
	}
	return ""
}

// stringPool decodes a string pool chunk, UTF-8 or UTF-16.
func stringPool(chunk []byte) []string {
	if len(chunk) < 28 {
		return nil
	}
	count := int(binary.LittleEndian.Uint32(chunk[8:]))
	utf8 := binary.LittleEndian.Uint32(chunk[16:])&flagUTF8 != 0
	strStart := int(binary.LittleEndian.Uint32(chunk[20:]))
	headerSize := int(binary.LittleEndian.Uint16(chunk[2:]))
	out := make([]string, 0, count)
	for i := 0; i < count && headerSize+4*i+4 <= len(chunk); i++ {
		off := strStart + int(binary.LittleEndian.Uint32(chunk[headerSize+4*i:]))
		if utf8 {
			out = append(out, utf8At(chunk, off))
		} else {
			out = append(out, utf16At(chunk, off))
		}
	}
	return out
}

// utf8At reads a UTF-8 pool string: a UTF-16 length, a byte length, bytes.
func utf8At(b []byte, off int) string {
	_, off = lenPrefix8(b, off)
	n, off := lenPrefix8(b, off)
	if off < 0 || off+n > len(b) {
		return ""
	}
	return string(b[off : off+n])
}

// lenPrefix8 reads a one- or two-byte length (high bit marks two).
func lenPrefix8(b []byte, off int) (int, int) {
	if off < 0 || off >= len(b) {
		return 0, -1
	}
	n := int(b[off])
	if n&0x80 == 0 {
		return n, off + 1
	}
	if off+1 >= len(b) {
		return 0, -1
	}
	return (n&0x7f)<<8 | int(b[off+1]), off + 2
}

// utf16At reads a UTF-16 pool string: a length in units, then the units.
func utf16At(b []byte, off int) string {
	if off < 0 || off+2 > len(b) {
		return ""
	}
	n := int(binary.LittleEndian.Uint16(b[off:]))
	off += 2
	if n&0x8000 != 0 {
		if off+2 > len(b) {
			return ""
		}
		n = (n&0x7fff)<<16 | int(binary.LittleEndian.Uint16(b[off:]))
		off += 2
	}
	if off+2*n > len(b) {
		return ""
	}
	units := make([]uint16, n)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(b[off+2*i:])
	}
	return string(utf16.Decode(units))
}
