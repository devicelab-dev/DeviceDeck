package apps

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// attr is one attribute: a name index, a string value index (noString for
// none), and a typed integer value.
type attr struct {
	name, raw uint32
	typ       byte
	data      uint32
}

const noString = 0xFFFFFFFF

// tag is a start tag: its name index and attributes.
type tag struct {
	name  uint32
	attrs []attr
}

func stringPoolChunk(strs []string, utf8 bool) []byte {
	le := binary.LittleEndian
	var body bytes.Buffer
	offsets := make([]uint32, len(strs))
	for i, s := range strs {
		offsets[i] = uint32(body.Len())
		if utf8 {
			body.Write([]byte{byte(len(s)), byte(len(s))})
			body.WriteString(s)
			body.WriteByte(0)
		} else {
			u := utf16.Encode([]rune(s))
			_ = binary.Write(&body, le, uint16(len(u)))
			_ = binary.Write(&body, le, u)
			body.Write([]byte{0, 0})
		}
	}
	pool := make([]byte, 28+4*len(strs))
	le.PutUint16(pool[0:], chunkStringPool)
	le.PutUint16(pool[2:], 28)
	le.PutUint32(pool[4:], uint32(len(pool)+body.Len()))
	le.PutUint32(pool[8:], uint32(len(strs)))
	if utf8 {
		le.PutUint32(pool[16:], flagUTF8)
	}
	le.PutUint32(pool[20:], uint32(len(pool)))
	for i, o := range offsets {
		le.PutUint32(pool[28+4*i:], o)
	}
	return append(pool, body.Bytes()...)
}

func startTag(t tag) []byte {
	le := binary.LittleEndian
	c := make([]byte, 36+attrSize*len(t.attrs))
	le.PutUint16(c[0:], chunkStartTag)
	le.PutUint16(c[2:], 16)
	le.PutUint32(c[4:], uint32(len(c)))
	le.PutUint32(c[20:], t.name)
	le.PutUint16(c[24:], 20) // attributes start 20 bytes after the header
	le.PutUint16(c[26:], attrSize)
	le.PutUint16(c[28:], uint16(len(t.attrs)))
	for i, a := range t.attrs {
		o := 36 + attrSize*i
		le.PutUint32(c[o+4:], a.name)
		le.PutUint32(c[o+8:], a.raw)
		c[o+15] = a.typ
		le.PutUint32(c[o+16:], a.data)
	}
	return c
}

// axml assembles a compiled manifest from a pool and tags.
func axml(strs []string, utf8 bool, tags ...tag) []byte {
	out := []byte{3, 0, 8, 0, 0, 0, 0, 0}
	out = append(out, stringPoolChunk(strs, utf8)...)
	for _, t := range tags {
		out = append(out, startTag(t)...)
	}
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)))
	return out
}

// testManifest is a manifest like TestHive's: string package and
// versionName, integer versionCode and minSdkVersion.
func testManifest(utf8 bool) []byte {
	strs := []string{"manifest", "package", "versionName", "versionCode", "uses-sdk", "minSdkVersion", "com.example.app", "2.3.1"}
	return axml(strs, utf8,
		tag{0, []attr{{1, 6, 0, 0}, {2, 7, 0, 0}, {3, noString, typeIntDec, 42}}},
		tag{4, []attr{{5, noString, typeIntDec, 24}}},
	)
}

func TestParseManifest(t *testing.T) {
	want := manifest{Package: "com.example.app", VersionName: "2.3.1", VersionCode: "42", MinSDK: "24"}
	for _, utf8 := range []bool{true, false} {
		if got, err := parseManifest(testManifest(utf8)); err != nil || got != want {
			t.Errorf("utf8=%v: %+v, %v", utf8, got, err)
		}
	}
	noPkg := axml([]string{"manifest", "versionName", "1"}, true, tag{0, []attr{{1, 2, 0, 0}}})
	if _, err := parseManifest(noPkg); err == nil {
		t.Error("a manifest without a package must be an error")
	}
	bad := testManifest(true)
	binary.LittleEndian.PutUint32(bad[12:], 4) // the pool claims a size below its header
	if _, err := parseManifest(bad); err == nil {
		t.Error("a malformed chunk must stop the walk")
	}
	if _, err := parseManifest(nil); err == nil {
		t.Error("empty data must be an error")
	}
}

func TestTagEdges(t *testing.T) {
	pool := []string{"manifest", "package", "com.x"}
	if tagName(make([]byte, 10), pool) != "" || len(tagAttrs(make([]byte, 20), pool)) != 0 {
		t.Error("a truncated tag has no name or attributes")
	}
	c := startTag(tag{0, []attr{{1, 2, 0, 0}}})
	binary.LittleEndian.PutUint16(c[28:], 3) // claims three attributes, holds one
	if got := tagAttrs(c, pool); len(got) != 1 || got["package"] != "com.x" {
		t.Errorf("attributes past the chunk must be ignored: %v", got)
	}
	untyped := startTag(tag{0, []attr{{1, noString, 0x03, 0}}}) // neither string nor integer
	if got := tagAttrs(untyped, pool); len(got) != 0 {
		t.Errorf("an unreadable value must be skipped: %v", got)
	}
	if stringPool(make([]byte, 10)) != nil {
		t.Error("a truncated pool has no strings")
	}
}

func TestPoolStringEdges(t *testing.T) {
	long := strings.Repeat("a", 200) // needs the two-byte UTF-8 length
	u8 := append([]byte{0x80 | byte(len(long)>>8), byte(len(long)), 0x80 | byte(len(long)>>8), byte(len(long))}, long...)
	if got := utf8At(u8, 0); got != long {
		t.Errorf("two-byte UTF-8 length: got %d chars", len(got))
	}
	for _, b := range [][]byte{{}, {0x80}, {5, 5, 'a'}} {
		if got := utf8At(b, 0); got != "" {
			t.Errorf("utf8At(%v) = %q, want empty", b, got)
		}
	}
	le := binary.LittleEndian
	u16 := make([]byte, 4)
	le.PutUint16(u16, 0x8000)
	le.PutUint16(u16[2:], 1)
	u16 = append(u16, 'z', 0)
	if got := utf16At(u16, 0); got != "z" {
		t.Errorf("long UTF-16 length: %q", got)
	}
	for _, b := range [][]byte{{}, {0x00, 0x80}, {9, 0, 'a', 0}} {
		if got := utf16At(b, 0); got != "" {
			t.Errorf("utf16At(%v) = %q, want empty", b, got)
		}
	}
}

// writeAPK zips a manifest and any extra (empty) files into an APK.
func writeAPK(t *testing.T, manifest []byte, files ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "app-release.apk")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	if manifest != nil {
		w, _ := zw.Create("AndroidManifest.xml")
		_, _ = w.Write(manifest)
	}
	for _, name := range files {
		_, _ = zw.Create(name)
	}
	_ = zw.Close()
	_ = f.Close()
	return p
}

func TestReadManifestErrors(t *testing.T) {
	for name, path := range map[string]string{
		"no manifest": writeAPK(t, nil, "classes.dex"),
		"corrupt":     corruptAPK(t),
	} {
		zr, err := zip.OpenReader(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := readManifest(&zr.Reader); err == nil {
			t.Errorf("%s: expected an error", name)
		}
		_ = zr.Close()
	}
}

// corruptAPK damages the compressed manifest bytes, so the zip opens but
// the read fails its checksum.
func corruptAPK(t *testing.T) string {
	t.Helper()
	p := writeAPK(t, bytes.Repeat([]byte("manifest"), 64))
	raw, _ := os.ReadFile(p)
	raw[55] ^= 0xff // inside the compressed data, past the 49-byte local header
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
