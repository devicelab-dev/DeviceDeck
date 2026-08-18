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

// The reconnecting input socket (input-common.js); created by start().
let input = null;

// ---------- video ----------

function wsURL(path) {
  const proto = location.protocol === "https:" ? "wss" : "ws";
  return `${proto}://${location.host}${path}`;
}

// Frame handling shared with the console (video-common.js).
const renderer = createScreenRenderer(canvas, ctx, { onResize: positionMirror });

function connectVideo() {
  const ws = new WebSocket(wsURL(`/api/devices/${udid}/video`));
  ws.binaryType = "arraybuffer";
  ws.onmessage = ({ data }) => renderer.handleMessage(new Uint8Array(data));
  ws.onclose = () => setTimeout(connectVideo, 1500);
}

function connectInput() {
  input = createInputSocket(wsURL(`/api/devices/${udid}/input`));
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

// Identity key for reconciliation: the identifier when present, else
// type + placeholder (stable while a field's value changes), else the
// label; same-key siblings are disambiguated by occurrence order.
function nodeKey(node, counts) {
  const base =
    node.identifier || `${node.type}|${node.placeholder || node.label || ""}`;
  const n = counts.get(base) || 0;
  counts.set(base, n + 1);
  return `${base}#${n}`;
}

function setOrRemove(el, attr, value) {
  if (value) el.setAttribute(attr, value);
  else el.removeAttribute(attr);
}

// syncNode positions el inside its rendered ancestor: node frames are
// screen-absolute, so coordinates convert to percentages of the
// ancestor's frame and stay proportional at any canvas size.
function syncNode(el, node, anchorFrame) {
  const f = node.frame;
  el.style.left = `${((f.x - anchorFrame.x) / anchorFrame.width) * 100}%`;
  el.style.top = `${((f.y - anchorFrame.y) / anchorFrame.height) * 100}%`;
  el.style.width = `${(f.width / anchorFrame.width) * 100}%`;
  el.style.height = `${(f.height / anchorFrame.height) * 100}%`;

  setOrRemove(el, "role", ROLES[node.type] || "");
  setOrRemove(el, "data-testid", node.identifier || "");
  // Accessible name: label first, placeholder as fallback so unnamed
  // fields still read as `textbox "Username"` in aria snapshots.
  setOrRemove(el, "aria-label", node.label || node.placeholder || "");
  setOrRemove(el, "aria-placeholder", node.placeholder || "");
  setOrRemove(el, "aria-disabled", node.enabled ? "" : "true");
  setOrRemove(el, "aria-selected", node.selected ? "true" : "");
  // Text for getByText: label, else value, painted transparent. Kept in
  // a dedicated leading text node — assigning textContent would destroy
  // the nested child elements.
  const text = node.label || node.value || "";
  const textNode =
    el.firstChild && el.firstChild.nodeType === Node.TEXT_NODE ? el.firstChild : null;
  if (!text) {
    if (textNode) textNode.remove();
  } else if (textNode) {
    if (textNode.data !== text) textNode.data = text;
  } else {
    el.insertBefore(document.createTextNode(text), el.firstChild);
  }
}

// A node earns a mirror element only if something can find it: an
// identifier, visible text, or a mapped role. Zero-size nodes never do.
function mirrorable(node) {
  if (node.depth === 0) return false;
  if (!(node.frame.width > 0) || !(node.frame.height > 0)) return false;
  return !!(node.identifier || node.label || node.value || ROLES[node.type]);
}

// Reuse the keyed element from a previous render when one exists.
function acquireEl(existing, key) {
  const found = existing.get(key);
  if (found) {
    existing.delete(key);
    return found;
  }
  const el = document.createElement("div");
  el.setAttribute("data-dd-node", "");
  el.setAttribute("data-dd-key", key);
  return el;
}

function renderMirror(nodes) {
  const json = JSON.stringify(nodes);
  if (json === lastTreeJSON) return;
  lastTreeJSON = json;

  positionMirror();
  const app = nodes.length ? nodes[0].frame : null;
  if (!app || !(app.width > 0) || !(app.height > 0)) {
    mirror.innerHTML = "";
    return;
  }

  // Reconcile instead of rebuilding: an element that survives a refresh
  // keeps its DOM identity, so automation handles held across refreshes
  // (Playwright aria-refs, Selenium elements) stay valid between actions.
  const existing = new Map();
  for (const el of mirror.querySelectorAll("[data-dd-key]")) {
    existing.set(el.getAttribute("data-dd-key"), el);
  }
  const counts = new Map();
  // The mirror nests like the native tree: a node's element is appended
  // under its nearest *rendered* ancestor. Nesting is what makes a text
  // child a legitimate hit target for clicks aimed at its container
  // (Playwright's actionability check), and native paint order becomes
  // plain DOM order — no z-index arithmetic. anchors[i] carries the
  // container element + frame that node i's children position against.
  const anchors = new Map();
  const rootAnchor = { el: mirror, frame: app };
  for (const node of nodes) {
    const parentAnchor =
      (node.parentIndex != null && anchors.get(node.parentIndex)) || rootAnchor;
    if (!mirrorable(node)) {
      anchors.set(node.index, parentAnchor); // children inherit the anchor
      continue;
    }
    const el = acquireEl(existing, nodeKey(node, counts));
    syncNode(el, node, parentAnchor.frame);
    // Append-or-move keeps DOM order tracking native order; moving
    // (including across parents) preserves element identity.
    parentAnchor.el.appendChild(el);
    anchors.set(node.index, { el, frame: node.frame });
  }
  for (const el of existing.values()) el.remove();
}

// ---------- tree sync: adaptive polling + refresh-after-action ----------

// Fast cadence while the screen is likely changing (recent action or
// recent tree change); idle cadence otherwise. Every input event also
// schedules a near-immediate fetch: keyboards and animations move the
// layout within a few hundred ms of an action, and waiting out a full
// poll tick left a window where the next click aimed at stale bounds —
// the §11.8 refresh-before-act failure mode. Sequenced so a stale fetch
// never overwrites a newer one.
const ACTIVE_MS = 60;
const FAST_MS = 300;
const IDLE_MS = 1000;
let lastActivity = 0;
let syncSeq = 0;
let syncTimer = null;
let fetchInFlight = false;

async function syncTree() {
  // One fetch at a time: tree dumps serialize on the device side, and a
  // pile-up would queue input calls behind them for seconds.
  if (fetchInFlight) {
    scheduleSync(FAST_MS);
    return;
  }
  fetchInFlight = true;
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
  fetchInFlight = false;
  scheduleSync(Date.now() - lastActivity < 5000 ? FAST_MS : IDLE_MS);
}

// scheduleSync (re)arms the single sync timer; a shorter pending delay
// is never lengthened.
function scheduleSync(delay) {
  clearTimeout(syncTimer);
  syncTimer = setTimeout(syncTree, delay);
}

function noteActivity() {
  lastActivity = Date.now();
  scheduleSync(ACTIVE_MS);
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
  input.send(touchFrame(PHASE.down, x, y));
  noteActivity();
});
mirror.addEventListener("pointermove", (e) => {
  if (!pointerDown) return;
  const { x, y } = normalized(e);
  input.send(touchFrame(PHASE.move, x, y));
});
mirror.addEventListener("pointerup", (e) => {
  if (!pointerDown) return;
  pointerDown = false;
  const { x, y } = normalized(e);
  input.send(touchFrame(PHASE.up, x, y));
  noteActivity();
});

document.addEventListener("keydown", (e) => {
  const frame = keyEventFrame(e);
  if (!frame) return;
  e.preventDefault();
  input.send(frame);
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
  // Unscoped trees follow the frontmost app (the runner resolves it);
  // ?app= narrows to one bundle when tests want isolation.
  syncTree();
}
start();
