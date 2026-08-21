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

// Roles that represent something a person taps. Only these carry their
// identifier in the name.
const CONTROL_ROLES = new Set([
  "button", "link", "textbox", "searchbox", "checkbox", "radio", "switch", "tab",
]);

// baseName is a node's own name before an identifier is folded in. A
// native container repeats this string, not the decorated one, so it is
// what an ancestor's name has to be compared against.
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

function setOrRemove(el, attr, value) {
  if (value) el.setAttribute(attr, value);
  else el.removeAttribute(attr);
}

// syncNode positions el inside its rendered ancestor: node frames are
// screen-absolute, so coordinates convert to percentages of the
// ancestor's frame and stay proportional at any canvas size.
function syncNode(el, node, anchorFrame, owners) {
  const f = node.frame;
  el.style.left = `${((f.x - anchorFrame.x) / anchorFrame.width) * 100}%`;
  el.style.top = `${((f.y - anchorFrame.y) / anchorFrame.height) * 100}%`;
  el.style.width = `${(f.width / anchorFrame.width) * 100}%`;
  el.style.height = `${(f.height / anchorFrame.height) * 100}%`;

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
  // An <input> holds its contents in .value and can have no children, so
  // it takes neither the text node below nor anything nested.
  if (el.tagName === "INPUT") {
    syncField(el, node);
    return;
  }
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

// syncField mirrors the device's own contents into the input — except
// while it holds focus. The tree lags a keystroke behind, so writing its
// value back mid-edit would delete characters as fast as they are typed.
// ddValue records what the device has already been told, and is the
// baseline the next edit is diffed against.
function syncField(el, node) {
  const value = node.value || "";
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
  if (el.tagName === "INPUT") {
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
  // Append-or-move keeps DOM order tracking native order; moving
  // (including across parents) preserves element identity.
  anchor.el.appendChild(el);
  const rendered = { el, frame: node.frame, key, lastChild: null };
  anchor.lastChild = rendered;
  pass.anchors.set(node.index, rendered);
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
function nameScreen(hash) {
  mirror.setAttribute("aria-label", hash ? `device screen ${hash.slice(0, 8)}` : "device");
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
      nameScreen(payload.hash);
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
  // Stamped on the way down, not up: focus lands on pointerdown, so the
  // focus handler must already be able to see that a click caused it.
  pointerAt = Date.now();
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
  pointerAt = Date.now();
  const { x, y } = normalized(e);
  input.send(touchFrame(PHASE.up, x, y));
  noteActivity();
});

document.addEventListener("keydown", (e) => {
  // A focused mirror field types itself: the browser writes the character
  // into the <input>, the input listener below diffs the value and sends
  // the keystroke. Taking this path too would send it twice, and the
  // preventDefault would stop the value ever changing in the first place.
  // Keys that leave the value alone still belong here.
  if (editsAField(e)) return;
  const frame = keyEventFrame(e);
  if (!frame) return;
  e.preventDefault();
  input.send(frame);
  noteActivity();
});

// ---------- typing into a mirrored field ----------

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

// tapField taps the device where el sits. Focus is not enough on its own:
// fill() and browser_type focus the element and write the value without
// ever clicking, so the device's own field would still be unfocused and
// every keystroke would land on whatever was focused before.
function tapField(el) {
  const r = el.getBoundingClientRect();
  const m = mirror.getBoundingClientRect();
  const x = (r.left + r.width / 2 - m.left) / m.width;
  const y = (r.top + r.height / 2 - m.top) / m.height;
  input.send(touchFrame(PHASE.down, x, y));
  input.send(touchFrame(PHASE.up, x, y));
}

// The device needs a moment to raise its keyboard and place the caret
// after that tap. Text sent inside the window would be typed into a field
// that is not yet listening, so an edit that follows a focus waits it out.
const FOCUS_SETTLE_MS = 400;
// A click focuses the field on its way through, and the pointer handlers
// have already tapped the device at that point. Tapping again on focus
// would make it a double tap, which selects a word instead of placing a
// caret — measured as scrambled text when a test clicked and then typed.
const POINTER_FOCUS_MS = 600;
let focusTapAt = 0;
let pointerAt = 0;

mirror.addEventListener("focusin", (e) => {
  if (!(e.target instanceof HTMLInputElement)) return;
  if (Date.now() - pointerAt < POINTER_FOCUS_MS) return;
  tapField(e.target);
  focusTapAt = Date.now();
  noteActivity();
});

// Edits run one at a time, in order. Each used to wait out the settle
// window on its own timer, and because those delays differed, a later
// keystroke could fire before an earlier one — "devicelab" reached the
// device as "vxdicelab". A chain keeps the order the typist produced.
let editChain = Promise.resolve();

mirror.addEventListener("input", (e) => {
  const el = e.target;
  if (!(el instanceof HTMLInputElement)) return;
  const before = el.dataset.ddValue ?? "";
  const after = el.value;
  el.dataset.ddValue = after;
  editChain = editChain.then(async () => {
    const wait = Math.max(0, FOCUS_SETTLE_MS - (Date.now() - focusTapAt));
    if (wait > 0) await new Promise((r) => setTimeout(r, wait));
    sendEdit(before, after);
  });
  noteActivity();
});

// sendEdit turns a value change into the keystrokes that would produce
// it: backspaces for what was removed, characters for what was added.
// fill() replaces the whole value in a single event, so diffing is the
// only way to know what the device actually has to be told.
function sendEdit(before, after) {
  let shared = 0;
  while (shared < before.length && shared < after.length &&
         before[shared] === after[shared]) {
    shared++;
  }
  for (let n = before.length - shared; n > 0; n--) {
    input.send(keyFrame(0, KEY_USAGE.Backspace));
  }
  for (const ch of after.slice(shared)) {
    const frame = keyFrameForChar(ch);
    if (frame) input.send(frame);
  }
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
