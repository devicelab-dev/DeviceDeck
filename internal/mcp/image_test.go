package mcp

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// testPNG is a w×h PNG, left half black and right half white.
func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := w / 2; x < w; x++ {
			img.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodeB64Image(t *testing.T, data string) image.Image {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestAgentImageCapsTheLongSide(t *testing.T) {
	tests := []struct {
		name         string
		w, h, max    int
		wantW, wantH int
	}{
		{"portrait phone", 1206, 2622, 1568, 721, 1568},
		{"landscape", 2622, 1206, 1568, 1568, 721},
		{"already small", 400, 800, 1568, 400, 800},
		{"no cap", 400, 800, 0, 400, 800},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, mime, err := agentImage(testPNG(t, tt.w, tt.h), tt.max)
			if err != nil || mime != "image/jpeg" {
				t.Fatalf("agentImage: %v %s", err, mime)
			}
			img, err := jpeg.Decode(bytes.NewReader(out))
			if err != nil {
				t.Fatal(err)
			}
			if b := img.Bounds(); b.Dx() != tt.wantW || b.Dy() != tt.wantH {
				t.Errorf("size = %dx%d, want %dx%d", b.Dx(), b.Dy(), tt.wantW, tt.wantH)
			}
		})
	}
}

// Averaging keeps content where it was: the left of the shrunk image is dark,
// the right is light.
func TestShrinkAverages(t *testing.T) {
	src, _ := png.Decode(bytes.NewReader(testPNG(t, 400, 200)))
	out := shrink(src, 100)
	if b := out.Bounds(); b.Dx() != 100 || b.Dy() != 50 {
		t.Fatalf("size = %v", b)
	}
	l, _, _, _ := out.At(10, 25).RGBA()
	r, _, _, _ := out.At(90, 25).RGBA()
	if l > 0x1000 || r < 0xF000 {
		t.Errorf("left %x right %x — content moved", l, r)
	}
	// A one-pixel-wide sliver still yields at least one pixel.
	thin := shrink(image.NewRGBA(image.Rect(0, 0, 1, 4000)), 100)
	if b := thin.Bounds(); b.Dx() != 1 || b.Dy() != 100 {
		t.Errorf("thin = %v", b)
	}
}

func TestAgentImageRejectsNonPNG(t *testing.T) {
	if _, _, err := agentImage([]byte("nope"), 100); err == nil {
		t.Error("want a decode error")
	}
}

// JPEG cannot hold a side of 65536 pixels or more; that fails loudly.
func TestAgentImageEncodeFailure(t *testing.T) {
	if _, _, err := agentImage(testPNG(t, 70000, 1), 0); err == nil {
		t.Error("want an encode error for an image JPEG cannot hold")
	}
}
