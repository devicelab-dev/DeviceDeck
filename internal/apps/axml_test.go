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

// axml builds a minimal compiled manifest: a string pool (UTF-8 or UTF-16)
// and one start tag carrying the given attributes (name index, value index).
func axml(strs []string, utf8 bool, attrs [][2]uint32) []byte {
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
	for body.Len()%4 != 0 {
		body.WriteByte(0)
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
	pool = append(pool, body.Bytes()...)

	tag := make([]byte, 36+attrSize*len(attrs))
	le.PutUint16(tag[0:], chunkStartTag)
	le.PutUint16(tag[2:], 16)
	le.PutUint32(tag[4:], uint32(len(tag)))
	le.PutUint16(tag[24:], 20) // attributes start 20 bytes after the header
	le.PutUint16(tag[26:], attrSize)
	le.PutUint16(tag[28:], uint16(len(attrs)))
	for i, a := range attrs {
		le.PutUint32(tag[36+attrSize*i+4:], a[0])
		le.PutUint32(tag[36+attrSize*i+8:], a[1])
	}
	head := make([]byte, 8)
	le.PutUint16(head, 0x0003)
	le.PutUint16(head[2:], 8)
	out := append(append(head, pool...), tag...)
	le.PutUint32(out[4:], uint32(len(out)))
	return out
}

func TestManifestPackage(t *testing.T) {
	strs := []string{"versionCode", "package", "com.example.app", "manifest"}
	pkg := [2]uint32{1, 2}
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"UTF-8 pool", axml(strs, true, [][2]uint32{{0, 3}, pkg}), "com.example.app"},
		{"UTF-16 pool", axml(strs, false, [][2]uint32{pkg}), "com.example.app"},
		{"no package attribute", axml(strs, true, [][2]uint32{{0, 3}}), ""},
		{"package value out of range", axml(strs, true, [][2]uint32{{1, 99}}), ""},
		{"no start tag", axml(strs, true, nil)[:8+28+4*len(strs)], ""},
		{"empty", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := manifestPackage(tc.data)
			if got != tc.want || (tc.want == "") != (err != nil) {
				t.Errorf("manifestPackage = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestMalformedChunks(t *testing.T) {
	good := axml([]string{"package", "com.x"}, true, [][2]uint32{{0, 1}})
	bad := append([]byte{}, good...)
	binary.LittleEndian.PutUint32(bad[12:], 4) // pool chunk claims 4 bytes
	if _, err := manifestPackage(bad); err == nil {
		t.Error("a chunk smaller than its header must stop the walk")
	}
	if _, err := packageAttr(make([]byte, 20), nil); err == nil {
		t.Error("a truncated tag must be an error")
	}
	tag := make([]byte, 36)
	binary.LittleEndian.PutUint16(tag[2:], 16)
	binary.LittleEndian.PutUint16(tag[24:], 100) // attributes start past the end
	binary.LittleEndian.PutUint16(tag[28:], 3)
	if _, err := packageAttr(tag, []string{"package"}); err == nil {
		t.Error("attributes past the chunk must be ignored")
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

func TestAPKPackageRealBuilds(t *testing.T) {
	for file, want := range map[string]string{
		"devicelab-android-driver.apk":      "dev.devicelab.driver.android",
		"devicelab-android-driver-test.apk": "dev.devicelab.driver.android.test",
	} {
		got, err := apkPackage(filepath.Join("..", "home", "android", file))
		if err != nil || got != want {
			t.Errorf("%s = %q, %v; want %q", file, got, err, want)
		}
	}
}

func TestAPKPackageErrors(t *testing.T) {
	dir := t.TempDir()
	notZip := filepath.Join(dir, "broken.apk")
	if err := os.WriteFile(notZip, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	noManifest := filepath.Join(dir, "empty.apk")
	f, _ := os.Create(noManifest)
	zw := zip.NewWriter(f)
	_, _ = zw.Create("classes.dex")
	_ = zw.Close()
	_ = f.Close()
	// A manifest whose compressed bytes are damaged: the zip opens, the
	// read fails its checksum.
	corrupt := filepath.Join(dir, "corrupt.apk")
	f, _ = os.Create(corrupt)
	zw = zip.NewWriter(f)
	w, _ := zw.Create("AndroidManifest.xml")
	_, _ = w.Write(bytes.Repeat([]byte("manifest"), 64))
	_ = zw.Close()
	_ = f.Close()
	raw, _ := os.ReadFile(corrupt)
	raw[55] ^= 0xff // inside the compressed data, past the 49-byte local header
	if err := os.WriteFile(corrupt, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{notZip, noManifest, corrupt} {
		if _, err := apkPackage(p); err == nil {
			t.Errorf("%s: expected an error", filepath.Base(p))
		}
	}
}
