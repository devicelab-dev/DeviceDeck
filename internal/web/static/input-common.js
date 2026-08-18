// Shared input path for the console and the device page: the sidecar
// wire protocol ([type:u8][payload], lockstep with Go's internal/input
// and the Swift sidecar's Frame.swift) plus the socket that carries it.
"use strict";

const PHASE = { down: 0, move: 1, up: 2 };
const GESTURE = { home: 1, appSwitcher: 2, notificationCenter: 3, lockScreen: 4 };

function touchFrame(phase, x, y, edge = 0) {
  const view = new DataView(new ArrayBuffer(10));
  view.setUint8(0, 0x01 + phase);
  view.setFloat32(1, x);
  view.setFloat32(5, y);
  view.setUint8(9, edge);
  return view.buffer;
}

function keyFrame(modifiers, usage) {
  const view = new DataView(new ArrayBuffer(6));
  view.setUint8(0, 0x0b);
  view.setUint8(1, modifiers);
  view.setUint32(2, usage);
  return view.buffer;
}

function gestureFrame(kind) {
  return new Uint8Array([0x0c, kind]).buffer;
}

// Browser KeyboardEvent.key → HID usage. Only keys the sidecar can
// synthesize are listed; anything absent is left to the browser.
const KEY_USAGE = (() => {
  const map = {
    Enter: 0x28, Escape: 0x29, Backspace: 0x2a, Tab: 0x2b, " ": 0x2c,
    "-": 0x2d, "=": 0x2e, "[": 0x2f, "]": 0x30, "\\": 0x31, ";": 0x33,
    "'": 0x34, "`": 0x35, ",": 0x36, ".": 0x37, "/": 0x38,
    ArrowRight: 0x4f, ArrowLeft: 0x50, ArrowDown: 0x51, ArrowUp: 0x52,
  };
  for (let i = 0; i < 26; i++) map[String.fromCharCode(97 + i)] = 0x04 + i;
  "1234567890".split("").forEach((d, i) => { map[d] = 0x1e + i; });
  return map;
})();

// keyEventFrame encodes a KeyboardEvent, or returns null when the key
// has no HID equivalent (the caller then leaves the event alone).
function keyEventFrame(e) {
  const usage = KEY_USAGE[e.key.length === 1 ? e.key.toLowerCase() : e.key];
  if (!usage) return null;
  const modifiers = (e.ctrlKey ? 0x01 : 0) | (e.shiftKey ? 0x02 : 0) |
    (e.altKey ? 0x04 : 0) | (e.metaKey ? 0x08 : 0);
  return keyFrame(modifiers, usage);
}

// Frames produced before the socket finishes its handshake are queued,
// not dropped: a driver that navigates and immediately taps (a test's
// very first action) would otherwise lose the whole gesture silently,
// and the page looks alive regardless because the mirror renders from
// the tree, not from input.
const PENDING_MAX = 256;
const PENDING_STALE_MS = 2000;
const RECONNECT_MS = 1500;

// createInputSocket keeps one reconnecting input socket for a device.
// now is injectable so tests can drive the staleness cutoff.
function createInputSocket(url, { now = Date.now } = {}) {
  let ws = null;
  let closed = false;
  const pending = [];

  function flush() {
    const cutoff = now() - PENDING_STALE_MS;
    for (const item of pending.splice(0, pending.length)) {
      // Anything older than the reconnect backoff is discarded rather
      // than injected into whatever screen the device moved on to.
      if (item.at >= cutoff) ws.send(item.buffer);
    }
  }

  function connect() {
    ws = new WebSocket(url);
    ws.binaryType = "arraybuffer";
    ws.onopen = flush;
    ws.onclose = () => { if (!closed) setTimeout(connect, RECONNECT_MS); };
  }

  connect();

  return {
    send(buffer) {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(buffer);
        return;
      }
      if (pending.length >= PENDING_MAX) pending.shift();
      pending.push({ buffer, at: now() });
    },
    close() {
      closed = true;
      pending.length = 0;
      if (ws) ws.close();
    },
  };
}
