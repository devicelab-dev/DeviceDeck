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
// pageSession prefixes this page's action IDs (see act), so a retried
// action is recognised and never done twice.
const pageSession = Math.random().toString(36).slice(2) + Date.now().toString(36);
// Video is off by default on this page, which is the automation surface.
// A driver reads the DOM mirror, not the pixels: clicks are normalised to
// the device's own coordinates and mirror nodes are positioned as
// percentages of the tree's device frame, so the video contributes
// nothing but load — one H.264 sidecar per headless session, streaming
// frames no driver ever decodes. Humans who want to watch pass ?video=1
// (the console at / is the usual place to watch). See sizeStage for how
// the canvas gets its dimensions without a frame to measure.
const wantVideo = new URLSearchParams(location.search).get("video") === "1";
// ?reset=yes launches the app fresh — data wiped, logged out — before the
// mirror shows anything, so a driver that opened this page for a new
// session starts at the app's first screen, not wherever the last session
// left it. Web automation frameworks assume a new session is clean; a
// native app persists across relaunches, so the page asks the server to
// make it so. Opt-in and gated on ?app= (which app to launch): launching
// on load is destructive, and a plain refresh must never wipe state.
const wantReset =
  !!appId && new URLSearchParams(location.search).get("reset") === "yes";
const canvas = /** @type {HTMLCanvasElement} */ (document.getElementById("video"));
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

// showNotice puts a human-readable reason on screen. The text goes into
// an attribute and is painted by CSS generated content, so the page gains
// no text node a locator could match — see device.html for why aria-hidden
// alone was not enough.
function showNotice(text) {
  const notice = document.getElementById("notice");
  notice.setAttribute("data-reason", text);
  notice.setAttribute("data-shown", "");
}

// The server's reason travels in a close frame, which holds 123 bytes —
// so the remedy is exactly the part that gets truncated away. The page
// knows the remedy without being told, so it says it itself.
const REFUSAL_REMEDY =
  "Close the other DeviceDeck tab or test run for this device, then reload.";

function connectInput() {
  input = createInputSocket(wsURL(`/api/devices/${udid}/input`), {
    // Surfaced in the title so a driver that cannot drive says so
    // somewhere a test's failure message will show it.
    onRefused: (reason) => {
      document.title = `DeviceDeck — ${reason || "device busy"}`;
      mirror.setAttribute("data-dd-input-refused", reason || "device busy");
      showNotice(`${reason || "Another client is driving this device."} — ${REFUSAL_REMEDY}`);
    },
  });
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
  RadioGroup: "radiogroup",
  SegmentedControl: "radiogroup",
  Stepper: "spinbutton",
  IncrementArrow: "button",
  DecrementArrow: "button",
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

// sizeStage gives the canvas the device's dimensions from the tree, so
// the mirror has the right size and aspect with no video frame to
// measure. When video is on it manages the canvas itself (at real pixel
// resolution), and this stands aside — the mirror's own layout is in
// percentages, so the absolute size never affects where a click lands.
function sizeStage(app) {
  if (wantVideo) return;
  const w = Math.round(app.width);
  const h = Math.round(app.height);
  if (canvas.width !== w || canvas.height !== h) {
    canvas.width = w;
    canvas.height = h;
  }
}

function positionMirror() {
  const stage = document.getElementById("stage").getBoundingClientRect();
  const rect = canvas.getBoundingClientRect();
  mirror.style.left = `${rect.left - stage.left}px`;
  mirror.style.top = `${rect.top - stage.top}px`;
  mirror.style.width = `${rect.width}px`;
  mirror.style.height = `${rect.height}px`;
  gutter = { x: rect.width, y: rect.height };
  mirror.style.setProperty("--dd-gutter-x", `${gutter.x}px`);
  mirror.style.setProperty("--dd-gutter-y", `${gutter.y}px`);
  resetScroll();
}

// gutter is how far the screen sits inside the mirror's scrollable area,
// one screen each way. A scroll box cannot scroll to a negative offset, so
// without it an element past the left or top edge — a filter chip a
// horizontal row has scrolled away — could never be scrolled into view:
// Playwright's click waited out its whole timeout on it. With the screen
// shifted by the gutter (device.html) and the view scrolled back by the
// same amount, nothing moves on screen, and there is room to scroll left
// and up as well as right and down.
let gutter = { x: 0, y: 0 };

// Identity key for reconciliation: the identifier when present, else
// type + placeholder (stable while a field's value changes) or label,
// scoped to the parent.
//
// Unidentified siblings are disambiguated by occurrence order *within their
// parent*, never across the whole tree. Counting globally means one
// unidentified node appearing or disappearing anywhere above shifts the
// ordinal of every later node sharing its base key — they all re-key,
// their elements are replaced rather than reused, and every automation
// handle into them (Playwright aria-refs, Selenium elements) dies for a
// change that never touched them. Unlabelled containers make that the
// common case, not the corner case. An identified node is the opposite
// case — its ancestry is the unstable part — so it keys globally by
// identifier; see below.
function nodeKey(node, parentKey, counts) {
  // A stable identifier keys the node on its own, independent of the volatile
  // ancestry above it. Scoping an identified control to its parent chain
  // renamed it whenever a transient container reshaped that chain — a nav
  // bar's Back button flickering in as the keyboard animates inserted an
  // ancestor, changing the key of the field being typed into. The renamed
  // node was then treated as new, its element recreated, and any in-flight
  // reconcile (and a test's held aria-ref) left pointing at an orphan. An
  // identifier is the promise that identity does not depend on surroundings,
  // so key it that way; unidentified nodes still key positionally.
  const scoped = node.identifier
    ? `#id/${node.identifier}`
    : `${parentKey}/${node.type}|${node.placeholder || node.label || ""}`;
  const n = counts.get(scoped) || 0;
  counts.set(scoped, n + 1);
  return `${scoped}#${n}`;
}

// FIELD_TYPES name themselves from their placeholder rather than their
// label — see the note in syncNode.
const FIELD_TYPES = new Set(["TextField", "SecureTextField", "SearchField"]);

// Roles that represent something a person taps. Only these carry their
// identifier in the name.
const CONTROL_ROLES = new Set([
  "button", "link", "textbox", "searchbox", "checkbox", "radio", "switch", "tab",
]);

// baseName is a node's own name before an identifier is folded in. A
// native container repeats this string, not the decorated one, so it is
// what an ancestor's name has to be compared against.
//
// For a field this is placeholder before label — the reverse of the
// HTML-AAM name computation, which puts placeholder last. Deliberate:
// the platforms report a field's current contents as its label, so the
// spec order would rename the field on every keystroke (see syncNode).
// The placeholder is what the field asks for, and the only stable name.
function baseName(node) {
  if (FIELD_TYPES.has(node.type) && node.placeholder) return node.placeholder;
  return node.label || node.placeholder || "";
}

// accessibleName is what an agent reads, and the only thing it can address
// an element by.
//
// A control's identifier is folded in because the name is the sole channel
// that survives into an accessibility snapshot: data-testid,
// aria-description, title, aria-keyshortcuts and aria-roledescription were
// each measured and none of them appear. An app can therefore label six
// buttons "Add", number their identifiers add-to-cart-1..6, and leave an
// agent choosing blindly between six identical names while the key that
// separates them sits in the DOM unread. That is exactly what happened
// when one was asked to add a named product to a cart.
//
// Controls only, because that is where ambiguity costs a wrong action
// rather than a slow reading, and it keeps the snapshot from doubling in
// size. The suffix is stable: an identifier is the app's own test ID, so
// unlike naming a field from its contents — which renamed it on every
// keystroke — this never changes as the screen does.
function accessibleName(node) {
  const base = baseName(node);
  const id = node.identifier || "";
  if (!id || !CONTROL_ROLES.has(ROLES[node.type] || "")) return base;
  if (!base) return id;
  return base === id ? base : `${base} (${id})`;
}

// ARIA carries a state only on the roles that own it: checked on a
// checkable role, a value on a range role, selected on a tab, option or
// row. Chrome drops the attribute anywhere else, so a selected segment of
// a segmented control — a Button on iOS — is exposed as pressed instead,
// which is what its aria snapshot prints and what an agent reads.
const CHECKABLE_ROLES = new Set(["switch", "checkbox", "radio"]);
const RANGE_ROLES = new Set(["slider", "progressbar", "spinbutton"]);

// checkedState reads a checkable control's state off its device value.
// iOS reports a switch or checkbox as "1"/"0". Android's page source
// carries the state in an attribute the runner does not parse yet, so its
// value is the control's own text — anything but "1"/"0" yields no state
// rather than a wrong one.
function checkedState(value) {
  if (value === "1") return "true";
  if (value === "0") return "false";
  return "";
}

// rangeNumber pulls the number out of a range control's value: iOS
// reports a slider as "50%" and a stepper as "3".
function rangeNumber(value) {
  const m = /-?\d+(\.\d+)?/.exec(value || "");
  return m ? m[0] : "";
}

// syncStates mirrors the device's states onto the ARIA attributes that
// carry them for this role. aria-disabled is always explicit, never
// removed: it is inherited down the ancestor chain, and a native
// container frequently reports disabled while an enabled control sits
// inside it — leaving the attribute off the child would let the
// container's "true" claim it. An explicit "false" stops the walk at the
// node itself.
function syncStates(el, node, role) {
  el.setAttribute("aria-disabled", node.enabled === false ? "true" : "false");
  const selected = node.selected ? "true" : "";
  setOrRemove(el, "aria-pressed", role === "button" ? selected : "");
  setOrRemove(el, "aria-selected", role === "button" ? "" : selected);
  setOrRemove(el, "aria-checked", CHECKABLE_ROLES.has(role) ? checkedState(node.value) : "");
  const range = RANGE_ROLES.has(role);
  setOrRemove(el, "aria-valuenow", range ? rangeNumber(node.value) : "");
  setOrRemove(el, "aria-valuetext", range ? node.value || "" : "");
}

function setOrRemove(el, attr, value) {
  if (value) el.setAttribute(attr, value);
  else el.removeAttribute(attr);
}

// syncNode positions el inside its rendered ancestor: node frames are
// screen-absolute, so coordinates convert to percentages of the
// ancestor's frame and stay proportional at any canvas size.
// nodeFrames maps each mirrored element to its node's frame in the tree's
// own units — what a fill tells the driver, which knows no page geometry.
const nodeFrames = new WeakMap();

function syncNode(el, node, anchorFrame, owners) {
  nodeFrames.set(el, node.frame);
  placeNode(el, node.frame, anchorFrame);
  setOrRemove(el, "role", ROLES[node.type] || "");
  // Only the owner of a contested identifier carries it; see
  // identifierOwners for why the others are left without one.
  const owner = owners.get(node.identifier);
  const mine = owner === undefined || owner === node.index;
  setOrRemove(el, "data-testid", mine ? node.identifier || "" : "");
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
  syncStates(el, node, ROLES[node.type] || "");
  // Device truth for the states ARIA cannot carry everywhere:
  // aria-disabled is only honoured for a fixed set of roles, so a
  // role-less container's disabled state is invisible to the web tools
  // without this. hittable is diagnostic only — XCUITest computes it
  // relative to the app under test, so everything in another window
  // (the keyboard, alerts) reports false even while plainly tappable,
  // which makes it unusable as a pointer-events signal.
  el.setAttribute("data-dd-enabled", node.enabled === false ? "false" : "true");
  el.setAttribute("data-dd-hittable", node.hittable ? "true" : "false");
  // The device's own focus, so typing can wait for the field to actually
  // hold focus rather than guess how long the tap took — see the input
  // handler. Mirrored per poll like the other states.
  el.setAttribute("data-dd-focused", node.focused ? "true" : "false");
  // An <input> holds its contents in .value and can have no children, so
  // it takes neither the text node below nor anything nested.
  if (el.tagName === "INPUT") {
    syncField(el, node);
    return;
  }
  syncText(el, node.label || node.value || "");
}

// placeNode positions el over frame f as percentages of its anchor's frame.
function placeNode(el, f, anchorFrame) {
  el.style.left = `${((f.x - anchorFrame.x) / anchorFrame.width) * 100}%`;
  el.style.top = `${((f.y - anchorFrame.y) / anchorFrame.height) * 100}%`;
  el.style.width = `${(f.width / anchorFrame.width) * 100}%`;
  el.style.height = `${(f.height / anchorFrame.height) * 100}%`;
}

// syncText keeps the text for getByText (label, else value, painted
// transparent) in a dedicated leading text node — assigning textContent
// would destroy the nested child elements.
function syncText(el, text) {
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

// syncField mirrors the device's own contents into the input — except
// while it holds focus. The tree lags a keystroke behind, so writing its
// value back mid-edit would delete characters as fast as they are typed.
// ddValue records what the page last showed or was given.
function syncField(el, node) {
  const value = node.value || "";
  // The device's own contents, kept even while the field holds focus (when
  // the write below is suppressed): tests and agents read it to see what
  // the device holds, and a fill sizes an empty-field clear by it. A secure
  // field reports bullets, so it is compared by length; mark it.
  el.dataset.ddDeviceValue = value;
  el.dataset.ddSecure = node.type === "SecureTextField" ? "true" : "false";
  if (el !== document.activeElement && el.value !== value) el.value = value;
  el.dataset.ddValue = el.value;
  setOrRemove(el, "placeholder", node.placeholder || "");
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

// dropRepeatedName strips a name from a container that only echoes the
// node beneath it.
//
// Native accessibility trees aggregate upward: one button's label also
// names its wrapper, that wrapper's wrapper, and so on to the window. A
// snapshot then prints the string once per level — and twice per level
// for a container that has element children, because its own text node
// prints alongside its name. Three levels of wrapper read as six
// identical entries, all with the same box, and an agent has no way to
// tell which one it is meant to click. That is not a hypothetical: a
// captured snapshot of the login screen spent its first eleven lines
// saying "Typing Predictions" and "Passwords" over and over before
// reaching the one button that existed.
//
// The echo is stripped from the container, never from the node that
// earned it: a container carrying a mapped role is a real control and
// keeps its name, as does the innermost wrapper of a chain, which
// nothing beneath it repeats. On the captured chain that takes the
// snapshot from twelve lines to four — a stripped wrapper carries
// neither name nor text, and the reader folds it away. Not every
// anonymous generic disappears, so this buys a smaller snapshot, not an
// empty one; mirror-noise.spec.ts holds the measurement.
function dropRepeatedName(el, name) {
  if (!name || el.hasAttribute("role")) return;
  if (el.getAttribute("aria-label") !== name) return;
  el.removeAttribute("aria-label");
  const first = el.firstChild;
  if (first && first.nodeType === Node.TEXT_NODE && first.data === name) first.remove();
}

// Several nodes routinely carry one identifier: a cart icon and the
// badge counting what is in it, a search glass and the field beside it.
// Every one of them becomes a data-testid, so getByTestId matches more
// than one element and a strict-mode query fails outright — which is how
// a spec here stopped being able to click the cart the moment anything
// was in it.
//
// Give the identifier to the node it is really for, but only when that
// is unambiguous: the single control among them, or the one whose frame
// contains the rest. Six product rows that all reuse "star.fill" are six
// real buttons, and picking one would hide five controls that exist —
// that ambiguity belongs to the app, and the mirror reports it as it is
// rather than inventing a uniqueness the app never had.
// Frames arrive as floating-point points, and a badge sitting flush to
// its icon's edge misses exact containment by a ten-thousandth of a
// point. The tolerance is sub-pixel: it forgives that rounding without
// reaching anything genuinely alongside — the search glass and its field
// are ten points apart.
const FRAME_SLACK = 1;

function contains(outer, inner) {
  return (
    inner.x >= outer.x - FRAME_SLACK && inner.y >= outer.y - FRAME_SLACK &&
    inner.x + inner.width <= outer.x + outer.width + FRAME_SLACK &&
    inner.y + inner.height <= outer.y + outer.height + FRAME_SLACK
  );
}

// identifierOwner returns the index of the node that keeps the shared
// identifier, or null to leave every node in the group carrying it.
function identifierOwner(group) {
  if (group.length < 2) return null;
  const controls = group.filter((n) => CONTROL_ROLES.has(ROLES[n.type] || ""));
  if (controls.length === 1) return controls[0].index;
  const outer = group.find((n) => group.every((m) => contains(n.frame, m.frame)));
  return outer ? outer.index : null;
}

// identifierOwners maps each contested identifier to the one node that
// should carry it. An identifier used once never appears here.
function identifierOwners(nodes) {
  const groups = new Map();
  for (const node of nodes) {
    if (!node.identifier || !mirrorable(node)) continue;
    const group = groups.get(node.identifier) || [];
    group.push(node);
    groups.set(node.identifier, group);
  }
  const owners = new Map();
  for (const [id, group] of groups) {
    const owner = identifierOwner(group);
    if (owner != null) owners.set(id, owner);
  }
  return owners;
}

// A node earns a mirror element only if something can find it: an
// identifier, visible text, or a mapped role. Zero-size nodes never do.
function mirrorable(node) {
  if (node.depth === 0) return false;
  if (!(node.frame.width > 0) || !(node.frame.height > 0)) return false;
  return !!(node.identifier || node.label || node.value || ROLES[node.type]);
}

// A text field is mirrored as a real <input>, every other node as a div.
// The reason is what the tools refuse: fill(), and the browser_type an AI
// agent reaches for first, both reject anything that is not an <input>,
// <textarea> or [contenteditable]. Against a div-based field an agent
// cannot type at all — it reports the page as broken and stops. Every
// example shipped here drives a field with click + keyboard.type, which
// is why the gap survived until an agent was actually pointed at it.
//
// The node's type is part of its key, so a node that changes type gets a
// new key and therefore a correctly-tagged new element.
function acquireEl(existing, key, node) {
  const found = existing.get(key);
  if (found) {
    existing.delete(key);
    return found;
  }
  const el = document.createElement(FIELD_TYPES.has(node.type) ? "input" : "div");
  if (el instanceof HTMLInputElement) {
    // type=text even for secure fields: the role attribute below already
    // says textbox, and type=password invites the browser's own password
    // manager into a page that is meant to contain nothing but the app.
    // The device reports a secure field's contents as bullets, so the
    // value stays honest about its length either way.
    el.type = "text";
    el.autocomplete = "off";
    el.spellcheck = false;
  }
  el.setAttribute("data-dd-node", "");
  el.setAttribute("data-dd-key", key);
  return el;
}

// renderNode places one node's element under its nearest rendered
// ancestor. pass carries what a single refresh shares between nodes:
// the elements left from last time, the sibling counters that key them,
// and the anchors that children position against.
function renderNode(node, pass) {
  const parentAnchor =
    (node.parentIndex != null && pass.anchors.get(node.parentIndex)) || pass.root;
  if (!mirrorable(node)) {
    pass.anchors.set(node.index, parentAnchor); // children inherit the anchor
    return;
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
  const key = nodeKey(node, anchor.key, pass.counts);
  const el = acquireEl(pass.existing, key, node);
  syncNode(el, node, anchor.frame, pass.owners);
  // Never the root: the mirror's own name is the screen fingerprint,
  // which belongs to no node and must survive every refresh.
  if (anchor !== pass.root) dropRepeatedName(anchor.el, baseName(node));
  placeInOrder(anchor, el);
  const rendered = { el, frame: node.frame, key, lastChild: null };
  anchor.lastChild = rendered;
  pass.anchors.set(node.index, rendered);
}

// placeInOrder keeps DOM order tracking native order: el goes right after
// the last element already placed under anchor. An element already there
// is left alone, and one that must move is moved atomically where the
// browser can: re-appending removes and reinserts, which blurs a focused
// field — and a field that lost focus shows the device's value, a secure
// field's bullets, instead of the text just filled into it.
function placeInOrder(anchor, el) {
  const prev = anchor.lastChild ? anchor.lastChild.el : null;
  const next = prev ? prev.nextSibling : anchor.el.firstElementChild;
  if (el === next) return;
  if (el.isConnected && "moveBefore" in anchor.el) {
    anchor.el.moveBefore(el, next);
  } else {
    anchor.el.insertBefore(el, next);
  }
}

function renderMirror(nodes) {
  const json = JSON.stringify(nodes);
  if (json === lastTreeJSON) return;
  lastTreeJSON = json;

  const app = nodes.length ? nodes[0].frame : null;
  if (!app || !(app.width > 0) || !(app.height > 0)) {
    positionMirror();
    mirror.innerHTML = "";
    return;
  }
  sizeStage(app);
  positionMirror();

  // Reconcile instead of rebuilding: an element that survives a refresh
  // keeps its DOM identity, so automation handles held across refreshes
  // (Playwright aria-refs, Selenium elements) stay valid between actions.
  const existing = new Map();
  for (const el of mirror.querySelectorAll("[data-dd-key]")) {
    existing.set(el.getAttribute("data-dd-key"), el);
  }
  // The mirror nests like the native tree: a node's element is appended
  // under its nearest *rendered* ancestor. Nesting is what makes a text
  // child a legitimate hit target for clicks aimed at its container
  // (Playwright's actionability check), and native paint order becomes
  // plain DOM order — no z-index arithmetic. An anchor carries the
  // container element + frame that a node's children position against.
  const pass = {
    existing,
    owners: identifierOwners(nodes),
    counts: new Map(),
    anchors: new Map(),
    root: { el: mirror, frame: app, key: "", lastChild: null },
  };
  for (const node of nodes) renderNode(node, pass);
  // Whatever no node claimed this pass is gone from the device.
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

// nameScreen puts the screen's fingerprint into the mirror root's name.
//
// An agent has no way to tell which screen it is on: the tree carries no
// title, no heading and no landmark, and the container names an app
// happens to expose are unreliable — one screen here reports itself as
// "testtube.2", an icon asset name. Asked directly, an agent said it
// inferred the screen every time and never read it. The fingerprint does
// not say what the screen is, but it says when it is no longer the same
// one, which is what an agent needs to know that an action landed. It
// goes in the root's name because a name is the only thing a snapshot
// carries, and the root is ours — putting it anywhere else would add our
// words to the app's own content.
function nameScreen(hash, foreground) {
  const base = hash ? `device screen ${hash.slice(0, 8)}` : "device";
  // When the app is not frontmost the mirror still holds its last
  // screen — iOS keeps a backgrounded app's tree, so the controls are
  // all still here and would still be clicked. The tree cannot show
  // this; the app's own lifecycle state can. It goes in the root name,
  // the one channel a snapshot carries, so an agent reads "app not in
  // foreground" rather than acting on a screen that is not on the
  // device. Annotated, never blocked: the tap still forwards, because a
  // wrong state read must not strand a caller.
  mirror.setAttribute("aria-label", foreground === false ? `${base} — app not in foreground` : base);
  mirror.setAttribute("data-dd-foreground", foreground === false ? "false" : "true");
}

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

// The barrier. An action marks the next fetch as "held": it carries the
// interaction hash of the screen the action was taken on, and the server
// does not answer until the screen has moved on from it and come to
// rest. That one request is what playwright-mcp waits on after every
// action, so an agent gets the settled screen without knowing it asked
// for it. Only the post-action fetch is held — idle polls answer at
// once, so anything else that happens to fall inside a tool's wait
// window costs nothing.
let lastInteraction = "";
// The first fetch is held too, for quiet alone — there is no hash to
// have moved on from yet. An agent's navigate waits on it, so its first
// snapshot is the settled screen rather than an empty mirror: measured
// as one ref and no fields, on a page whose app was fully up.
let holdNext = true;
// An action taken while a held fetch is still open aborts it: the new
// action's barrier has to be issued promptly, and the old hold was
// waiting for a screen the user has already moved past.
let inFlight = null;

function treeURL() {
  const params = new URLSearchParams();
  if (appId) params.set("app", appId);
  const held = holdNext;
  if (held) params.set("after", lastInteraction);
  holdNext = false;
  const query = params.toString();
  return { url: `/api/devices/${udid}/tree${query ? `?${query}` : ""}`, held };
}

async function syncTree() {
  // One fetch at a time: tree dumps serialize on the device side, and a
  // pile-up would queue input calls behind them for seconds.
  if (fetchInFlight) {
    scheduleSync(FAST_MS);
    return;
  }
  fetchInFlight = true;
  const seq = ++syncSeq;
  const ctl = new AbortController();
  inFlight = ctl;
  try {
    const { url, held } = treeURL();
    const res = await fetch(url, { signal: ctl.signal });
    if (res.ok && seq === syncSeq) applyTree(await res.json(), held);
  } catch {}
  // An aborted fetch owns nothing any more: the action that aborted it
  // has already released the in-flight state and armed its own timer.
  if (ctl.signal.aborted) return;
  inFlight = null;
  fetchInFlight = false;
  scheduleSync(Date.now() - lastActivity < 5000 ? FAST_MS : IDLE_MS);
}

// applyTree renders a tree payload and updates everything derived from
// it: the fingerprint, the hash the next barrier compares against, the
// scroll offset, and settledness.
function applyTree(payload, held) {
  const before = lastTreeJSON;
  renderMirror(payload.nodes);
  nameScreen(payload.hash, payload.foreground);
  lastInteraction = payload.interaction || "";
  if (lastTreeJSON !== before) lastActivity = Date.now();
  if (payload.hash !== lastHash) resetScroll();
  // A held response — and every /act answer — is the settled screen by
  // construction (the server verified it), so the barrier closes here
  // rather than after three more polls.
  if (held) quietPolls = SETTLE_POLLS;
  noteSettle(payload.hash);
}

// scheduleSync (re)arms the single sync timer; a shorter pending delay
// is never lengthened.
function scheduleSync(delay) {
  clearTimeout(syncTimer);
  syncTimer = setTimeout(syncTree, delay);
}

function noteActivity() {
  lastActivity = Date.now();
  holdNext = true;
  // The gate: nothing in the mirror takes a click until the settled
  // tree is back. See the rule on #mirror[data-dd-settled="false"].
  quietPolls = 0;
  mirror.setAttribute("data-dd-settled", "false");
  if (inFlight) {
    inFlight.abort();
    inFlight = null;
    fetchInFlight = false;
  }
  scheduleSync(ACTIVE_MS);
}

// ---------- interaction forwarding ----------

// normalized maps a page point onto the device screen, 0..1 each way,
// clamped to the edges. Screen space: what a finger can touch.
function normalized(event) {
  const rect = canvas.getBoundingClientRect();
  const clamp = (v) => Math.min(1, Math.max(0, v));
  return {
    x: clamp((event.clientX - rect.left) / rect.width),
    y: clamp((event.clientY - rect.top) / rect.height),
  };
}

// contentPoint maps a page point onto the mirror's content, unclamped.
//
// The mirror is a scroll container, and tools scroll it: Playwright
// aligns an element before clicking — with alternating alignments when
// it retries, even for an element already in view — and Cypress does the
// same. That scroll is a view transform the tool is entitled to apply,
// and it is undone here: the point a click lands on, plus the offset the
// mirror is scrolled by, is where that element sits on the device. A
// result inside 0..1 is a place a finger can touch. A result past 1 is a
// row below the fold, which XCUITest reports with its real frame and the
// mirror holds past its own edge — see tapOffscreen.
function contentPoint(clientX, clientY) {
  const rect = mirror.getBoundingClientRect();
  return {
    x: (clientX - rect.left + mirror.scrollLeft - gutter.x) / rect.width,
    y: (clientY - rect.top + mirror.scrollTop - gutter.y) / rect.height,
  };
}

function onDevice(p) {
  return p.x >= 0 && p.x <= 1 && p.y >= 0 && p.y <= 1;
}

// ---------- acting on the device ----------

// act performs one action on the device and returns only when the device
// has done it: a synchronous request to /act, which answers with the
// settled tree after the action, applied here before returning.
//
// Blocking is the point. Chromium acknowledges an input event only after
// the page's handlers for it have run, and a browser automation tool's
// click(), fill() or press() resolves on that acknowledgement — so the
// tool's action finishes when the device's does, and its next step and its
// assertions read the device's real screen. Measured with Playwright: a
// handler blocked 6s → click() 6008ms, fill() 6007ms. Handing the action
// off asynchronously let the tool move on while the device was still
// acting; the page then had to guess when the device caught up, and each
// guess failed under some tool: text in the wrong field, a click ahead of
// the text, a value typed twice. The console (app.js) is the page for
// people and streams input live; this page is the automation surface.
let actSeq = 0;
function act(intent) {
  // An older poll still in flight must not land on top of this answer.
  syncSeq++;
  if (inFlight) {
    inFlight.abort();
    inFlight = null;
    fetchInFlight = false;
  }
  mirror.setAttribute("data-dd-settled", "false");
  const xhr = new XMLHttpRequest();
  xhr.open("POST", `/api/devices/${udid}/act`, false);
  xhr.setRequestHeader("Content-Type", "application/json");
  try {
    xhr.send(JSON.stringify({ id: `${pageSession}-${++actSeq}`, app: appId, after: lastInteraction, ...intent }));
  } catch {
    noteActivity();
    return null;
  }
  if (xhr.status !== 200) {
    noteActivity();
    return null;
  }
  const payload = JSON.parse(xhr.responseText);
  applyTree(payload, true);
  lastActivity = Date.now();
  scheduleSync(FAST_MS);
  return payload;
}

// A press and its release are one tap, or — past DRAG_SLOP_PX — one
// swipe: the device gets the gesture whole, when the release says what it
// was. The mirror node under the pointer is for finding; the device gets
// the true coordinates, like a finger.
const DRAG_SLOP_PX = 8;
let press = null;
mirror.addEventListener("pointerdown", (e) => {
  press = { clientX: e.clientX, clientY: e.clientY, target: e.target, at: Date.now() };
  // Synthetic events may carry no capturable pointerId; capture only keeps
  // a drag's release on the mirror.
  try { mirror.setPointerCapture(e.pointerId); } catch {}
});
mirror.addEventListener("pointerup", (e) => {
  if (!press) return;
  const p = press;
  press = null;
  if (Math.hypot(e.clientX - p.clientX, e.clientY - p.clientY) > DRAG_SLOP_PX) {
    const from = normalized(p);
    const to = normalized(e);
    act({ kind: "swipe", x: from.x, y: from.y, toX: to.x, toY: to.y, durationMs: Date.now() - p.at });
    return;
  }
  tapAt(p.target, contentPoint(e.clientX, e.clientY));
});

// tapAt taps the device where a mirrored element sits — revealing it first
// when the mirror holds it past the device's edge.
function tapAt(el, at) {
  if (!onDevice(at)) {
    revealAndTap(el, at);
    return;
  }
  act({ kind: "tap", x: at.x, y: at.y });
}

// ---------- scrolling ----------

// Scroll input — wheel, trackpad, Playwright's mouse.wheel — becomes one
// swipe on the device once the burst ends.
const WHEEL_COALESCE_MS = 40;
let wheelAccum = { x: 0, y: 0, clientX: 0, clientY: 0 };
let wheelTimer = null;

document.addEventListener("wheel", (e) => {
  e.preventDefault();
  wheelAccum.x += e.deltaX;
  wheelAccum.y += e.deltaY;
  wheelAccum.clientX = e.clientX;
  wheelAccum.clientY = e.clientY;
  clearTimeout(wheelTimer);
  wheelTimer = setTimeout(flushWheel, WHEEL_COALESCE_MS);
}, { passive: false });

// flushWheel swipes by the accumulated delta, from the pointer when it is
// over the device, else from the centre.
function flushWheel() {
  const { x: dx, y: dy, clientX, clientY } = wheelAccum;
  wheelAccum = { x: 0, y: 0, clientX: 0, clientY: 0 };
  const rect = canvas.getBoundingClientRect();
  const inside =
    clientX >= rect.left && clientX <= rect.right && clientY >= rect.top && clientY <= rect.bottom;
  const from = inside ? normalized({ clientX, clientY }) : { x: 0.5, y: 0.5 };
  const to = {
    x: clamp01(from.x - dx / rect.width),
    y: clamp01(from.y - dy / rect.height),
  };
  act({ kind: "swipe", x: from.x, y: from.y, toX: to.x, toY: to.y, durationMs: 250 });
}

function clamp01(v) {
  return Math.min(1, Math.max(0, v));
}

// resetScroll puts the mirror's view back on the screen, one gutter in.
// Tools scroll it to align an element before clicking; that offset belongs
// to the screen it was taken on.
function resetScroll() {
  if (mirror.scrollLeft !== gutter.x || mirror.scrollTop !== gutter.y) mirror.scrollTo(gutter.x, gutter.y);
}

// revealAndTap brings a row the mirror holds past the device's edge on
// screen, then taps it. Each swipe returns the settled tree (act), so the
// row is found again by key — the same row, though every frame on screen
// has moved — before the next step. The swipe runs along the element's own
// row or column: a horizontal list scrolls only under a finger on it, and
// a drag through the screen's centre moved a different list (measured: a
// filter chip past the right edge was "clicked" and nothing happened).
const REVEAL_AT = 0.75;
const REVEAL_TRIES = 4;
const EDGE = 0.05;
function revealAndTap(el, at) {
  const key = el.closest("[data-dd-key]")?.getAttribute("data-dd-key");
  if (!key) return;
  for (let i = 0; i < REVEAL_TRIES && !onDevice(at); i++) {
    act(revealSwipe(at));
    resetScroll();
    const found = mirror.querySelector(`[data-dd-key="${CSS.escape(key)}"]`);
    if (!found) return;
    at = centreOf(found);
  }
  if (onDevice(at)) act({ kind: "tap", x: at.x, y: at.y });
}

// revealSwipe is the swipe that moves a point outside the screen toward
// REVEAL_AT (vertically) or the middle (horizontally).
function revealSwipe(at) {
  const inset = (v) => Math.min(1 - EDGE, Math.max(EDGE, v));
  const horizontal = at.x < 0 || at.x > 1;
  const from = { x: 0.5, y: horizontal ? inset(at.y) : 0.5 };
  const dx = horizontal ? at.x - 0.5 : 0;
  const dy = at.y < 0 || at.y > 1 ? at.y - REVEAL_AT : 0;
  return { kind: "swipe", x: from.x, y: from.y, toX: inset(from.x - dx), toY: inset(from.y - dy), durationMs: 300 };
}

// ---------- keys and fields ----------

// Keys that do not edit a field's text — Enter, Tab, arrows — go to the
// device as key presses. Keys that edit a field arrive as input events.
document.addEventListener("keydown", (e) => {
  if (editsAField(e)) return;
  const frame = keyEventFrame(e);
  if (!frame) return;
  e.preventDefault();
  const b = new Uint8Array(frame);
  const usage = ((b[2] << 24) | (b[3] << 16) | (b[4] << 8) | b[5]) >>> 0;
  act({ kind: "key", usage, modifiers: b[1] });
});

// editsAField reports whether this keystroke will change a mirrored
// input's value, and so will arrive as an input event instead.
function editsAField(e) {
  const el = e.target;
  return (
    el instanceof HTMLInputElement &&
    el.hasAttribute("data-dd-node") &&
    (e.key.length === 1 || e.key === "Backspace" || e.key === "Delete")
  );
}

// Fields are set, not typed. fill() — and each keystroke of
// keyboard.type(), which lands as an input event just the same — asks for
// the field to hold a value, so the device's driver is asked for exactly
// that, and confirms it: iOS reads the value back and repairs once; Android
// re-reads the field. No keystrokes are replayed and no focus is guessed at.
mirror.addEventListener("input", (e) => {
  const el = e.target;
  if (!(el instanceof HTMLInputElement)) return;
  el.dataset.ddValue = el.value;
  fillField(el);
});

// fillField asks the device to make el hold its current value. The field
// is named by its identifier only when no other mirrored element shares
// it — the driver matches the first it finds — and always by its centre.
function fillField(el) {
  const f = nodeFrames.get(el);
  if (!f) return;
  const id = el.getAttribute("data-testid") || "";
  const unique = id && mirror.querySelectorAll(`[data-testid="${CSS.escape(id)}"]`).length === 1;
  const at = centreOf(el);
  act({
    kind: "fill",
    field: unique ? id : "",
    x: at.x, y: at.y,
    px: f.x + f.width / 2, py: f.y + f.height / 2,
    text: el.value,
    prev: (el.dataset.ddDeviceValue ?? "").length,
  });
}

// centreOf is an element's centre in content space.
function centreOf(el) {
  const r = el.getBoundingClientRect();
  return contentPoint(r.left + r.width / 2, r.top + r.height / 2);
}

// ---------- audit: what no agent can address ----------

// Roles an agent acts on. Every ref-minting tool — Playwright's aria
// snapshot, agent-browser, chrome-devtools-mcp, the Claude browser tool —
// keys its refs on role + accessible name, and data-testid alone never
// reaches a snapshot; a control here with no name has no handle at all.
const ACTIONABLE_ROLES = new Set([
  ...CONTROL_ROLES, "slider", "spinbutton", "menuitem", "listbox",
]);

// controlName is the name a snapshot would print for a mirrored control.
function controlName(el) {
  const label = el.getAttribute("aria-label");
  if (label) return label;
  return el.tagName === "INPUT" ? el.placeholder : el.textContent.trim();
}

// auditMirror lists the mirrored controls an agent cannot address: those
// with no name, and the names several controls share on one screen, where
// an agent asked for one of them is choosing blindly. Both are app facts
// the mirror reports as they are; an empty result is a fully legible
// screen. Reachable as devicedeck.audit() from page.evaluate().
function auditMirror() {
  const unnamed = [];
  const seen = new Map();
  for (const el of mirror.querySelectorAll("[data-dd-node][role]")) {
    const role = el.getAttribute("role");
    if (!ACTIONABLE_ROLES.has(role)) continue;
    const name = controlName(el);
    const testid = el.getAttribute("data-testid") || "";
    if (!name) {
      unnamed.push({ role, testid, key: el.getAttribute("data-dd-key") });
      continue;
    }
    const k = `${role}\u0000${name}`;
    seen.set(k, (seen.get(k) || 0) + 1);
  }
  const duplicates = [];
  for (const [k, count] of seen) {
    if (count < 2) continue;
    const [role, name] = k.split("\u0000");
    duplicates.push({ role, name, count });
  }
  return { unnamed, duplicates };
}

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

/** @type {any} */ (window).devicedeck = {
  udid,
  tap: (x, y) => api("/tap", { x, y }),
  swipe: (x1, y1, x2, y2, durationMs = 250) => api("/swipe", { x1, y1, x2, y2, durationMs }),
  gesture: (kind) => api("/gesture", { kind }),
  button: (button) => api("/button", { button }),
  key: (usage, modifiers = 0) => api("/key", { usage, modifiers }),
  // The device video renders to the #video canvas, so a data URL of it is the
  // device screen alone — no browser chrome, no mirror overlay — which is what
  // an agent wants to reason over. Needs video streaming; the canvas is
  // otherwise blank.
  screenshot: () => canvas.toDataURL("image/png"),
  // What on this screen no agent can address; see auditMirror.
  audit: auditMirror,
};

window.addEventListener("resize", positionMirror);

// renderFirstTree renders the tree the server put in the page, so the
// mirror is complete before the load event. Synchronous on purpose: an
// agent's navigate waits for load and nothing after it. A page served
// without one — the server could not get the tree in time — falls back
// to the first poll, held for quiet.
function renderFirstTree() {
  const slot = document.getElementById("dd-first-tree");
  if (!slot || !slot.textContent.trim()) return;
  try {
    applyTree(JSON.parse(slot.textContent), true);
    holdNext = false;
  } catch {}
}

// resetApp launches the app fresh through the launch endpoint, which
// clears stored data by default and whose reply waits until the app is
// taking input. Awaited before the first poll so the mirror's opening
// snapshot is the fresh app, not the one it replaced. A failure falls
// through to a normal poll rather than stranding the page on a bad launch.
async function resetApp() {
  try {
    await fetch(`/api/devices/${udid}/app/launch`, {
      method: "POST",
      body: JSON.stringify({ app: appId }),
    });
  } catch {}
}

// "booted" is a convenience alias; resolve it to the actual UDID up
// front — the tree engine needs a concrete device.
async function start() {
  // The inlined first tree is last session's screen; on a reset it would
  // flash the logged-in state a fresh session is leaving, so it is skipped
  // and the mirror stays empty until the relaunched app polls in. On the
  // normal path it renders synchronously, before load — all an agent's
  // navigate waits for.
  if (!wantReset) renderFirstTree();
  if (udid === "booted") {
    try {
      const { devices } = await (await fetch("/api/devices")).json();
      if (devices.length) udid = devices[0].udid;
    } catch {}
  }
  if (wantReset) await resetApp();
  if (wantVideo) connectVideo();
  connectInput();
  // Unscoped trees follow the frontmost app (the runner resolves it);
  // ?app= narrows to one bundle when tests want isolation.
  syncTree();
}
start();
