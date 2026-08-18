// Shared screen rendering for the console and the device page: the
// sidecar frame protocol ([type:u8][payload]) painted onto a canvas —
// H.264 via WebCodecs, PNG stills (type 4, Android static screens)
// drawn directly.
"use strict";

// createScreenRenderer wires a canvas to the video frame protocol.
// onResize fires when canvas dimensions change (each page repositions
// its own overlay); onStatus receives decoder state text (the console
// displays it, the device page passes nothing).
function createScreenRenderer(canvas, ctx, { onResize, onStatus } = {}) {
  let decoder = null;

  function resize(width, height) {
    if (canvas.width !== width || canvas.height !== height) {
      canvas.width = width;
      canvas.height = height;
      onResize?.();
    }
  }

  async function drawStill(png) {
    try {
      const bitmap = await createImageBitmap(new Blob([png], { type: "image/png" }));
      resize(bitmap.width, bitmap.height);
      ctx.drawImage(bitmap, 0, 0);
      bitmap.close();
    } catch {}
  }

  function configureDecoder(avcC) {
    const hex = (b) => b.toString(16).padStart(2, "0");
    if (decoder) { try { decoder.close(); } catch {} }
    const codec = `avc1.${hex(avcC[1])}${hex(avcC[2])}${hex(avcC[3])}`;
    decoder = new VideoDecoder({
      output: (frame) => {
        resize(frame.displayWidth, frame.displayHeight);
        ctx.drawImage(frame, 0, 0);
        frame.close();
      },
      error: (e) => onStatus?.(`decode error: ${e.message}`),
    });
    decoder.configure({ codec, description: avcC, optimizeForLatency: true });
    onStatus?.(`streaming (${codec})`);
  }

  // handleMessage dispatches one protocol frame received off the wire.
  function handleMessage(bytes) {
    const payload = bytes.subarray(1);
    if (bytes[0] === 1) {
      configureDecoder(payload);
    } else if (bytes[0] === 4) {
      drawStill(payload);
    } else if (decoder && decoder.state === "configured") {
      decoder.decode(new EncodedVideoChunk({
        type: bytes[0] === 2 ? "key" : "delta",
        timestamp: performance.now() * 1000,
        data: payload,
      }));
    }
  }

  // close releases the decoder; the renderer is done after this.
  function close() {
    if (decoder) { try { decoder.close(); } catch {} decoder = null; }
  }

  return { handleMessage, close };
}
