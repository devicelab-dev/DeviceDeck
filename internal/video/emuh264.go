package video

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"

	"github.com/devicelab-dev/DeviceDeck/internal/emugrpc"
)

// Android H.264: the emulator's gRPC stream sends raw RGB888 frames, which
// are piped into devicedeck-video --stdin-frames — the same VideoToolbox
// encoder iOS uses — so Android viewers get an H.264 stream instead of one
// PNG per frame (~168KB each, which held scrolling to ~3 fps). Opt-in with
// DEVICEDECK_ANDROID_CAPTURE=h264 until it has been measured on devices.

// Environment the video manager passes to the Android capture process: the
// encoder binary (the video sidecar) and the frame rate. EncoderWidthEnv
// optionally asks the emulator to scale frames down to that width.
const (
	EncoderEnv      = "DEVICEDECK_VIDEO_ENCODER"
	EncoderFPSEnv   = "DEVICEDECK_VIDEO_FPS"
	EncoderWidthEnv = "DEVICEDECK_ANDROID_VIDEO_WIDTH"
)

// frameEncoder is a running devicedeck-video --stdin-frames. Frame and
// keyframe writes share its stdin, so they are serialized.
type frameEncoder struct {
	mu  sync.Mutex
	cmd *exec.Cmd
	in  io.WriteCloser
	out io.ReadCloser
}

// startEncoder launches the encoder. Its stdout carries framed H.264 in the
// same format the iOS sidecar writes.
//
// Coverage waiver: StdinPipe and StdoutPipe only fail when the command's
// pipes are already wired or it has started — impossible for the fresh cmd
// built here, as in runOnce.
func startEncoder(path string, fps int) (*frameEncoder, error) {
	if path == "" {
		return nil, fmt.Errorf("no video encoder: %s is not set", EncoderEnv)
	}
	cmd := exec.Command(path, "--stdin-frames", strconv.Itoa(fps))
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start video encoder: %w", err)
	}
	return &frameEncoder{cmd: cmd, in: in, out: out}, nil
}

// frame sends one RGB888 frame: 'F', width, height, length, pixels.
func (e *frameEncoder) frame(width, height uint32, rgb []byte) error {
	header := make([]byte, 13)
	header[0] = 'F'
	binary.BigEndian.PutUint32(header[1:], width)
	binary.BigEndian.PutUint32(header[5:], height)
	binary.BigEndian.PutUint32(header[9:], uint32(len(rgb)))
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.in.Write(header); err != nil {
		return err
	}
	_, err := e.in.Write(rgb)
	return err
}

// keyframe asks for the next frame, or the last one again, as a keyframe.
func (e *frameEncoder) keyframe() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.in.Write([]byte{'K'})
	return err
}

// close ends the encoder: EOF on its stdin makes it exit.
func (e *frameEncoder) close() {
	_ = e.in.Close()
	_ = e.cmd.Wait()
}

// streamH264 runs the H.264 path until shutdown: start the encoder, relay
// its output, and feed it the emulator's raw frames.
func (c *androidCapture) streamH264(ep emulatorEndpoint) error {
	fps, _ := strconv.Atoi(os.Getenv(EncoderFPSEnv))
	if fps <= 0 {
		fps = 30
	}
	enc, err := startEncoder(os.Getenv(EncoderEnv), fps)
	if err != nil {
		return err
	}
	defer enc.close()
	c.mu.Lock()
	c.encoder = enc
	c.mu.Unlock()
	go func() { _, _ = io.Copy(c.out, enc.out) }() // already framed; one producer
	return c.streamGRPC(ep, func(client emugrpc.EmulatorControllerClient) error {
		return c.pumpRawStream(client, enc)
	})
}

// pumpRawStream forwards one raw screenshot stream's frames to the encoder.
func (c *androidCapture) pumpRawStream(client emugrpc.EmulatorControllerClient, enc *frameEncoder) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.setCancel(cancel)
	width, _ := strconv.ParseUint(os.Getenv(EncoderWidthEnv), 10, 32)
	stream, err := client.StreamScreenshot(ctx, &emugrpc.ImageFormat{
		Format: emugrpc.ImageFormat_RGB888,
		Width:  uint32(width),
	})
	if err != nil {
		return err
	}
	for {
		img, err := stream.Recv()
		if err != nil {
			return err
		}
		f := img.GetFormat()
		if len(img.GetImage()) == 0 || f.GetWidth() == 0 || f.GetHeight() == 0 {
			continue
		}
		if err := enc.frame(f.GetWidth(), f.GetHeight(), img.GetImage()); err != nil {
			return fmt.Errorf("video encoder: %w", err)
		}
	}
}
