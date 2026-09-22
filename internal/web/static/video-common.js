// Shared screen rendering for the console and the device page: the
// sidecar frame protocol ([type:u8][payload]) painted onto a canvas —
// H.264 via WebCodecs, PNG stills (type 4, Android static screens)
// drawn directly.
"use strict";

// createScreenRenderer wires a canvas to the video frame protocol.
// onResize fires when canvas dimensions change (each page repositions
// its own overlay); onStatus receives decoder state text (the console
// displays it, the device page passes nothing).
/**
 * @param {HTMLCanvasElement} canvas
 * @param {CanvasRenderingContext2D} ctx
 * @param {{onResize?: () => void, onStatus?: (text: string) => void}} [options]
 */
function createScreenRenderer(canvas, ctx, { onResize, onStatus } = {}) {
  const screen = { canvas, ctx, onResize, onStatus, decoder: null };
  return {
    // handleMessage dispatches one protocol frame received off the wire.
    handleMessage: (bytes) => screenHandleFrame(screen, bytes),
    // close releases the decoder; the renderer is done after this.
    close: () => screenClose(screen),
  };
}

// The renderer's steps, each taking the renderer's state. They live at the
// top level so no single function grows past the KISS limit; the screen
// prefix keeps them clear of the pages' own globals.

function screenResize(screen, width, height) {
  if (screen.canvas.width !== width || screen.canvas.height !== height) {
    screen.canvas.width = width;
    screen.canvas.height = height;
    screen.onResize?.();
  }
}

async function screenDrawStill(screen, png) {
  try {
    const bitmap = await createImageBitmap(new Blob([png], { type: "image/png" }));
    screenResize(screen, bitmap.width, bitmap.height);
    screen.ctx.drawImage(bitmap, 0, 0);
    bitmap.close();
  } catch {}
}

function screenConfigure(screen, avcC) {
  const hex = (b) => b.toString(16).padStart(2, "0");
  screenClose(screen);
  const codec = `avc1.${hex(avcC[1])}${hex(avcC[2])}${hex(avcC[3])}`;
  screen.decoder = new VideoDecoder({
    output: (frame) => {
      screenResize(screen, frame.displayWidth, frame.displayHeight);
      screen.ctx.drawImage(frame, 0, 0);
      frame.close();
    },
    error: (e) => screen.onStatus?.(`decode error: ${e.message}`),
  });
  screen.decoder.configure({ codec, description: avcC, optimizeForLatency: true });
  screen.onStatus?.(`streaming (${codec})`);
}

function screenHandleFrame(screen, bytes) {
  const payload = bytes.subarray(1);
  if (bytes[0] === 1) {
    screenConfigure(screen, payload);
  } else if (bytes[0] === 4) {
    screenDrawStill(screen, payload);
  } else if (screen.decoder && screen.decoder.state === "configured") {
    screen.decoder.decode(new EncodedVideoChunk({
      type: bytes[0] === 2 ? "key" : "delta",
      timestamp: performance.now() * 1000,
      data: payload,
    }));
  }
}

function screenClose(screen) {
  if (screen.decoder) { try { screen.decoder.close(); } catch {} screen.decoder = null; }
}
