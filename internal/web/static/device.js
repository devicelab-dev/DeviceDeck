// DeviceDeck automation page: the device as a normal webpage.
//
// The native UI tree is mirrored as real DOM nodes positioned over the
// video — data-testid from accessibility identifiers, ARIA roles mapped
// from XCUITest element types, accessible text from labels/values. Any
// browser automation tool (Playwright, Cypress, Selenium, Puppeteer)
// drives the device through its own native protocol against this page;
// DeviceDeck implements none of their protocols.
//
// URL shape: /device/{udid}?app={bundleId}   (app scopes the UI tree)
"use strict";

let udid = decodeURIComponent(location.pathname.split("/")[2] || "booted");
const appId = new URLSearchParams(location.search).get("app") || "";
const canvas = document.getElementById("video");
const ctx = canvas.getContext("2d");
const mirror = document.getElementById("mirror");

// ---------- sidecar protocol frames (lockstep with Go/Swift) ----------

function touchFrame(phase, x, y) {
  const view = new DataView(new ArrayBuffer(10));
  view.setUint8(0, 0x01 + phase);
  view.setFloat32(1, x);
  view.setFloat32(5, y);
  view.setUint8(9, 0);
  return view.buffer;
}

function keyFrame(modifiers, usage) {
  const view = new DataView(new ArrayBuffer(6));
  view.setUint8(0, 0x0b);
  view.setUint8(1, modifiers);
  view.setUint32(2, usage);
  return view.buffer;
}

let inputWS = null;

function sendFrame(buffer) {
  if (inputWS && inputWS.readyState === WebSocket.OPEN) inputWS.send(buffer);
}

// ---------- video ----------

function wsURL(path) {
  const proto = location.protocol === "https:" ? "wss" : "ws";
  return `${proto}://${location.host}${path}`;
}

let decoder = null;

function connectVideo() {
  const ws = new WebSocket(wsURL(`/api/devices/${udid}/video`));
  ws.binaryType = "arraybuffer";
  ws.onmessage = ({ data }) => {
    const bytes = new Uint8Array(data);
    const payload = bytes.subarray(1);
    if (bytes[0] === 1) {
      configureDecoder(payload);
    } else if (decoder && decoder.state === "configured") {
      decoder.decode(new EncodedVideoChunk({
        type: bytes[0] === 2 ? "key" : "delta",
        timestamp: performance.now() * 1000,
        data: payload,
      }));
    }
  };
  ws.onclose = () => setTimeout(connectVideo, 1500);
}

function configureDecoder(avcC) {
  const hex = (b) => b.toString(16).padStart(2, "0");
  if (decoder) { try { decoder.close(); } catch {} }
  decoder = new VideoDecoder({
    output: (frame) => {
      if (canvas.width !== frame.displayWidth || canvas.height !== frame.displayHeight) {
        canvas.width = frame.displayWidth;
        canvas.height = frame.displayHeight;
        positionMirror();
      }
      ctx.drawImage(frame, 0, 0);
      frame.close();
    },
    error: () => {},
  });
  decoder.configure({
    codec: `avc1.${hex(avcC[1])}${hex(avcC[2])}${hex(avcC[3])}`,
    description: avcC,
    optimizeForLatency: true,
  });
}

function connectInput() {
  inputWS = new WebSocket(wsURL(`/api/devices/${udid}/input`));
  inputWS.binaryType = "arraybuffer";
  inputWS.onclose = () => setTimeout(connectInput, 1500);
}

// ---------- DOM mirror ----------

// XCUITest element type → ARIA role. Types without a sensible role
// become plain text carriers (getByText still finds them).
const ROLES = {
  Button: "button",
  Link: "link",
  TextField: "textbox",
  SecureTextField: "textbox",
  SearchField: "searchbox",
  TextView: "textbox",
  Switch: "switch",
  Toggle: "switch",
  Slider: "slider",
  CheckBox: "checkbox",
  RadioButton: "radio",
  SegmentedControl: "radiogroup",
  Picker: "listbox",
  PickerWheel: "listbox",
  Image: "img",
  Icon: "img",
  Cell: "listitem",
  Table: "list",
  CollectionView: "list",
  NavigationBar: "navigation",
  TabBar: "tablist",
  Tab: "tab",
  ToolBar: "toolbar",
  ActivityIndicator: "progressbar",
  ProgressIndicator: "progressbar",
  Alert: "alertdialog",
  Sheet: "dialog",
  Menu: "menu",
  MenuItem: "menuitem",
  StatusBar: "status",
};

let lastTreeJSON = "";

function positionMirror() {
  const stage = document.getElementById("stage").getBoundingClientRect();
  const rect = canvas.getBoundingClientRect();
  mirror.style.left = `${rect.left - stage.left}px`;
  mirror.style.top = `${rect.top - stage.top}px`;
  mirror.style.width = `${rect.width}px`;
  mirror.style.height = `${rect.height}px`;
}

function renderMirror(nodes) {
  const json = JSON.stringify(nodes);
  if (json === lastTreeJSON) return;
  lastTreeJSON = json;

  mirror.innerHTML = "";
  positionMirror();
  if (!nodes.length) return;
  const app = nodes[0].frame;
  if (!(app.width > 0) || !(app.height > 0)) return;

  for (const node of nodes) {
    if (node.depth === 0) continue;
    const f = node.frame;
    if (!(f.width > 0) || !(f.height > 0)) continue;
    if (!node.identifier && !node.label && !node.value && !ROLES[node.type]) continue;

    const el = document.createElement("div");
    el.setAttribute("data-dd-node", "");
    el.style.left = `${(f.x / app.width) * 100}%`;
    el.style.top = `${(f.y / app.height) * 100}%`;
    el.style.width = `${(f.width / app.width) * 100}%`;
    el.style.height = `${(f.height / app.height) * 100}%`;
    // Deeper nodes stack above their containers so the most specific
    // element receives the click, mirroring native hit-testing.
    el.style.zIndex = String(node.depth);

    const role = ROLES[node.type];
    if (role) el.setAttribute("role", role);
    if (node.identifier) el.setAttribute("data-testid", node.identifier);
    // Accessible name: label first, placeholder as fallback so unnamed
    // fields still read as `textbox "Username"` in aria snapshots.
    const name = node.label || node.placeholder || "";
    if (name) el.setAttribute("aria-label", name);
    if (node.placeholder) el.setAttribute("aria-placeholder", node.placeholder);
    if (!node.enabled) el.setAttribute("aria-disabled", "true");
    if (node.selected) el.setAttribute("aria-selected", "true");
    // Text content for getByText: label, else value, painted transparent.
    const text = node.label || node.value || "";
    if (text) el.textContent = text;

    mirror.appendChild(el);
  }
}

// ---------- tree sync: adaptive polling + refresh-after-action ----------

// Fast cadence while the screen is likely changing (recent action or
// recent tree change); idle cadence otherwise. Sequenced so a stale
// fetch never overwrites a newer one.
const FAST_MS = 300;
const IDLE_MS = 1000;
let lastActivity = 0;
let syncSeq = 0;

async function syncTree() {
  const seq = ++syncSeq;
  try {
    const url = `/api/devices/${udid}/tree${appId ? `?app=${encodeURIComponent(appId)}` : ""}`;
    const res = await fetch(url);
    if (res.ok && seq === syncSeq) {
      const before = lastTreeJSON;
      renderMirror((await res.json()).nodes);
      if (lastTreeJSON !== before) lastActivity = Date.now();
    }
  } catch {}
  const cadence = Date.now() - lastActivity < 5000 ? FAST_MS : IDLE_MS;
  setTimeout(syncTree, cadence);
}

function noteActivity() {
  lastActivity = Date.now();
}

// ---------- interaction forwarding ----------

function normalized(event) {
  const rect = canvas.getBoundingClientRect();
  const clamp = (v) => Math.min(1, Math.max(0, v));
  return {
    x: clamp((event.clientX - rect.left) / rect.width),
    y: clamp((event.clientY - rect.top) / rect.height),
  };
}

// Clicks land wherever the pointer is — the mirror node under it exists
// for *finding*; the device receives the true coordinates, exactly like
// a finger.
let pointerDown = false;
mirror.addEventListener("pointerdown", (e) => {
  pointerDown = true;
  // Synthetic events (Cypress, jsdom) may carry no capturable pointerId;
  // capture is an optimization for drags, never a precondition.
  try { mirror.setPointerCapture(e.pointerId); } catch {}
  const { x, y } = normalized(e);
  sendFrame(touchFrame(0, x, y));
  noteActivity();
});
mirror.addEventListener("pointermove", (e) => {
  if (!pointerDown) return;
  const { x, y } = normalized(e);
  sendFrame(touchFrame(1, x, y));
});
mirror.addEventListener("pointerup", (e) => {
  if (!pointerDown) return;
  pointerDown = false;
  const { x, y } = normalized(e);
  sendFrame(touchFrame(2, x, y));
  noteActivity();
});

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

document.addEventListener("keydown", (e) => {
  const usage = KEY_USAGE[e.key.length === 1 ? e.key.toLowerCase() : e.key];
  if (!usage) return;
  e.preventDefault();
  const modifiers = (e.ctrlKey ? 0x01 : 0) | (e.shiftKey ? 0x02 : 0) |
    (e.altKey ? 0x04 : 0) | (e.metaKey ? 0x08 : 0);
  sendFrame(keyFrame(modifiers, usage));
  noteActivity();
});

// ---------- scripting escape hatch ----------

// Gestures DOM events cannot express, callable from page.evaluate().
async function api(path, body) {
  noteActivity();
  const res = await fetch(`/api/devices/${udid}${path}`, {
    method: "POST",
    body: JSON.stringify(body),
  });
  return res.json();
}

window.devicedeck = {
  udid,
  tap: (x, y) => api("/tap", { x, y }),
  swipe: (x1, y1, x2, y2, durationMs = 250) => api("/swipe", { x1, y1, x2, y2, durationMs }),
  gesture: (kind) => api("/gesture", { kind }),
  button: (button) => api("/button", { button }),
  key: (usage, modifiers = 0) => api("/key", { usage, modifiers }),
};

window.addEventListener("resize", positionMirror);

// "booted" is a convenience alias; resolve it to the actual UDID up
// front — the tree engine needs a concrete device.
async function start() {
  if (udid === "booted") {
    try {
      const { devices } = await (await fetch("/api/devices")).json();
      if (devices.length) udid = devices[0].udid;
    } catch {}
  }
  connectVideo();
  connectInput();
  syncTree();
}
start();
