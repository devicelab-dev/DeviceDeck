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
  clearFaults();
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
// flowing, the input socket is open, and its engine (the tree reader behind
// Inspect, Record and launches) has started. A device showing its screen
// before then looked ready while taps went nowhere; if any of the three
// fails, the fault says so over the loading screen instead.
let videoReady = false;
let inputReady = false;
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
  clearFaults();
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
  inputReady = false;
  engineSettled = false;
  setEngineTools(false);
  if (inspecting) toggleInspector(); // a new device starts with Inspect off
}

// connectDevice starts everything for the device on screen: video, input,
// its apps, and its engine.
function connectDevice() {
  connectVideo();
  connectInput();
  loadApps();
  warmEngine(udid);
}

// ---------- faults ----------

// A fault is something the device page depends on that is down. Each one
// failed silently before — a refused input socket looked exactly like taps
// that did nothing — so the stage turns red and says which, in words. When
// several are down, the one listed first explains the rest.
const FAULT_ORDER = ["server", "input", "engine", "video", "session"];
const faults = new Map();

// setFault raises a fault ({title, detail, action?: {label, run}}), or
// clears it when fault is omitted.
function setFault(kind, fault) {
  if (fault) faults.set(kind, fault);
  else faults.delete(kind);
  const top = FAULT_ORDER.find((k) => faults.has(k));
  $("stage-fault").hidden = !top;
  if (!top) return;
  const f = faults.get(top);
  $("fault-title").textContent = f.title;
  $("fault-detail").textContent = f.detail;
  const button = $("fault-action");
  button.hidden = !f.action;
  if (f.action) {
    button.textContent = f.action.label;
    button.onclick = f.action.run;
  }
}

function clearFaults() {
  for (const kind of FAULT_ORDER) setFault(kind);
}

const SERVER_RETRY_MS = 2000;
let serverWatch = null;

// serverDown raises the server fault and polls until DeviceDeck answers
// again, then reconnects the device on screen from scratch: its sockets
// and engine belonged to the server process that went away.
function serverDown() {
  if (serverWatch) return;
  setFault("server", {
    title: "DeviceDeck is not reachable",
    detail: "The devicedeck server stopped or the network dropped. Start it again " +
      "(run devicedeck in a terminal); this page reconnects on its own.",
  });
  serverWatch = setInterval(async () => {
    const res = await fetch("/api/devices").catch(() => null);
    if (!res || !res.ok) return;
    clearInterval(serverWatch);
    serverWatch = null;
    setFault("server");
    reconnecting = true;
    if (udid) showConsole(udid);
  }, SERVER_RETRY_MS);
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
    if (udid !== device) return;
    if (!res) { serverDown(); return; }
    const st = res.ok ? await res.json() : { state: "failed", error: `HTTP ${res.status}` };
    if (st.state === "starting" || st.state === "") {
      const secs = st.startedAt ? Math.round((Date.now() - Date.parse(st.startedAt)) / 1000) : 0;
      showStage(`${st.detail || "Starting the device engine"}… ${secs}s`);
      engineTimer = setTimeout(() => poll("GET"), 700);
      return;
    }
    if (st.state === "failed") { engineFailed(device, st.error); return; }
    engineSettled = true;
    setEngineTools(true);
    revealWhenReady();
  };
  poll("POST");
}

// engineFailed keeps the loading screen up and says why: a device whose
// engine is down is not ready to use, so it is not shown as if it were.
function engineFailed(device, error) {
  setEngineTools(false);
  setFault("engine", {
    title: "The device engine did not start",
    detail: `${error}\n\nDeviceDeck reads the screen and runs Inspect, Record and launches through it.`,
    action: { label: "Retry", run: () => { setFault("engine"); warmEngine(device); } },
  });
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

// revealWhenReady drops the loading screen once video, input and engine
// are all up, and otherwise says which one it is still waiting on.
function revealWhenReady() {
  if (!engineSettled) return;
  if (!videoReady) { showStage("Connecting video…"); return; }
  if (!inputReady) { showStage("Connecting touch and keyboard…"); return; }
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
  const rejoining = reconnecting;
  reconnecting = false;
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
  const current = pendingLaunch || $("app").value.trim();
  if (apps.some((a) => a.id === current)) pick.value = current;
  else $("app").value = pick.value;
  $("apps-picker").hidden = false;
  updateUseLinks();
  const launch = pendingLaunch || (rejoining ? null : autoLaunch(apps, pick.value));
  pendingLaunch = null;
  if (launch && pick.value === launch) launchApp();
}

// pendingLaunch is a build the Apps section asked to launch; the device
// page launches it once it has listed the device's apps.
let pendingLaunch = null;
// reconnecting is set while the page rejoins a restarted server: the app
// on the device is whatever the person left there, and must stay so.
let reconnecting = false;

// autoLaunch picks the build to launch as a device opens: the one the App
// menu shows, when this server has launched none of them there yet. Only
// the tab in use does it (a reconnect never asks), because a launch starts
// the app fresh and would wipe whatever the person was doing.
function autoLaunch(apps, picked) {
  if (document.hidden || !document.hasFocus()) return null;
  return apps.some((a) => a.launched) ? null : picked;
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
  const running = devices.filter((d) => d.booted);
  $("library-summary").textContent = devices.length
    ? `${running.length} running · ${devices.length} on this Mac`
    : "";
  // Running first: those are the ones you came to use.
  const ordered = [...running, ...devices.filter((d) => !d.booted)];
  $("library-groups").replaceChildren(...(devices.length ? ordered.map(deviceCard) : [emptyNote(
    "No simulators or emulators found. Add simulators in Xcode (Settings → Platforms) " +
    "or create a virtual device in Android Studio, then reload.")]));
  renderApps(running);
  return devices;
}

function emptyNote(text) {
  return Object.assign(document.createElement("p"), { className: "library-empty", textContent: text });
}

// el builds an element with a class and, optionally, text.
function el(tag, className, text) {
  const node = document.createElement(tag);
  node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

// IOS_MODEL matches the model a simulator's name starts with ("iPhone 17
// Pro", "iPad Air 11-inch (M3)"); whatever follows is the name it was given.
const IOS_MODEL = /^(iPhone|iPad)(\s+(\d+(\.\d+)?(-inch)?|Pro|Max|Plus|mini|Air|SE|e|\([^)]*\)))*/;

// splitName separates a device's model from the label it was given, so a
// long name reads as two lines instead of breaking mid-word.
function splitName(name) {
  const model = (IOS_MODEL.exec(name) || [""])[0];
  const label = name.slice(model.length).trim();
  return model && label ? [model, label] : [name, ""];
}

// deviceCard is one device: a small silhouette lit when it runs, its model
// and label, its runtime and state, and its one action. The whole card is
// the action, so it can be picked from anywhere on it.
function deviceCard(d) {
  const android = d.os.startsWith("android");
  const verb = d.booted ? "Use" : "Boot";
  const card = el("button", "card device" + (d.booted ? " on" : ""));
  card.title = `${verb} ${d.name}\n${d.udid}`;
  card.setAttribute("aria-label", `${verb} ${d.name}${d.booted ? ", running" : ""}`);
  const glyph = el("span", "mini-phone" + (android ? " android" : ""));
  glyph.appendChild(platformBadge(android));
  const [model, label] = splitName(d.name);
  const text = el("span", "card-text");
  text.append(el("span", "card-title", model));
  if (label) text.append(el("span", "card-sub", label));
  const meta = el("span", "card-meta");
  meta.append(el("span", "chip", android ? "Android" : d.os), el("span", "state", d.booted ? "Running" : "Off"));
  text.append(meta);
  card.append(glyph, text, el("span", "card-action" + (d.booted ? " primary" : ""), verb));
  card.addEventListener("click", () => (d.booted ? showConsole(d.udid, d.name, deviceShape(d)) : bootDevice(d)));
  return card;
}

// ---------- apps ----------

// renderApps lists the builds passed with --app: what each is and what it
// needs, with a Launch on a running device of its platform. Builds a
// folder held that cannot run here get one quiet line each, with why.
async function renderApps(running) {
  const res = await fetch("/api/apps").catch(() => null);
  if (!res || !res.ok) return;
  const { apps, skipped } = await res.json();
  $("apps-summary").textContent = apps.length ? `${apps.length} ${apps.length === 1 ? "build" : "builds"}` : "";
  const grid = el("div", "cards");
  grid.append(...apps.map((a) => appCard(a, running)));
  const parts = apps.length ? [grid] : [emptyNote(
    "Start devicedeck with --app <file or folder> to list your builds here. " +
    "Each one is installed on a device the first time it is launched there.")];
  parts.push(...skipped.map(skippedNote));
  $("library-apps").replaceChildren(...parts);
}

// appCard is one build: its platform tile, name and version, bundle id,
// what it needs, and Launch. The rest of what the build is — its
// architectures, date and file — is on hover, where it does not crowd.
function appCard(a, running) {
  const android = a.platform === "Android";
  const card = el("div", "card app");
  card.title = [a.path, (a.arch || []).join(" "), a.modified && `built ${formatDate(a.modified)}`].filter(Boolean).join("\n");
  const tile = el("span", "app-tile" + (android ? " android" : ""));
  tile.appendChild(platformBadge(android));
  const text = el("span", "card-text");
  const head = el("span", "card-title");
  head.append(document.createTextNode(a.name));
  if (a.version) head.append(el("span", "version", a.version));
  text.append(head, el("span", "card-sub mono", a.id),
    el("span", "card-meta", [a.minOS && `${a.minOS}+`, formatSize(a.size)].filter(Boolean).join(" · ")));
  for (const w of a.warnings || []) text.append(el("span", "card-warning", w));
  const launch = launchButton(a, running);
  // Without a device to launch on, the card says so in its text and keeps
  // its full width for the name.
  if (launch.classList.contains("card-hint")) text.append(launch);
  else card.append(tile, text, launch);
  if (!card.childNodes.length) card.append(tile, text);
  return card;
}

// skippedNote is a build a folder held that cannot run here, and why.
function skippedNote(s) {
  const note = el("p", "skipped-note");
  note.title = s.path;
  note.append(el("strong", "", `Skipped ${s.path.split("/").slice(-2).join("/")}`),
    document.createTextNode(` — ${s.reason.replace(/^read [^:]+: /, "")}`));
  return note;
}

// launchButton launches a build on a running device of its platform, from
// its device page; with none running, it says what to boot instead.
function launchButton(a, running) {
  const android = a.platform === "Android";
  const target = running.find((d) => d.os.startsWith("android") === android);
  if (!target) return el("span", "card-hint", `Boot ${android ? "an emulator" : "a simulator"} to launch`);
  const button = el("button", "card-action primary", "Launch");
  button.title = `Launch on ${target.name}`;
  button.addEventListener("click", () => {
    pendingLaunch = a.id;
    showConsole(target.udid, target.name, deviceShape(target));
  });
  return button;
}

function formatSize(bytes) {
  if (!bytes) return "";
  return bytes >= 1e6 ? `${(bytes / 1e6).toFixed(1)} MB` : `${Math.max(1, Math.round(bytes / 1e3))} KB`;
}

function formatDate(iso) {
  return new Date(iso).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
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
  const socket = videoWS;
  videoWS.onclose = () => { if (videoWS === socket) videoLost(); };
}

// videoLost tells a stopped server from a stopped stream: the first is
// the server fault, the second leaves the server up and the screen dark.
async function videoLost() {
  const res = await fetch("/api/devices").catch(() => null);
  if (!res) { serverDown(); return; }
  setFault("video", {
    title: "The screen stream stopped",
    detail: "Video from this device ended. It may have shut down; the run's log folder says why.",
    action: { label: "Reconnect", run: () => { setFault("video"); connectVideo(); } },
  });
}

// ---------- live input ----------

// A device takes one driver at a time, and a second console tab on the
// same device was refused with nothing on screen to say so: its taps
// simply did nothing. Console tabs in one browser therefore hand the
// device over: the tab that opens it last tells the others, which let go.
const tabs = "BroadcastChannel" in window ? new BroadcastChannel("devicedeck-input") : null;
// HANDOFF_MS is how long a tab that just took a device over keeps
// retrying a refusal: the tab it took it from may not have let go yet.
const HANDOFF_MS = 3000;
const HANDOFF_RETRY_MS = 400;
let takenAt = 0;

// connectInput opens this page's input socket; takeOver disconnects
// whoever holds the device first (the fault card's Take over).
function connectInput(takeOver = false) {
  if (input) input.close();
  input = null;
  // A background tab never drives: see the visibilitychange handler.
  if (document.hidden) return;
  setFault("input");
  takenAt = Date.now();
  // Only the tab the person is using asks the others to let go. A tab that
  // reconnects on its own — after a server restart, in another window —
  // has no focus, so it never takes the device from the one in use.
  if (document.hasFocus()) tabs?.postMessage({ take: udid, focused: true });
  openInput(udid, takeOver === true);
}

function openInput(device, takeOver = false) {
  input = createInputSocket(wsURL(`/api/devices/${device}/input`), {
    takeOver,
    onOpen: () => { if (udid === device) { inputReady = true; revealWhenReady(); } },
    onRefused: (reason) => inputRefused(device, reason),
  });
}

const TAKEN_OVER = "taken over by ";
const SESSION_ENDED = "session ended";
const takeOverAction = { label: "Take over", run: () => connectInput(true) };

// inputRefused says out loud that taps from this page are not reaching the
// device, and who holds it, after a short grace for a tab handoff.
function inputRefused(device, reason = "") {
  if (udid !== device || endingSession === device) return;
  input = null;
  const kicked = kickedFault(reason);
  if (kicked) {
    setFault("input", kicked);
    return;
  }
  if (Date.now() - takenAt < HANDOFF_MS) {
    setTimeout(() => { if (udid === device) openInput(device); }, HANDOFF_RETRY_MS);
    return;
  }
  setFault("input", heldFault(reason));
}

// kickedFault is the fault for a page the server disconnected on purpose —
// its session ended, or another page took the device — or null. Coming
// back to such a page must not seize the device back: its button decides.
function kickedFault(reason) {
  if (reason.startsWith(SESSION_ENDED)) {
    return {
      kicked: true,
      title: "This device's session was ended",
      detail: "It was shut down from another page. Boot it again from the device list.",
      action: { label: "Device list", run: showLibrary },
    };
  }
  if (!reason.startsWith(TAKEN_OVER)) return null;
  return {
    kicked: true,
    title: "Another page took this device over",
    detail: `It is now driven from ${reason.slice(TAKEN_OVER.length)}; taps from this page no longer reach it.`,
    action: takeOverAction,
  };
}

// heldFault is the fault for a device someone else holds. The server's
// reason ends in advice for test suites; the holder is what a person here
// needs.
function heldFault(reason) {
  const holder = reason.split(";")[0].replace(/^device \S+ is already being driven by /, "");
  return {
    title: "Another client is driving this device",
    detail: `Taps and typing from this page are not reaching it. It is held by ${holder || "another client"}, ` +
      "most likely another tab left open.\n\nTake over disconnects it and gives the device to this page.",
    action: takeOverAction,
  };
}

// The tab the person is using drives its device. Console tabs left open
// reconnect the moment a restarted server comes back, before anyone has
// booted the device, and one of them held it while the tab in use was
// refused. So a tab going to the background lets go, and a tab coming to the
// front or getting focus takes the device (unless it was taken over from
// outside this browser, where the fault card's own button decides).
function reclaimInput() {
  if (udid && !input && !document.hidden && !faults.get("input")?.kicked) connectInput();
}
document.addEventListener("visibilitychange", () => {
  if (!udid) return;
  if (!document.hidden) { reclaimInput(); return; }
  if (input) input.close();
  input = null;
});
window.addEventListener("focus", reclaimInput);

// A take from a tab without focus is ignored: tabs from before this rule
// announce a take on every reconnect, from the background too.
tabs?.addEventListener("message", ({ data }) => {
  if (!data?.focused || data.take !== udid || !input) return;
  input.close();
  input = null;
  setFault("input", {
    title: "This device is open in another tab",
    detail: "Only one page drives a device at a time, and the newest tab took it over.",
    action: { label: "Use it here", run: connectInput },
  });
});

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
  // The right button opens the check menu; only the primary one drives.
  if (e.button !== 0) return;
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
  if (e.button !== 0) return;
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
$("btn-home").addEventListener("click", () => leaveApp(() => send(gestureFrame(GESTURE.home))));
$("btn-switcher").addEventListener("click", () => leaveApp(() => send(gestureFrame(GESTURE.appSwitcher))));
$("btn-lock").addEventListener("click", () => leaveApp(() =>
  fetch(`/api/devices/${udid}/button`, { method: "POST", body: JSON.stringify({ button: "lock" }) })));

// LEAVE_SETTLE_MS is how long a snapshot already running on the device is
// given to finish before a Home, Switcher or Lock is sent.
const LEAVE_SETTLE_MS = 400;

// leaveApp sends an action that takes the app off screen. While Inspect
// follows the screen a snapshot is nearly always running, and on iOS one
// caught mid-way by the app leaving the screen stalls the runner for over
// a minute. So the follow loop is stopped first, the running snapshot is
// given a moment to finish, the action is sent, and following resumes.
async function leaveApp(action) {
  if (!inspecting) {
    action();
    return;
  }
  treeLoop++;
  if (treeAbort) treeAbort.abort();
  await sleep(LEAVE_SETTLE_MS);
  await action();
  await sleep(700);
  if (inspecting) followTree();
}
$("btn-end").addEventListener("click", endSession);

// endingSession is the device this page is shutting down itself. Ending a
// session tells the device's driver it ended, and closes its video; here
// the driver is this page, so neither is news to report.
let endingSession = null;

// letGo drops this page's own hold on a device it is about to shut down —
// input, video, Inspect and the engine poll — and puts the loading frame up
// saying so, in place of a live screen about to go dark.
function letGo(device) {
  endingSession = device;
  if (input) input.close();
  input = null;
  const socket = videoWS;
  videoWS = null; // before close, so its onclose is not taken for a lost stream
  socket?.close();
  if (inspecting) toggleInspector();
  clearTimeout(engineTimer);
  clearFaults();
  setEngineTools(false);
  $("stage-loading").hidden = false;
  canvas.classList.add("connecting");
  $("loading-text").textContent = `Shutting down ${$("device-label").textContent || device}…`;
}

// endSession stops everything DeviceDeck runs for the device, powers the
// device off and returns to the device list. A recording in progress would
// be discarded, so that is confirmed first.
async function endSession() {
  const device = udid;
  if (!device) return;
  if (recording && !confirm("A recording is in progress and will be discarded. End the session?")) return;
  const button = $("btn-end");
  button.disabled = true;
  button.textContent = "Shutting down…";
  letGo(device);
  const res = await fetch(`/api/devices/${encodeURIComponent(device)}/shutdown`, { method: "POST" }).catch(() => null);
  endingSession = null;
  button.disabled = false;
  button.textContent = "End session";
  if (udid !== device) return;
  if (!res) { serverDown(); return; }
  if (res.ok) { showLibrary(); return; }
  const body = await res.json().catch(() => ({}));
  setFault("session", {
    title: "The session did not end",
    detail: body.error || `HTTP ${res.status}`,
    action: { label: "Try again", run: () => { setFault("session"); endSession(); } },
  });
}

$("btn-shot").addEventListener("click", () => window.open(`/api/devices/${udid}/screenshot`));
$("btn-inspect").addEventListener("click", toggleInspector);

// ---------- flow capture ----------

let recording = false;
let capturePoll = null;

$("btn-record").addEventListener("click", toggleRecord);
$("btn-launch").addEventListener("click", launchApp);
$("app-pick").addEventListener("change", () => { $("app").value = $("app-pick").value; updateUseLinks(); });
$("app").addEventListener("input", updateUseLinks);
for (const b of document.querySelectorAll("button.copy")) b.addEventListener("click", () => copyField(b));

// CHECK_RECORDED is what the status line says once a check is recorded.
const CHECK_RECORDED = { visible: "assertion recorded", wait: "wait recorded" };

// recordCheck resolves a point to an element server-side, where the tree
// already lives, and appends a check on it to the recording: "visible"
// asserts it is there (assertVisible), "wait" waits for it to appear
// (extendedWaitUntil). The device is never touched.
async function recordCheck(check, { x, y }) {
  const res = await fetch(`/api/devices/${udid}/capture/assert`, {
    method: "POST",
    body: JSON.stringify({ x, y, check }),
  }).catch(() => null);
  if (!res) { serverDown(); return; }
  const body = await res.json().catch(() => ({}));
  status.textContent = res.ok ? CHECK_RECORDED[check] : `${check}: ${body.error || res.status}`;
}

// ---------- check menu ----------

// While recording, a right-click on the device opens a menu of the checks
// that can be recorded on the element under the pointer. It is the direct
// way to say "assert this" or "wait for this": no arming a mode first, and
// nothing reaches the device.
const checkMenu = $("check-menu");
let checkPoint = null;

$("stage").addEventListener("contextmenu", (e) => {
  if (!recording || !overCanvas(e)) return; // the browser's own menu otherwise
  e.preventDefault();
  checkPoint = normalized(e);
  const stage = $("stage").getBoundingClientRect();
  checkMenu.hidden = false;
  const left = Math.min(e.clientX - stage.left, stage.width - checkMenu.offsetWidth - 8);
  const top = Math.min(e.clientY - stage.top, stage.height - checkMenu.offsetHeight - 8);
  checkMenu.style.left = `${Math.max(8, left)}px`;
  checkMenu.style.top = `${Math.max(8, top)}px`;
  checkMenu.querySelector("button").focus();
});

checkMenu.addEventListener("click", (e) => {
  const check = e.target.closest("button")?.dataset.check;
  if (!check) return;
  closeCheckMenu();
  recordCheck(check, checkPoint);
});

function closeCheckMenu() {
  checkMenu.hidden = true;
}

document.addEventListener("pointerdown", (e) => { if (!checkMenu.contains(e.target)) closeCheckMenu(); }, true);
document.addEventListener("keydown", (e) => { if (e.key === "Escape") closeCheckMenu(); });
window.addEventListener("blur", closeCheckMenu);

// overCanvas reports whether a pointer event is on the device screen.
function overCanvas(e) {
  const r = canvas.getBoundingClientRect();
  return e.clientX >= r.left && e.clientX <= r.right && e.clientY >= r.top && e.clientY <= r.bottom;
}

function toggleRecord() {
  return recording ? stopRecording() : startRecording();
}

// startRecording begins Flow Capture for the app named in the sidebar.
async function startRecording() {
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
  capturePoll = setInterval(pollCapture, 700);
  status.textContent = "recording — right-click an element to assert or wait for it";
}

// stopRecording ends Flow Capture and shows the captured flow.
async function stopRecording() {
  clearInterval(capturePoll);
  const res = await fetch(`/api/devices/${udid}/capture/stop`, { method: "POST", body: "{}" });
  const body = await res.json();
  recording = false;
  closeCheckMenu();
  $("btn-record").classList.remove("recording");
  setTool($("btn-record"), RECORD_ICON, "Record");
  if (!res.ok) {
    status.textContent = `stop: ${body.error}`;
    return;
  }
  showFlow(body.yaml, body.steps);
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

// setFlowPanel widens the panel for a captured flow and narrows it back for
// the inspector. The device shrinks or grows with it, and the Inspect
// overlay follows only window resizes, so it is realigned here.
function setFlowPanel(on) {
  if (!on) $("flow-actions").hidden = true;
  if ($("panel").classList.contains("flow") === on) return;
  $("panel").classList.toggle("flow", on);
  requestAnimationFrame(positionOverlay);
}

// capturedFlow is the YAML on show, for Copy.
let capturedFlow = "";

$("flow-copy").addEventListener("click", async () => {
  const button = $("flow-copy");
  const done = await navigator.clipboard.writeText(capturedFlow).then(() => true, () => false);
  button.textContent = done ? "Copied" : "Copy failed";
  setTimeout(() => { button.textContent = "Copy"; }, 1200);
});

// flowFileName suggests a name for a saved flow: the app it drives and
// when it was captured, so saved flows sort and never overwrite each other.
function flowFileName() {
  const app = ($("app").value.trim().split(".").pop() || "flow").replace(/[^\w-]/g, "");
  const at = new Date();
  const pad = (n) => String(n).padStart(2, "0");
  return `${app}-${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}-${pad(at.getHours())}${pad(at.getMinutes())}.yaml`;
}

// Download asks where to save and under what name, where the browser can
// (Chrome and Edge: the system Save dialog). Elsewhere the link downloads
// with the suggested name, as any download does.
$("flow-download").addEventListener("click", async (e) => {
  if (!window.showSaveFilePicker) return;
  e.preventDefault();
  try {
    const file = await window.showSaveFilePicker({
      suggestedName: $("flow-download").download,
      types: [{ description: "Maestro flow", accept: { "text/yaml": [".yaml", ".yml"] } }],
    });
    const out = await file.createWritable();
    await out.write(capturedFlow);
    await out.close();
    status.textContent = `saved ${file.name}`;
  } catch (err) {
    if (err.name !== "AbortError") status.textContent = `save: ${err.message}`;
  }
});

function showFlow(yaml, steps) {
  $("panel").hidden = false;
  setFlowPanel(true);
  $("node-title").textContent = `Captured flow — ${steps.length} steps`;
  $("node-info").classList.remove("hint");
  $("node-info").replaceChildren(highlightFlow(yaml));
  capturedFlow = yaml;
  URL.revokeObjectURL($("flow-download").href);
  $("flow-download").href = URL.createObjectURL(new Blob([yaml], { type: "text/yaml" }));
  $("flow-download").download = flowFileName();
  $("flow-actions").hidden = false;
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
  // whole-screen). Inspect never sends the app id: that would bring the app
  // back to the front on every snapshot (see followTree).
  inspecting = !inspecting;
  $("btn-inspect").classList.toggle("active", inspecting);
  overlay.hidden = !inspecting;
  if (inspecting) {
    if ($("node-info").classList.contains("hint")) {
      $("node-info").textContent = "Hover an element on the device to see its id, role and text here.";
    }
    followTree();
  } else {
    treeLoop++; // stops the loop
    overlay.innerHTML = "";
    panelIdle();
  }
}

// panelIdle is what the right panel says when nothing is being inspected.
// On a wide screen the panel stays open, so it explains how to fill it.
function panelIdle() {
  setFlowPanel(false);
  $("node-title").textContent = "Inspector";
  $("node-info").classList.add("hint");
  $("node-info").textContent =
    "Press Inspect, then hover an element on the device to see its id, role and text: " +
    "the selectors a test or Claude uses.\n\nPress Record to capture what you do as a " +
    "Maestro flow; the steps appear here when you stop.";
}
panelIdle();

// treeLoop numbers the running follow loop; bumping it stops the loop.
let treeLoop = 0;
// treeAbort cancels the follow loop's waiting request.
let treeAbort = null;

// IDLE_PAUSE_MS is the breather after a round in which the screen did not
// change, so a still screen is not sampled back to back.
const IDLE_PAUSE_MS = 400;

// followTree keeps the Inspect overlay on the current screen. Each answer
// carries the screen's interaction hash, and the next request hands it
// back as ?after=, which the server holds until the screen has changed and
// come to rest (or two seconds pass). So the overlay follows taps on the
// video, typing, and the app changing by itself, not only overlay clicks.
async function followTree() {
  const gen = ++treeLoop;
  const device = udid;
  let after = null;
  status.textContent = "fetching tree…";
  while (inspecting && gen === treeLoop && udid === device) {
    // No app id: naming one makes the iOS runner bring that app to the
    // front on every snapshot, which undid Home and the app switcher while
    // Inspect was on. Without it the tree is whatever is on screen.
    const params = new URLSearchParams();
    if (after !== null) params.set("after", after);
    treeAbort = new AbortController();
    const res = await fetch(`/api/devices/${encodeURIComponent(device)}/tree?${params}`, { signal: treeAbort.signal })
      .catch(() => null);
    if (!inspecting || gen !== treeLoop || udid !== device) return;
    if (!res) serverDown();
    const body = res ? await res.json().catch(() => ({})) : {};
    if (!res || !res.ok) {
      if (res) treeFailed(body.error || `HTTP ${res.status}`);
      await sleep(1500);
      continue;
    }
    setFault("engine");
    const changed = body.interaction !== after;
    if (changed) {
      renderOverlay(body.nodes);
      status.textContent = `tree: ${body.nodes.length} nodes`;
    }
    after = body.interaction;
    if (!changed) await sleep(IDLE_PAUSE_MS);
  }
}

// treeFailed raises the engine fault while Inspect cannot read the screen;
// the follow loop keeps retrying and clears it on the next good answer.
function treeFailed(error) {
  setFault("engine", {
    title: "The device engine is not answering",
    detail: `Inspect cannot read the screen: ${error}`,
    action: { label: "Restart it", run: () => { setFault("engine"); warmEngine(udid); } },
  });
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
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
  setFlowPanel(false);
  $("node-title").textContent = node.identifier || node.label || node.type;
  $("node-info").classList.remove("hint");
  $("node-info").textContent = JSON.stringify(node, null, 2);
}

// nodeCenter is an element's centre as a normalized point on the screen.
function nodeCenter(node, app) {
  return {
    x: (node.frame.x + node.frame.width / 2) / app.width,
    y: (node.frame.y + node.frame.height / 2) / app.height,
  };
}

async function tapNode(node, app) {
  // The click landed on the overlay, not the video, so give the video its
  // keyboard focus back: typing after an Inspect tap must reach the device.
  canvas.focus({ preventScroll: true });
  const { x, y } = nodeCenter(node, app);
  await fetch(`/api/devices/${udid}/tap`, {
    method: "POST",
    body: JSON.stringify({ x, y }),
  });
  // No refresh here: the follow loop picks up the change once it settles.
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
