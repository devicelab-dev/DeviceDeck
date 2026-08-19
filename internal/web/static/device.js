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
// label.
//
// Same-key siblings are disambiguated by occurrence order *within their
// parent*, never across the whole tree. Counting globally means one
// unidentified node appearing or disappearing anywhere above shifts the
// ordinal of every later node sharing its base key — they all re-key,
// their elements are replaced rather than reused, and every automation
// handle into them (Playwright aria-refs, Selenium elements) dies for a
// change that never touched them. Unlabelled containers make that the
// common case, not the corner case.
function nodeKey(node, parentKey, counts) {
  const base =
    node.identifier || `${node.type}|${node.placeholder || node.label || ""}`;
  const scoped = `${parentKey}/${base}`;
  const n = counts.get(scoped) || 0;
  counts.set(scoped, n + 1);
  return `${scoped}#${n}`;
}

// FIELD_TYPES name themselves from their placeholder rather than their
// label — see the note in syncNode.
const FIELD_TYPES = new Set(["TextField", "SecureTextField", "SearchField"]);

function accessibleName(node) {
  if (FIELD_TYPES.has(node.type) && node.placeholder) return node.placeholder;
  return node.label || node.placeholder || "";
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
  // elements still read as `textbox "Username"` in aria snapshots.
  //
  // Text fields invert that order, because the platforms report a
  // field's current contents as its label. Naming from it renames the
  // field on every keystroke: getByRole("textbox", {name: "Username"})
  // and getByLabel("Username") stop matching the moment a user types,
  // and any handle held on the element — including the refs an AI agent
  // addresses it by — is invalidated. A field is named by what it asks
  // for, not by what has been entered; the contents stay reachable as
  // the element's text.
  setOrRemove(el, "aria-label", accessibleName(node));
  setOrRemove(el, "aria-placeholder", node.placeholder || "");
  // Always explicit, never removed. aria-disabled is inherited down the
  // ancestor chain, and a native container frequently reports disabled
  // while an enabled control sits inside it — leaving the attribute off
  // the child would let the container's "true" claim it. An explicit
  // "false" stops the walk at the node itself.
  el.setAttribute("aria-disabled", node.enabled === false ? "true" : "false");
  setOrRemove(el, "aria-selected", node.selected ? "true" : "");
  // Device truth for the states ARIA cannot carry everywhere:
  // aria-disabled is only honoured for a fixed set of roles, so a
  // role-less container's disabled state is invisible to the web tools
  // without this. hittable is diagnostic only — XCUITest computes it
  // relative to the app under test, so everything in another window
  // (the keyboard, alerts) reports false even while plainly tappable,
  // which makes it unusable as a pointer-events signal.
  el.setAttribute("data-dd-enabled", node.enabled === false ? "false" : "true");
  el.setAttribute("data-dd-hittable", node.hittable ? "true" : "false");
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

// twinOf returns the immediately preceding rendered sibling when it
// occupies exactly the same frame, meaning the two nodes describe one
// control. Only an exact match counts: a merely overlapping element is a
// real overlay and must keep intercepting, because on the device it
// would genuinely take the touch.
function twinOf(parentAnchor, node) {
  const prev = parentAnchor.lastChild;
  if (!prev) return null;
  const a = prev.frame;
  const b = node.frame;
  const same =
    a.x === b.x && a.y === b.y && a.width === b.width && a.height === b.height;
  return same ? prev : null;
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
  const rootAnchor = { el: mirror, frame: app, key: "", lastChild: null };
  for (const node of nodes) {
    const parentAnchor =
      (node.parentIndex != null && anchors.get(node.parentIndex)) || rootAnchor;
    if (!mirrorable(node)) {
      anchors.set(node.index, parentAnchor); // children inherit the anchor
      continue;
    }
    // One control, two nodes: platforms routinely split a control's
    // identifier onto a wrapper and its label and role onto a twin with
    // the same frame — Flutter does it for every merged-semantics
    // widget. Left as siblings they overlap exactly, the later one
    // paints on top, and a click aimed at the identifier is refused as
    // intercepted by its own twin. Nested, the twin is a descendant,
    // which is what a hit test accepts, so both selectors resolve to
    // something clickable.
    const anchor = twinOf(parentAnchor, node) || parentAnchor;
    const key = nodeKey(node, anchor.key, counts);
    const el = acquireEl(existing, key);
    syncNode(el, node, anchor.frame);
    // Append-or-move keeps DOM order tracking native order; moving
    // (including across parents) preserves element identity.
    anchor.el.appendChild(el);
    const rendered = { el, frame: node.frame, key, lastChild: null };
    anchor.lastChild = rendered;
    anchors.set(node.index, rendered);
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

// Quiescence signal for tests. Playwright's own "stable" check samples
// getBoundingClientRect across two animation frames, but the mirror only
// moves at poll boundaries, so between polls it looks stable however
// hard the device is animating — the check cannot see a native
// transition. Publishing settledness on the root gives tests something
// real to await: expect(mirror).toHaveAttribute("data-dd-settled","true").
const SETTLE_POLLS = 3;
let quietPolls = 0;
let lastHash = null;

// The server decides what counts as a change — it excludes geometry noise
// and the status-bar clock, which would otherwise stop any screen from
// ever looking quiet. Comparing raw snapshots here would re-derive those
// exclusions in a second place, and get them wrong.
function noteSettle(hash) {
  quietPolls = hash === lastHash ? quietPolls + 1 : 0;
  lastHash = hash;
  mirror.setAttribute("data-dd-settled", quietPolls >= SETTLE_POLLS ? "true" : "false");
}
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
      const payload = await res.json();
      renderMirror(payload.nodes);
      if (lastTreeJSON !== before) lastActivity = Date.now();
      noteSettle(payload.hash);
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
