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
let input = null;
let pointerDown = false;
let inspecting = false;

// ---------- devices ----------

// The console has two views: the device library (pick or boot a device)
// and the streaming console for one device. The URL carries the state
// (/?device=UDID) so consoles are linkable and reload-safe.

function showLibrary() {
  if (videoWS) { videoWS.close(); videoWS = null; }
  if (input) { input.close(); input = null; }
  renderer.close();
  udid = null;
  $("library").hidden = false;
  $("console-view").hidden = true;
  $("console-controls").hidden = true;
  $("console-actions").hidden = true;
  $("stage-loading").hidden = true;
  canvas.classList.remove("connecting");
  status.textContent = "";
  if (new URLSearchParams(location.search).get("device")) {
    history.pushState({}, "", location.pathname);
  }
  refreshLibrary();
}

let awaitingFirstFrame = false;
let loadingShownAt = 0;

// The connecting state stays up at least this long even when the first
// frame is instant — a flash of loading reads as a glitch, a beat of it
// reads as arrival.
const MIN_LOADING_MS = 700;

function showConsole(next, name, shape) {
  udid = next;
  $("library").hidden = true;
  $("console-view").hidden = false;
  $("console-controls").hidden = false;
  $("console-actions").hidden = false;
  $("device-label").textContent = name || next;
  // The first frame can take seconds on a cold session — show the
  // pulsing silhouette instead of an empty canvas until it arrives.
  awaitingFirstFrame = true;
  loadingShownAt = Date.now();
  $("device-frame").className = "device-frame" +
    (shape && shape.tablet ? " tablet" : "") + (shape && shape.android ? " android" : "");
  $("loading-text").textContent = `Connecting to ${name || next}…`;
  $("stage-loading").hidden = false;
  canvas.classList.add("connecting");
  if (new URLSearchParams(location.search).get("device") !== next) {
    history.pushState({}, "", `${location.pathname}?device=${encodeURIComponent(next)}`);
  }
  connectVideo();
  connectInput();
}

async function refreshLibrary() {
  const { devices } = await (await fetch("/api/devices")).json();
  const groups = $("library-groups");
  groups.innerHTML = "";
  const running = devices.filter((d) => d.booted);
  const stopped = devices.filter((d) => !d.booted);
  $("library-summary").textContent =
    devices.length ? `${running.length} running · ${stopped.length} available` : "";

  if (!devices.length) {
    const empty = document.createElement("p");
    empty.className = "library-empty";
    empty.textContent =
      "No simulators or emulators found. Add simulators in Xcode " +
      "(Settings → Platforms) or create a virtual device in Android Studio, " +
      "then reload.";
    groups.appendChild(empty);
    return devices;
  }
  for (const g of [
    { label: `Running · ${running.length}`, items: running },
    { label: `Available · ${stopped.length}`, items: stopped },
  ]) {
    if (!g.items.length) continue;
    const section = document.createElement("div");
    section.className = "device-group";
    const label = document.createElement("div");
    label.className = "group-label";
    label.textContent = g.label;
    section.appendChild(label);
    const grid = document.createElement("div");
    grid.className = "device-grid";
    for (const d of g.items) grid.appendChild(deviceCard(d));
    section.appendChild(grid);
    groups.appendChild(section);
  }
  return devices;
}

// deviceCard renders one device: a state-lit silhouette, the details,
// and its single action (Use when running, Boot when stopped).
function deviceCard(d) {
  const card = document.createElement("div");
  card.className = "device-card" + (d.booted ? " running" : "");

  const glyph = document.createElement("div");
  const isTablet = /iPad|Tablet/i.test(d.name);
  const isAndroid = d.os.startsWith("android");
  glyph.className = "glyph" + (isTablet ? " tablet" : "") +
    (isAndroid ? " android" : "") + (d.booted ? " on" : "");
  const screen = Object.assign(document.createElement("div"), { className: "screen" });
  screen.appendChild(platformBadge(isAndroid)); // boot-splash style
  glyph.appendChild(screen);
  card.appendChild(glyph);

  const meta = document.createElement("div");
  meta.className = "device-meta";
  const name = Object.assign(document.createElement("div"), { className: "device-name", textContent: d.name, title: d.name });
  const os = Object.assign(document.createElement("div"), { className: "device-os", textContent: d.os });
  const id = Object.assign(document.createElement("div"), { className: "device-id", textContent: d.udid, title: d.udid });
  meta.append(name, os, id);
  card.appendChild(meta);

  const action = document.createElement("button");
  if (d.booted) {
    action.className = "use";
    action.textContent = "Use";
    action.addEventListener("click", () => showConsole(d.udid, d.name, deviceShape(d)));
  } else {
    action.textContent = "Boot";
    action.addEventListener("click", () => bootDevice(d, card, glyph, action));
  }
  card.appendChild(action);
  return card;
}

// bootDevice starts a stopped device and polls until it comes up, then
// enters its console. AVDs come back under a fresh adb serial, so the
// poll watches for any newly running device rather than the id.
async function bootDevice(d, card, glyph, action) {
  action.disabled = true;
  action.textContent = "Starting…";
  glyph.classList.add("booting");
  const before = new Set(
    (await (await fetch("/api/devices")).json()).devices.filter((x) => x.booted).map((x) => x.udid));
  const res = await fetch(`/api/devices/${encodeURIComponent(d.udid)}/boot`, { method: "POST", body: "{}" });
  if (!res.ok) {
    action.disabled = false;
    action.textContent = "Boot";
    glyph.classList.remove("booting");
    status.textContent = `boot failed: ${(await res.json()).error}`;
    return;
  }
  const deadline = Date.now() + 120_000;
  const poll = async () => {
    if (udid) return; // user entered another console meanwhile
    const { devices } = await (await fetch("/api/devices")).json();
    const fresh = devices.find((x) => x.booted && (x.udid === d.udid || !before.has(x.udid)));
    if (fresh) {
      showConsole(fresh.udid, fresh.name, deviceShape(fresh));
      return;
    }
    if (Date.now() > deadline) {
      action.disabled = false;
      action.textContent = "Boot";
      glyph.classList.remove("booting");
      status.textContent = "boot timed out — check the device manually";
      return;
    }
    setTimeout(poll, 2000);
  };
  setTimeout(poll, 2000);
}

function selectDevice(next) {
  showConsole(next);
}

// deviceShape classifies a device for the connecting frame.
function deviceShape(d) {
  return { tablet: /iPad|Tablet/i.test(d.name), android: d.os.startsWith("android") };
}

// platformBadge returns the platform mark: the Android robot head as
// inline SVG, or the Apple glyph — the system font renders it, and this
// console only runs on Macs.
function platformBadge(isAndroid) {
  const badge = document.createElement("span");
  badge.className = "platform-badge" + (isAndroid ? " android" : " apple");
  if (isAndroid) {
    badge.innerHTML =
      '<svg viewBox="0 0 24 15" aria-label="Android" role="img">' +
      '<path fill="#3DDC84" d="M17.5 4.6l1.7-2.9a.35.35 0 0 0-.6-.35l-1.7 3A10.6 10.6 0 0 0 12 3.4c-1.75 0-3.4.33-4.9.95l-1.7-3a.35.35 0 0 0-.6.35l1.7 2.9C3.6 6.2 1.7 9 1.5 12.3h21c-.2-3.3-2.1-6.1-5-7.7z"/>' +
      '<circle fill="#101216" cx="7.6" cy="9" r="1"/>' +
      '<circle fill="#101216" cx="16.4" cy="9" r="1"/></svg>';
  } else {
    badge.textContent = "\uF8FF"; // Apple logo glyph in the system font
    badge.setAttribute("aria-label", "Apple");
  }
  return badge;
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

  videoWS.onmessage = ({ data }) => {
    const bytes = new Uint8Array(data);
    // Types 2 (keyframe), 3 (delta), 4 (still) all paint pixels; the
    // decoder description (1) alone does not.
    if (awaitingFirstFrame && bytes[0] !== 1) {
      awaitingFirstFrame = false;
      const device = udid;
      const reveal = () => {
        if (udid !== device) return; // navigated away meanwhile
        $("stage-loading").hidden = true;
        canvas.classList.remove("connecting");
        // Still-based streams (Android static screens) never configure a
        // decoder, so nothing else clears the connecting status.
        if (status.textContent === "connecting video…") status.textContent = "live";
      };
      const remaining = MIN_LOADING_MS - (Date.now() - loadingShownAt);
      if (remaining > 0) setTimeout(reveal, remaining);
      else reveal();
    }
    renderer.handleMessage(bytes);
  };
  videoWS.onclose = () => { status.textContent = "video disconnected"; };
}

// ---------- live input ----------

function connectInput() {
  if (input) input.close();
  input = createInputSocket(wsURL(`/api/devices/${udid}/input`));
}

// Pointer and toolbar handlers stay bound while the library view is
// showing, where there is no device and no socket — drop those frames
// instead of throwing.
function send(frame) {
  input?.send(frame);
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
  send(touchFrame(PHASE.down, x, y));
});

canvas.addEventListener("pointermove", (e) => {
  if (!pointerDown) return;
  const { x, y } = normalized(e);
  send(touchFrame(PHASE.move, x, y));
});

canvas.addEventListener("pointerup", (e) => {
  if (!pointerDown) return;
  pointerDown = false;
  const { x, y } = normalized(e);
  send(touchFrame(PHASE.up, x, y));
});

// USB HID keyboard usages for the keys the console forwards.
canvas.addEventListener("keydown", (e) => {
  const frame = keyEventFrame(e);
  if (!frame) return;
  e.preventDefault();
  send(frame);
});

// ---------- toolbar ----------

$("btn-back").addEventListener("click", showLibrary);
$("btn-home").addEventListener("click", () => send(gestureFrame(GESTURE.home)));
$("btn-switcher").addEventListener("click", () => send(gestureFrame(GESTURE.appSwitcher)));
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
  // No app id needed: the tree engine targets whatever is on screen
  // (the runner resolves the frontmost app; Android trees are
  // whole-screen). The id input still narrows the tree when set.
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

// Back/forward moves between the library and device consoles.
window.addEventListener("popstate", route);

// route enters the view the URL names.
async function route() {
  const wanted = new URLSearchParams(location.search).get("device");
  if (!wanted) {
    showLibrary();
    return;
  }
  const devices = await refreshLibrary();
  const d = devices.find((x) => x.udid === wanted && x.booted);
  if (d) showConsole(d.udid, d.name, deviceShape(d));
  else showLibrary();
}
route();
