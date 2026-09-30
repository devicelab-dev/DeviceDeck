package mcp

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
)

// Screenshots go into the agent's conversation, and a model API refuses an
// image past its size limits — Anthropic's at 2000px on a side (many images)
// or 5MB — and then every later turn fails too, because the image stays in the
// history: the agent's session is lost. A simulator screenshot is often
// 1206x2622 and several MB of PNG. So the tool sends a downscaled JPEG by
// default: the long side at most agentImageMax (the size Anthropic recommends,
// which it would scale to anyway), at agentImageQuality. `full` asks for the
// original PNG when a detail needs it.
const (
	agentImageMax     = 1568
	agentImageQuality = 80
)

// agentImage turns a device PNG into what is safe to hand an agent: a JPEG
// whose long side is at most maxSide. It returns the bytes and their MIME type.
func agentImage(pngBytes []byte, maxSide int) ([]byte, string, error) {
	src, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, "", fmt.Errorf("decode screenshot: %w", err)
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, shrink(src, maxSide), &jpeg.Options{Quality: agentImageQuality}); err != nil {
		return nil, "", fmt.Errorf("encode screenshot: %w", err)
	}
	return out.Bytes(), "image/jpeg", nil
}

// shrink scales src down so its long side is at most maxSide, averaging each
// destination pixel's source area so text stays legible. An image already
// small enough is returned as is.
func shrink(src image.Image, maxSide int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	long := max(w, h)
	if long <= maxSide || maxSide <= 0 {
		return src
	}
	dw, dh := max(w*maxSide/long, 1), max(h*maxSide/long, 1)
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		y0, y1 := b.Min.Y+y*h/dh, b.Min.Y+max((y+1)*h/dh, y*h/dh+1)
		for x := 0; x < dw; x++ {
			x0, x1 := b.Min.X+x*w/dw, b.Min.X+max((x+1)*w/dw, x*w/dw+1)
			dst.Set(x, y, average(src, x0, y0, x1, y1))
		}
	}
	return dst
}

// average is the mean colour of src over [x0,x1)×[y0,y1).
func average(src image.Image, x0, y0, x1, y1 int) rgba {
	var r, g, b, a, n uint64
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			cr, cg, cb, ca := src.At(x, y).RGBA()
			r, g, b, a, n = r+uint64(cr), g+uint64(cg), b+uint64(cb), a+uint64(ca), n+1
		}
	}
	return rgba{uint16(r / n), uint16(g / n), uint16(b / n), uint16(a / n)}
}

// rgba is a 16-bit-per-channel colour, as image/color reports them.
type rgba struct{ r, g, b, a uint16 }

// RGBA implements color.Color.
func (c rgba) RGBA() (r, g, b, a uint32) {
	return uint32(c.r), uint32(c.g), uint32(c.b), uint32(c.a)
}
