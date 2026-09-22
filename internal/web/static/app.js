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
// knownDevices is the last device listing, so the console view can show a
// device's name and OS without asking again.
let knownDevices = [];
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
  $("apps-picker").hidden = true;
  clearTimeout(engineTimer);
  $("stage-loading").hidden = true;
  canvas.classList.remove("connecting");
  status.textContent = "";
  if (new URLSearchParams(location.search).get("device")) {
    history.pushState({}, "", location.pathname);
  }
  refreshLibrary();
}

let awaitingFirstFrame = false;
// The loading screen stays until the device is fully usable: video is
// flowing and its engine (the tree reader behind Inspect, Record and
// launches) has started, or failed, in which case video and touch still work.
let videoReady = false;
let engineSettled = false;
let engineTimer = null;
let loadingShownAt = 0;

// The connecting state stays up at least this long even when the first
// frame is instant — a flash of loading reads as a glitch, a beat of it
// reads as arrival.
const MIN_LOADING_MS = 700;

function showConsole(next, name, shape) {
  openDeviceView(next, name, shape);
  connectDevice();
}

// openDeviceView switches to a device's page at once and puts up its
// loading screen; connecting to the device is a separate step, so a device
// that is still booting can show that here rather than on the library card.
function openDeviceView(next, name, shape) {
  udid = next;
  $("library").hidden = true;
  $("console-view").hidden = false;
  $("console-controls").hidden = false;
  showDeviceInfo(next, name);
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
  videoReady = false;
  engineSettled = false;
  setEngineTools(false);
}

// connectDevice starts everything for the device on screen: video, input,
// its apps, and its engine.
function connectDevice() {
  connectVideo();
  connectInput();
  loadApps();
  warmEngine(udid);
}

// ---------- engine warm-up ----------

// warmEngine starts the device's engine as the view opens and reports each
// stage on the loading screen, with elapsed seconds, so a first-time runner
// build reads as progress rather than a hang.
async function warmEngine(device) {
  clearTimeout(engineTimer);
  const poll = async (method) => {
    if (udid !== device) return;
    const res = await fetch(`/api/devices/${encodeURIComponent(device)}/engine`, { method }).catch(() => null);
    const st = res && res.ok ? await res.json() : { state: "failed", error: "server unreachable" };
    if (udid !== device) return;
    if (st.state === "starting" || st.state === "") {
      const secs = st.startedAt ? Math.round((Date.now() - Date.parse(st.startedAt)) / 1000) : 0;
      showStage(`${st.detail || "Starting the device engine"}… ${secs}s`);
      engineTimer = setTimeout(() => poll("GET"), 700);
      return;
    }
    engineSettled = true;
    setEngineTools(st.state === "ready");
    if (st.state === "failed") status.textContent = `engine failed: ${st.error}. Video and touch still work; Inspect and Record need the engine.`;
    revealWhenReady();
  };
  poll("POST");
}

// showStage puts a line on the loading screen while it is up.
function showStage(text) {
  if (!$("stage-loading").hidden) $("loading-text").textContent = text;
}

// setEngineTools enables the controls that read the UI tree.
function setEngineTools(ready) {
  for (const id of ["btn-inspect", "btn-record"]) {
    $(id).disabled = !ready;
    $(id).title = ready ? $(id).dataset.title || $(id).title : "Waiting for the device engine to start";
  }
}

// revealWhenReady drops the loading screen once video and engine are both
// settled, and otherwise says which one it is still waiting on.
function revealWhenReady() {
  if (!videoReady) { if (engineSettled) showStage("Connecting video…"); return; }
  if (!engineSettled) return;
  $("stage-loading").hidden = true;
  canvas.classList.remove("connecting");
  // Still-based streams (Android static screens) never configure a
  // decoder, so nothing else clears the connecting status.
  if (status.textContent === "connecting video…") status.textContent = "live";
}

// ---------- controls ----------

for (const id of ["btn-inspect", "btn-record"]) $(id).dataset.title = $(id).title;

// The Record tool's two faces; the button shows a dot to start and a
// square to stop, as a recorder does.
const RECORD_ICON = '<circle cx="12" cy="12" r="6" class="fill"/>';
const STOP_ICON = '<rect x="7" y="7" width="10" height="10" rx="1.5" class="fill"/>';

// setTool swaps a tool button's icon and label together.
function setTool(button, icon, label) {
  button.querySelector("svg").innerHTML = icon;
  button.querySelector("span").textContent = label;
}

// ---------- sidebar ----------

// showDeviceInfo fills the sidebar for the device on screen: its name, OS
// and id, and the links a test or Claude uses to reach it.
function showDeviceInfo(id, name) {
  const d = knownDevices.find((x) => x.udid === id);
  $("device-label").textContent = (d && d.name) || name || id;
  const android = d ? d.os.startsWith("android") : false;
  $("device-meta").textContent = d ? (android ? "Android emulator" : `${d.os} simulator`) : "";
  $("device-id").value = id;
  updateUseLinks();
}

// updateUseLinks keeps the "Use it" fields on the device page for the app
// in the app field: the URL a test opens, and a prompt for Claude Code
// with Playwright MCP. The origin is whatever this console was opened on,
// so a teammate on the network gets an address that works for them.
function updateUseLinks() {
  if (!udid) return;
  const app = $("app").value.trim();
  const page = `${location.origin}/device/${encodeURIComponent(udid)}` +
    (app ? `?app=${encodeURIComponent(app)}` : "");
  $("device-url").value = page;
  $("claude-ask").value = `Open ${page} with Playwright and tell me what is on screen`;
}

// copyField copies a sidebar field. The clipboard API needs a secure
// context, which a console opened over plain http on the network is not,
// so the fallback selects the text and uses the older copy command.
async function copyField(button) {
  const field = $(button.dataset.copy);
  try {
    await navigator.clipboard.writeText(field.value);
  } catch {
    field.select();
    document.execCommand("copy");
  }
  button.textContent = "Copied";
  setTimeout(() => { button.textContent = "Copy"; }, 1200);
}

// ---------- registered apps (--app) ----------

// loadApps fills the Apps menu with the builds that suit this device, each
// marked when it will install on first launch. The menu stays hidden when
// devicedeck was started without --app. Picking one also fills the app
// field, so Record and Inspect work on it straight away.
async function loadApps() {
  const forDevice = udid;
  $("apps-picker").hidden = true;
  const res = await fetch(`/api/devices/${encodeURIComponent(forDevice)}/apps`).catch(() => null);
  if (!res || !res.ok || udid !== forDevice) return;
  const { apps } = await res.json();
  if (!apps.length) return;
  const pick = $("app-pick");
  pick.replaceChildren(...apps.map((a) => {
    const opt = document.createElement("option");
    opt.value = a.id;
    opt.textContent = `${a.name}${a.version ? ` ${a.version}` : ""}${a.installed ? "" : " · installs on launch"}`;
    return opt;
  }));
  const current = $("app").value.trim();
  if (apps.some((a) => a.id === current)) pick.value = current;
  else $("app").value = pick.value;
  $("apps-picker").hidden = false;
  updateUseLinks();
}

// launchApp starts the picked app at its first screen; the server installs
// the build first when this device does not have it.
async function launchApp() {
  const pick = $("app-pick");
  const id = pick.value;
  const label = pick.selectedOptions[0] ? pick.selectedOptions[0].textContent.split(" · ")[0] : id;
  $("app").value = id;
  updateUseLinks();
  $("btn-launch").disabled = true;
  status.textContent = `launching ${label}…`;
  const res = await fetch(`/api/devices/${encodeURIComponent(udid)}/app/launch`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ app: id }),
  }).catch((err) => ({ ok: false, json: async () => ({ error: err.message }) }));
  const body = await res.json().catch(() => ({}));
  $("btn-launch").disabled = false;
  status.textContent = res.ok ? `${label} launched` : `launch: ${body.error || res.status}`;
  if (res.ok) loadApps(); // it is installed now
}

async function refreshLibrary() {
  const { devices } = await (await fetch("/api/devices")).json();
  knownDevices = devices;
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
    action.addEventListener("click", () => bootDevice(d));
  }
  card.appendChild(action);
  return card;
}

// bootDevice starts a stopped device and polls until it comes up, then
// enters its console. AVDs come back under a fresh adb serial, so the
// poll watches for any newly running device rather than the id.
// bootDevice opens the device's page straight away and shows the boot
// there, counting seconds, then connects once the device is up. A stopped
// emulator's id (avd:<name>) changes to an adb serial when it boots, so it
// is recognised by its AVD name, never as "whatever booted next", which
// could be a device someone else started at the same moment.
async function bootDevice(d) {
  const booting = d.udid;
  const t0 = Date.now();
  openDeviceView(d.udid, d.name, deviceShape(d));
  const tick = () => showStage(`Booting ${d.name}… ${Math.round((Date.now() - t0) / 1000)}s`);
  tick();
  const res = await fetch(`/api/devices/${encodeURIComponent(d.udid)}/boot`, { method: "POST", body: "{}" });
  if (!res.ok) {
    bootFailed(`Boot failed: ${(await res.json().catch(() => ({}))).error || res.status}`);
    return;
  }
  const poll = async () => {
    if (udid !== booting) return; // navigated away
    tick();
    const { devices } = await (await fetch("/api/devices")).json();
    knownDevices = devices;
    const up = devices.find((x) => x.booted && (x.udid === d.udid || (d.avd && x.avd === d.avd)));
    if (up) {
      udid = up.udid;
      history.replaceState({}, "", `${location.pathname}?device=${encodeURIComponent(up.udid)}`);
      showDeviceInfo(up.udid, up.name);
      connectDevice();
      return;
    }
    if (Date.now() - t0 > 180_000) bootFailed("Boot timed out; check the device and try again");
    else setTimeout(poll, 1500);
  };
  setTimeout(poll, 1500);
}

// bootFailed says so on the device page and in the status line.
function bootFailed(message) {
  showStage(message);
  status.textContent = message;
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
      const remaining = MIN_LOADING_MS - (Date.now() - loadingShownAt);
      setTimeout(() => { if (udid === device) { videoReady = true; revealWhenReady(); } }, Math.max(0, remaining));
    }
    renderer.handleMessage(bytes);
  };
  videoWS.onclose = () => { status.textContent = "video disconnected"; };
}

// ---------- live input ----------

function connectInput() {
  if (input) input.close();
  input = createInputSocket(wsURL(`/api/devices/${udid}/input`), {
    onRefused: (reason) => {
      status.textContent = reason || "another client is driving this device";
    },
  });
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
  // While an assertion is armed the tap must not reach the device: the
  // recording would then alter the very screen it is asserting on.
  if (asserting) return;
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
  if (asserting) {
    recordAssertion(normalized(e));
    return;
  }
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
// asserting arms the next tap to record an assertion rather than drive
// the device. One assertion per arming, so a mis-armed click cannot
// silently swallow a whole session's taps.
let asserting = false;

$("btn-record").addEventListener("click", toggleRecord);
$("btn-launch").addEventListener("click", launchApp);
$("app-pick").addEventListener("change", () => { $("app").value = $("app-pick").value; updateUseLinks(); });
$("app").addEventListener("input", updateUseLinks);
for (const b of document.querySelectorAll("button.copy")) b.addEventListener("click", () => copyField(b));
$("btn-assert").addEventListener("click", () => setAsserting(!asserting));

function setAsserting(on) {
  asserting = on;
  $("btn-assert").classList.toggle("armed", on);
  canvas.classList.toggle("asserting", on);
  if (on) status.textContent = "tap the element to assert is visible";
}

// recordAssertion resolves the tapped point server-side, where the tree
// already lives, and appends an assertVisible step to the recording.
async function recordAssertion({ x, y }) {
  setAsserting(false);
  const res = await fetch(`/api/devices/${udid}/capture/assert`, {
    method: "POST",
    body: JSON.stringify({ x, y }),
  });
  const body = await res.json();
  status.textContent = res.ok
    ? "assertion recorded"
    : `assert: ${body.error}`;
}

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
    setTool($("btn-record"), STOP_ICON, "Stop");
    $("btn-assert").hidden = false;
    capturePoll = setInterval(pollCapture, 700);
    status.textContent = "recording — drive the device";
  } else {
    clearInterval(capturePoll);
    const res = await fetch(`/api/devices/${udid}/capture/stop`, { method: "POST", body: "{}" });
    const body = await res.json();
    recording = false;
    setAsserting(false);
    $("btn-assert").hidden = true;
    $("btn-record").classList.remove("recording");
    setTool($("btn-record"), RECORD_ICON, "Record");
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
// stepGrade colours a recorded step by how durable its selector is,
// mirroring the export's provenance grades (see export.go): an identifier
// is green, a text match yellow, a relational qualifier orange, and a
// positional index or a raw coordinate red. A reviewer sees the weak steps
// live while capturing, not only later in the flow's comments.
function stepGrade(step) {
  const selText = step.id ? `id: ${step.id}` : step.text ? `text: ${step.text}` : "point";
  let cls = "grade-point";
  let grade = "point";
  if (step.id || step.text) {
    if (step.index) {
      cls = "grade-index";
      grade = "index";
    } else if (step.childOfId) {
      cls = "grade-relational";
      grade = "relational";
    } else if (step.id) {
      cls = "grade-id";
      grade = "id";
    } else {
      cls = "grade-text";
      grade = "text";
    }
  }
  return { cls, selText, grade };
}

function flashResolved(step) {
  positionOverlay();
  overlay.hidden = false;
  const el = document.createElement("div");
  const g = stepGrade(step);
  el.className = `resolved-flash ${g.cls}`;
  el.style.left = `${step.bounds.x * 100}%`;
  el.style.top = `${step.bounds.y * 100}%`;
  el.style.width = `${step.bounds.width * 100}%`;
  el.style.height = `${step.bounds.height * 100}%`;
  const tag = document.createElement("span");
  tag.textContent = `${g.selText} · ${g.grade}`;
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
  $("node-info").classList.remove("hint");
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
  // The flow runs unchanged on real devices: say where.
  if (!$("panel").querySelector("a.realdevices")) {
    const cta = document.createElement("a");
    cta.className = "realdevices";
    cta.href = "https://devicelab.dev";
    cta.target = "_blank";
    cta.rel = "noopener";
    cta.textContent = "Run this flow unchanged on real devices at devicelab.dev →";
    $("panel").appendChild(cta);
  }
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
  overlay.hidden = !inspecting;
  if (inspecting) {
    if ($("node-info").classList.contains("hint")) {
      $("node-info").textContent = "Hover an element on the device to see its id, role and text here.";
    }
    await refreshTree();
  } else {
    overlay.innerHTML = "";
    panelIdle();
  }
}

// panelIdle is what the right panel says when nothing is being inspected.
// On a wide screen the panel stays open, so it explains how to fill it.
function panelIdle() {
  $("node-title").textContent = "Inspector";
  $("node-info").classList.add("hint");
  $("node-info").textContent =
    "Press Inspect, then hover an element on the device to see its id, role and text: " +
    "the selectors a test or Claude uses.\n\nPress Record to capture what you do as a " +
    "Maestro flow; the steps appear here when you stop.";
}
panelIdle();

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
    // Wholly off-screen: a recycled list cell parked below the fold, or
    // a scrollable row's overflow. It is in the tree and cannot be
    // pointed at, so drawing it only adds boxes with nothing under them.
    if (f.x >= app.width || f.y >= app.height || f.x + f.width <= 0 || f.y + f.height <= 0) continue;
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
  $("node-info").classList.remove("hint");
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
