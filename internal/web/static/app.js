// DeviceDeck console: H.264 over WebSocket → WebCodecs → canvas, pointer
// and keyboard events → sidecar protocol frames → input WebSocket, and a
// tree inspector overlay (the visible variant of the DOM mirror).
"use strict";

const $ = (id) => document.getElementById(id);
const canvas = $("video");
const ctx = canvas.getContext("2d");
const overlay = $("overlay");
const status = $("status");

let udid = null;
let videoWS = null;
let inputWS = null;
let decoder = null;
let pointerDown = false;
let inspecting = false;

// ---------- sidecar protocol frame encoders (lockstep with Go/Swift) ----------

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

function sendFrame(buffer) {
  if (inputWS && inputWS.readyState === WebSocket.OPEN) inputWS.send(buffer);
}

// ---------- devices ----------

async function loadDevices() {
  const res = await fetch("/api/devices");
  const { devices } = await res.json();
  const select = $("devices");
  select.innerHTML = "";
  for (const d of devices) {
    const opt = document.createElement("option");
    opt.value = d.udid;
    opt.textContent = `${d.name} (${d.os})`;
    select.appendChild(opt);
  }
  if (devices.length) selectDevice(devices[0].udid);
  else status.textContent = "no booted simulators";
}

function selectDevice(next) {
  udid = next;
  $("devices").value = udid;
  connectVideo();
  connectInput();
}

// ---------- video ----------

function wsURL(path) {
  const proto = location.protocol === "https:" ? "wss" : "ws";
  return `${proto}://${location.host}${path}`;
}

function connectVideo() {
  if (videoWS) videoWS.close();
  if (decoder) { try { decoder.close(); } catch {} decoder = null; }

  videoWS = new WebSocket(wsURL(`/api/devices/${udid}/video`));
  videoWS.binaryType = "arraybuffer";
  status.textContent = "connecting video…";

  videoWS.onmessage = ({ data }) => {
    const bytes = new Uint8Array(data);
    const type = bytes[0];
    const payload = bytes.subarray(1);
    if (type === 1) {
      configureDecoder(payload);
    } else if (decoder && decoder.state === "configured") {
      decoder.decode(new EncodedVideoChunk({
        type: type === 2 ? "key" : "delta",
        timestamp: performance.now() * 1000,
        data: payload,
      }));
    }
  };
  videoWS.onclose = () => { status.textContent = "video disconnected"; };
}

function configureDecoder(avcC) {
  const hex = (b) => b.toString(16).padStart(2, "0");
  const codec = `avc1.${hex(avcC[1])}${hex(avcC[2])}${hex(avcC[3])}`;
  decoder = new VideoDecoder({
    output: (frame) => {
      if (canvas.width !== frame.displayWidth || canvas.height !== frame.displayHeight) {
        canvas.width = frame.displayWidth;
        canvas.height = frame.displayHeight;
        positionOverlay();
      }
      ctx.drawImage(frame, 0, 0);
      frame.close();
    },
    error: (e) => { status.textContent = `decode error: ${e.message}`; },
  });
  decoder.configure({ codec, description: avcC, optimizeForLatency: true });
  status.textContent = `streaming (${codec})`;
}

// ---------- live input ----------

function connectInput() {
  if (inputWS) inputWS.close();
  inputWS = new WebSocket(wsURL(`/api/devices/${udid}/input`));
  inputWS.binaryType = "arraybuffer";
}

function normalized(event) {
  const rect = canvas.getBoundingClientRect();
  const clamp = (v) => Math.min(1, Math.max(0, v));
  return {
    x: clamp((event.clientX - rect.left) / rect.width),
    y: clamp((event.clientY - rect.top) / rect.height),
  };
}

canvas.addEventListener("pointerdown", (e) => {
  canvas.focus();
  canvas.setPointerCapture(e.pointerId);
  pointerDown = true;
  const { x, y } = normalized(e);
  sendFrame(touchFrame(PHASE.down, x, y));
});

canvas.addEventListener("pointermove", (e) => {
  if (!pointerDown) return;
  const { x, y } = normalized(e);
  sendFrame(touchFrame(PHASE.move, x, y));
});

canvas.addEventListener("pointerup", (e) => {
  if (!pointerDown) return;
  pointerDown = false;
  const { x, y } = normalized(e);
  sendFrame(touchFrame(PHASE.up, x, y));
});

// USB HID keyboard usages for the keys the console forwards.
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

canvas.addEventListener("keydown", (e) => {
  const usage = KEY_USAGE[e.key.length === 1 ? e.key.toLowerCase() : e.key];
  if (!usage) return;
  e.preventDefault();
  const modifiers = (e.ctrlKey ? 0x01 : 0) | (e.shiftKey ? 0x02 : 0) |
    (e.altKey ? 0x04 : 0) | (e.metaKey ? 0x08 : 0);
  sendFrame(keyFrame(modifiers, usage));
});

// ---------- toolbar ----------

$("devices").addEventListener("change", (e) => selectDevice(e.target.value));
$("btn-home").addEventListener("click", () => sendFrame(gestureFrame(GESTURE.home)));
$("btn-switcher").addEventListener("click", () => sendFrame(gestureFrame(GESTURE.appSwitcher)));
$("btn-lock").addEventListener("click", () =>
  fetch(`/api/devices/${udid}/button`, { method: "POST", body: JSON.stringify({ button: "lock" }) }));
$("btn-shot").addEventListener("click", () => window.open(`/api/devices/${udid}/screenshot`));
$("btn-inspect").addEventListener("click", toggleInspector);

// ---------- flow capture ----------

let recording = false;
let capturePoll = null;

$("btn-record").addEventListener("click", toggleRecord);

async function toggleRecord() {
  if (!recording) {
    const app = $("app").value.trim();
    if (!app) {
      status.textContent = "enter an app bundle id to record";
      $("app").focus();
      return;
    }
    status.textContent = "starting capture (tree warm-up)…";
    const res = await fetch(`/api/devices/${udid}/capture/start`, {
      method: "POST",
      body: JSON.stringify({ app }),
    });
    const body = await res.json();
    if (!res.ok) {
      status.textContent = `record: ${body.error}`;
      return;
    }
    recording = true;
    $("btn-record").classList.add("recording");
    $("btn-record").innerHTML = "&#9632; Stop";
    capturePoll = setInterval(pollCapture, 1000);
    status.textContent = "recording — drive the device";
  } else {
    clearInterval(capturePoll);
    const res = await fetch(`/api/devices/${udid}/capture/stop`, { method: "POST", body: "{}" });
    const body = await res.json();
    recording = false;
    $("btn-record").classList.remove("recording");
    $("btn-record").innerHTML = "&#9679; Record";
    if (!res.ok) {
      status.textContent = `stop: ${body.error}`;
      return;
    }
    showFlow(body.yaml, body.steps);
  }
}

async function pollCapture() {
  const res = await fetch(`/api/devices/${udid}/capture`);
  const body = await res.json();
  if (body.recording) {
    status.textContent = `recording — ${body.steps.length} steps`;
  }
}

function showFlow(yaml, steps) {
  $("panel").hidden = false;
  $("node-title").textContent = `Captured flow — ${steps.length} steps`;
  $("node-info").textContent = yaml;
  let link = $("panel").querySelector("a.download");
  if (!link) {
    link = document.createElement("a");
    link.className = "download";
    link.textContent = "Download flow.yaml";
    $("panel").appendChild(link);
  }
  link.href = URL.createObjectURL(new Blob([yaml], { type: "text/yaml" }));
  link.download = "flow.yaml";
  status.textContent = `captured ${steps.length} steps`;
}

// ---------- inspector overlay ----------

function positionOverlay() {
  const stage = $("stage").getBoundingClientRect();
  const rect = canvas.getBoundingClientRect();
  overlay.style.left = `${rect.left - stage.left}px`;
  overlay.style.top = `${rect.top - stage.top}px`;
  overlay.style.width = `${rect.width}px`;
  overlay.style.height = `${rect.height}px`;
}

async function toggleInspector() {
  inspecting = !inspecting;
  $("btn-inspect").classList.toggle("active", inspecting);
  $("panel").hidden = !inspecting;
  overlay.hidden = !inspecting;
  if (inspecting) await refreshTree();
  else overlay.innerHTML = "";
}

async function refreshTree() {
  status.textContent = "fetching tree…";
  const app = $("app").value.trim();
  const url = `/api/devices/${udid}/tree${app ? `?app=${encodeURIComponent(app)}` : ""}`;
  const res = await fetch(url);
  const body = await res.json();
  if (!res.ok) {
    status.textContent = `tree: ${body.error || res.status}`;
    return;
  }
  renderOverlay(body.nodes);
  status.textContent = `tree: ${body.nodes.length} nodes`;
}

function renderOverlay(nodes) {
  overlay.innerHTML = "";
  positionOverlay();
  if (!nodes.length) return;
  const app = nodes[0].frame; // application bounds in points
  for (const node of nodes) {
    const f = node.frame;
    if (node.depth === 0 || f.width <= 0 || f.height <= 0) continue;
    if (!node.identifier && !node.label && !node.hittable) continue;
    const el = document.createElement("div");
    el.className = "node";
    el.style.left = `${(f.x / app.width) * 100}%`;
    el.style.top = `${(f.y / app.height) * 100}%`;
    el.style.width = `${(f.width / app.width) * 100}%`;
    el.style.height = `${(f.height / app.height) * 100}%`;
    if (node.identifier) el.dataset.testid = node.identifier;
    if (node.label) el.dataset.label = node.label;
    el.dataset.type = node.type;
    el.addEventListener("mouseenter", () => showNode(node));
    el.addEventListener("click", (e) => {
      e.stopPropagation();
      tapNode(node, app);
    });
    overlay.appendChild(el);
  }
}

function showNode(node) {
  $("node-title").textContent = node.identifier || node.label || node.type;
  $("node-info").textContent = JSON.stringify(node, null, 2);
}

async function tapNode(node, app) {
  const x = (node.frame.x + node.frame.width / 2) / app.width;
  const y = (node.frame.y + node.frame.height / 2) / app.height;
  await fetch(`/api/devices/${udid}/tap`, {
    method: "POST",
    body: JSON.stringify({ x, y }),
  });
  // The tap likely changed the screen; refresh so the mirror tracks it.
  setTimeout(refreshTree, 600);
}

window.addEventListener("resize", positionOverlay);
loadDevices();
