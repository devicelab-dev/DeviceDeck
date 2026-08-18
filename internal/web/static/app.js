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

async function loadDevices(preselect) {
  const res = await fetch("/api/devices");
  const { devices } = await res.json();
  const select = $("devices");
  select.innerHTML = "";
  // The whole inventory, running first: users see every device they
  // could use, and picking a stopped one boots it.
  const groups = [
    { label: "Running", items: devices.filter((d) => d.booted) },
    { label: "Available (select to boot)", items: devices.filter((d) => !d.booted) },
  ];
  for (const g of groups) {
    if (!g.items.length) continue;
    const optgroup = document.createElement("optgroup");
    optgroup.label = g.label;
    for (const d of g.items) {
      const opt = document.createElement("option");
      opt.value = d.udid;
      opt.textContent = `${d.name} (${d.os})`;
      optgroup.appendChild(opt);
    }
    select.appendChild(optgroup);
  }
  const running = groups[0].items;
  if (preselect && running.some((d) => d.udid === preselect)) selectDevice(preselect);
  else if (running.length) selectDevice(running[0].udid);
  else if (devices.length) status.textContent = "no running devices — pick one to boot it";
  else status.textContent = "no simulators or emulators found";
  return devices;
}

function selectDevice(next) {
  udid = next;
  $("devices").value = udid;
  connectVideo();
  connectInput();
}

// bootDevice starts a stopped device and polls until it shows up
// running, then connects to it. AVDs come back under a fresh adb serial,
// so polling watches for any newly running device rather than the id.
async function bootDevice(id) {
  status.textContent = "booting…";
  const before = new Set(
    (await (await fetch("/api/devices")).json()).devices.filter((d) => d.booted).map((d) => d.udid));
  const res = await fetch(`/api/devices/${encodeURIComponent(id)}/boot`, { method: "POST", body: "{}" });
  if (!res.ok) {
    status.textContent = `boot failed: ${(await res.json()).error}`;
    return;
  }
  const deadline = Date.now() + 120_000;
  const poll = async () => {
    const { devices } = await (await fetch("/api/devices")).json();
    const fresh = devices.find((d) => d.booted && (d.udid === id || !before.has(d.udid)));
    if (fresh) {
      await loadDevices(fresh.udid);
      return;
    }
    if (Date.now() > deadline) {
      status.textContent = "boot timed out — check the device manually";
      return;
    }
    status.textContent = `booting… (${Math.round((deadline - Date.now()) / 1000)}s left)`;
    setTimeout(poll, 2000);
  };
  setTimeout(poll, 2000);
}

// ---------- video ----------

function wsURL(path) {
  const proto = location.protocol === "https:" ? "wss" : "ws";
  return `${proto}://${location.host}${path}`;
}

// Frame handling shared with the device page (video-common.js).
const renderer = createScreenRenderer(canvas, ctx, {
  onResize: () => positionOverlay(),
  onStatus: (text) => { status.textContent = text; },
});

function connectVideo() {
  if (videoWS) videoWS.close();

  videoWS = new WebSocket(wsURL(`/api/devices/${udid}/video`));
  videoWS.binaryType = "arraybuffer";
  status.textContent = "connecting video…";

  videoWS.onmessage = ({ data }) => renderer.handleMessage(new Uint8Array(data));
  videoWS.onclose = () => { status.textContent = "video disconnected"; };
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

$("devices").addEventListener("change", (e) => {
  const opt = e.target.selectedOptions[0];
  if (opt && opt.parentElement.label && opt.parentElement.label.startsWith("Available")) {
    bootDevice(opt.value);
  } else {
    selectDevice(e.target.value);
  }
});
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
    seenSteps = 0;
    $("btn-record").classList.add("recording");
    $("btn-record").innerHTML = "&#9632; Stop";
    capturePoll = setInterval(pollCapture, 700);
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

let seenSteps = 0;

async function pollCapture() {
  const res = await fetch(`/api/devices/${udid}/capture`);
  const body = await res.json();
  if (!body.recording) return;
  status.textContent = `recording — ${body.steps.length} steps`;
  if (body.steps.length > seenSteps) {
    const step = body.steps[body.steps.length - 1];
    if (step.bounds) flashResolved(step);
    seenSteps = body.steps.length;
  }
}

// Flash a highlight rectangle over the video where the last recorded step
// resolved, labeled with its selector — live confirmation of what the
// captured flow will actually target.
function flashResolved(step) {
  positionOverlay();
  overlay.hidden = false;
  const el = document.createElement("div");
  el.className = "resolved-flash";
  el.style.left = `${step.bounds.x * 100}%`;
  el.style.top = `${step.bounds.y * 100}%`;
  el.style.width = `${step.bounds.width * 100}%`;
  el.style.height = `${step.bounds.height * 100}%`;
  const tag = document.createElement("span");
  tag.textContent = step.id ? `id: ${step.id}` : `text: ${step.text}`;
  el.appendChild(tag);
  overlay.appendChild(el);
  setTimeout(() => {
    el.remove();
    if (!inspecting && !overlay.children.length) overlay.hidden = true;
  }, 1600);
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
