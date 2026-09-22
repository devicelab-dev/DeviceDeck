package apps

import (
	"archive/zip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unicode/utf16"
)

// Android's compiled XML (AXML) chunk types this reader needs.
const (
	chunkStringPool = 0x0001
	chunkStartTag   = 0x0102
	flagUTF8        = 1 << 8
	attrSize        = 20
)

// apkPackage reads the package name from an APK's compiled
// AndroidManifest.xml. It needs no Android SDK tools: DeviceDeck runs on a
// Mac that may have none, and the name is one attribute of the first tag.
func apkPackage(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	defer func() { _ = zr.Close() }()
	f, err := zr.Open("AndroidManifest.xml")
	if err != nil {
		return "", fmt.Errorf("%s has no AndroidManifest.xml: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 16<<20))
	if err != nil {
		return "", err
	}
	return manifestPackage(data)
}

// manifestPackage walks the AXML chunks: the string pool first, then the
// first start tag, which is <manifest>, and its package attribute.
func manifestPackage(data []byte) (string, error) {
	var pool []string
	for off := 8; off+8 <= len(data); {
		typ := binary.LittleEndian.Uint16(data[off:])
		size := int(binary.LittleEndian.Uint32(data[off+4:]))
		if size < 8 || off+size > len(data) {
			break
		}
		chunk := data[off : off+size]
		switch typ {
		case chunkStringPool:
			pool = stringPool(chunk)
		case chunkStartTag:
			return packageAttr(chunk, pool)
		}
		off += size
	}
	return "", errors.New("no manifest tag in AndroidManifest.xml")
}

// packageAttr finds the attribute named "package" on a start tag.
func packageAttr(chunk []byte, pool []string) (string, error) {
	if len(chunk) < 36 {
		return "", errors.New("truncated manifest tag")
	}
	headerSize := int(binary.LittleEndian.Uint16(chunk[2:]))
	start := headerSize + int(binary.LittleEndian.Uint16(chunk[24:]))
	count := int(binary.LittleEndian.Uint16(chunk[28:]))
	for i := 0; i < count; i++ {
		a := start + i*attrSize
		if a+attrSize > len(chunk) {
			break
		}
		if lookup(pool, binary.LittleEndian.Uint32(chunk[a+4:])) == "package" {
			if v := lookup(pool, binary.LittleEndian.Uint32(chunk[a+8:])); v != "" {
				return v, nil
			}
		}
	}
	return "", errors.New("manifest has no package attribute")
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
