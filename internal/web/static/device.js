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
}

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
  // The device's own contents, kept even while the field holds focus (when
  // the write below is suppressed) so the edit chain can read back what the
  // device actually received and repair any drift. A secure field reports
  // bullets, so its read-back is by length; mark it.
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
  // A held response is the settled screen by construction — the server
  // verified it — so the barrier closes here rather than after three
  // more polls, and whoever was waiting on it is released.
  if (held) {
    quietPolls = SETTLE_POLLS;
    const waiters = settledWaiters;
    settledWaiters = [];
    for (const resolve of waiters) resolve();
  }
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
    x: (clientX - rect.left + mirror.scrollLeft) / rect.width,
    y: (clientY - rect.top + mirror.scrollTop) / rect.height,
  };
}

function onDevice(p) {
  return p.x >= 0 && p.x <= 1 && p.y >= 0 && p.y <= 1;
}

// Clicks land wherever the pointer is — the mirror node under it exists
// for *finding*; the device receives the true coordinates, exactly like
// a finger.
let pointerDown = false;
// A click on a row below the fold is taken over entirely: the device is
// scrolled and the row tapped once it is on screen. The up that follows
// belongs to that click and must not become a stray touch.
let pointerDeferred = false;
mirror.addEventListener("pointerdown", (e) => {
  // Stamped on the way down, not up: focus lands on pointerdown, so the
  // focus handler must already be able to see that a click caused it.
  pointerAt = Date.now();
  const at = contentPoint(e.clientX, e.clientY);
  if (!onDevice(at)) {
    pointerDeferred = true;
    tapOffscreen(e.target, at);
    return;
  }
  pointerDown = true;
  // Synthetic events (Cypress, jsdom) may carry no capturable pointerId;
  // capture is an optimization for drags, never a precondition.
  try { mirror.setPointerCapture(e.pointerId); } catch {}
  input.send(touchFrame(PHASE.down, at.x, at.y));
  noteActivity();
});
mirror.addEventListener("pointermove", (e) => {
  if (!pointerDown) return;
  const { x, y } = normalized(e);
  input.send(touchFrame(PHASE.move, x, y));
});
mirror.addEventListener("pointerup", (e) => {
  if (pointerDeferred) {
    pointerDeferred = false;
    return;
  }
  if (!pointerDown) return;
  pointerDown = false;
  pointerAt = Date.now();
  // A click can focus a field (Cypress/Puppeteer click then type through
  // the keyboard, never fill()), so this tap arms the typing settle too.
  focusTapAt = Date.now();
  const at = contentPoint(e.clientX, e.clientY);
  const { x, y } = normalized(e);
  // A drag that runs past the edge ends at the edge; a click's up lands
  // where its down did.
  input.send(touchFrame(PHASE.up, onDevice(at) ? at.x : x, onDevice(at) ? at.y : y));
  noteActivity();
});

// ---------- scrolling ----------

// A wheel event becomes a finger drag. This is what page.mouse.wheel(),
// Puppeteer's mouse.wheel(), Selenium's scroll actions and a trackpad
// in the console all produce, and without it none of them did anything
// — the mirror has no scrollable box, so the event fell on the page and
// the device never heard of it. Tools that write scrollTop directly
// (Cypress's scrollTo, scrollIntoView before a click) dispatch no event
// and are a separate problem.
//
// A trackpad emits a burst of small deltas; they are coalesced over a
// short window into one gesture rather than one drag per tick. The
// drag is deliberately slow and ends with the finger held still: a
// quick flick keeps scrolling after it lifts, and "scroll by 300 pixels"
// would land somewhere different every run.
const WHEEL_COALESCE_MS = 40;
const DRAG_STEPS = 12;
const DRAG_STEP_MS = 16;
const DRAG_HOLD_MS = 90;
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

// flushWheel turns the accumulated delta into one drag under the
// pointer — or from the device's centre when the pointer is off the
// device, which is where a page-level scroll would act anyway.
function flushWheel() {
  const { x: dx, y: dy, clientX, clientY } = wheelAccum;
  wheelAccum = { x: 0, y: 0, clientX: 0, clientY: 0 };
  const rect = canvas.getBoundingClientRect();
  const inside =
    clientX >= rect.left && clientX <= rect.right && clientY >= rect.top && clientY <= rect.bottom;
  const from = inside
    ? { x: clientX, y: clientY }
    : { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
  // Wheel down means the content moves up, which is a finger moving up.
  dragGesture(from, { x: from.x - dx, y: from.y - dy });
}

// The mirror's scroll offset is a view transform, never a gesture. It
// is put back to zero when the screen changes, because the tree that
// arrives then is laid out against the real screen, and a stale offset
// would shift every click on it.
function resetScroll() {
  if (mirror.scrollTop || mirror.scrollLeft) mirror.scrollTo(0, 0);
}

// settledRender resolves the next time a held tree — the settled screen
// after an action — has been rendered.
let settledWaiters = [];
function settledRender() {
  return new Promise((resolve) => settledWaiters.push(resolve));
}

// How far down the screen a row is brought when the device has to be
// scrolled to reach it: clear of the bottom edge, clear of any tab bar.
const REVEAL_AT = 0.75;
const REVEAL_TRIES = 4;

// tapOffscreen taps a row the mirror holds below the device's edge. The
// device is dragged until the row is on screen, then it is tapped where
// the settled tree says it now is. Identity survives by key — that is
// what the reconciliation keeps stable — so it is the same row even
// though every frame on screen has changed. A tool's click returns
// before any of this lands, which is already how typing works here:
// fill() returns when the mirror has the text and the keystrokes follow.
async function tapOffscreen(el, at) {
  const key = el.closest("[data-dd-key]")?.getAttribute("data-dd-key");
  if (!key) return;
  for (let i = 0; i < REVEAL_TRIES; i++) {
    const rect = canvas.getBoundingClientRect();
    const from = { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
    const dy = (at.y - REVEAL_AT) * rect.height;
    const dx = (at.x > 1 ? at.x - 0.5 : at.x < 0 ? at.x - 0.5 : 0) * rect.width;
    dragGesture(from, { x: from.x - dx, y: from.y - dy });
    await settledRender();
    resetScroll();
    const found = mirror.querySelector(`[data-dd-key="${CSS.escape(key)}"]`);
    if (!found) return;
    at = centreOf(found);
    if (onDevice(at)) break;
  }
  if (!onDevice(at)) return;
  input.send(touchFrame(PHASE.down, at.x, at.y));
  input.send(touchFrame(PHASE.up, at.x, at.y));
  noteActivity();
}

// dragGesture moves a finger from one page point to another in timed
// steps, then lets go. Points are clamped to the device, so a drag
// that would run off the edge scrolls as far as the edge allows.
function dragGesture(from, to) {
  const at = (p) => normalized({ clientX: p.x, clientY: p.y });
  const start = at(from);
  const end = at(to);
  // Opened here as well as on release: a click aimed at a row while the
  // finger is still moving would land on bounds the screen has left.
  noteActivity();
  input.send(touchFrame(PHASE.down, start.x, start.y));
  for (let i = 1; i <= DRAG_STEPS; i++) {
    const t = i / DRAG_STEPS;
    setTimeout(() => {
      input.send(touchFrame(PHASE.move, start.x + (end.x - start.x) * t, start.y + (end.y - start.y) * t));
    }, i * DRAG_STEP_MS);
  }
  setTimeout(() => {
    input.send(touchFrame(PHASE.up, end.x, end.y));
    noteActivity();
  }, DRAG_STEPS * DRAG_STEP_MS + DRAG_HOLD_MS);
}

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
  const at = centreOf(el);
  // The settle before typing is timed from here — the tap that focuses
  // the field — whether it lands on-screen or is scrolled into view.
  focusTapAt = Date.now();
  if (!onDevice(at)) return tapOffscreen(el, at);
  input.send(touchFrame(PHASE.down, at.x, at.y));
  input.send(touchFrame(PHASE.up, at.x, at.y));
  return Promise.resolve();
}

// centreOf is an element's centre in content space.
function centreOf(el) {
  const r = el.getBoundingClientRect();
  return contentPoint(r.left + r.width / 2, r.top + r.height / 2);
}

// After a focus tap the device raises its keyboard and moves the caret,
// which takes from tens of ms to most of a second under load. A keystroke
// sent inside that window lands on whatever was focused before — fill()
// on one field then the next bled the second field's first character into
// the first ("devicelab" then a password reached the device as
// "devicelabr"). Wait it out, timed from the focus tap so the cost falls
// once per field and not once per keystroke: browser_type's later
// characters, already past the window, send immediately, and fill()'s one
// event waits once.
//
// Where the platform reports which node holds focus (data-dd-focused),
// that ends the wait the moment focus actually lands — load-independent.
// iOS reports no focus state at all (measured: no focus flag, and no tree
// change whatsoever when focus moves between two fields with the keyboard
// already up), so there the cap is the whole wait, sized to cover a focus
// switch on a loaded device. A field that never reports focus is sent to
// after the cap rather than hanging the chain — the 5s cap that replaced
// this stalled browser_type, whose every character then waited the full
// timeout because iOS never set the flag.
const FOCUS_SETTLE_MS = 750;
function waitReady(el) {
  return new Promise((resolve) => {
    const tick = () => {
      if (el.getAttribute("data-dd-focused") === "true" ||
          Date.now() - focusTapAt >= FOCUS_SETTLE_MS) resolve();
      else setTimeout(tick, 30);
    };
    tick();
  });
}
// A click focuses the field on its way through, and the pointer handlers
// have already tapped the device at that point. Tapping again on focus
// would make it a double tap, which selects a word instead of placing a
// caret — measured as scrambled text when a test clicked and then typed.
const POINTER_FOCUS_MS = 600;
let pointerAt = 0;
// When the field last received a focus tap, on either path: fill()/
// browser_type through the focusin handler's tapField, or a click through
// the pointer handler. waitReady above times the settle from it.
let focusTapAt = 0;

// Edits run one at a time, in order. Each used to wait out the settle
// window on its own timer, and because those delays differed, a later
// keystroke could fire before an earlier one — "devicelab" reached the
// device as "vxdicelab". A chain keeps the order the typist produced.
//
// The focus tap is on the same chain, for the same reason one step
// further out: fill() on one field and then fill() on the next fires
// two focus taps back to back while the first field's keystrokes are
// still waiting out their settle window. Tapped immediately, the second
// field had focus by the time the first field's text arrived, and both
// values landed in it — eighteen bullets in the password field. A tap
// that queues behind the pending edits lands after them, as typed.
let editChain = Promise.resolve();

mirror.addEventListener("focusin", (e) => {
  if (!(e.target instanceof HTMLInputElement)) return;
  if (Date.now() - pointerAt < POINTER_FOCUS_MS) return;
  const el = e.target;
  editChain = editChain.then(() => tapField(el));
  noteActivity();
});

mirror.addEventListener("input", (e) => {
  const el = e.target;
  if (!(el instanceof HTMLInputElement)) return;
  const before = el.dataset.ddValue ?? "";
  const after = el.value;
  el.dataset.ddValue = after;
  // What the caller wants in this field, kept apart from ddValue because a
  // poll overwrites ddValue with the device's contents once focus leaves.
  // The reconcile below reads it back against the device to catch a bleed.
  el.dataset.ddIntended = after;
  // First edit of this field: the value shown is the device's own, which on
  // Android is the field's hint, not its content — an empty EditText reports
  // its hint as its value. Prefix-diffing the typed text against a hint drops
  // any shared leading character ("1 Market St" against a hint starting "1"
  // lost its "1", disabling the form), so the first edit types the whole
  // value; later keystrokes, now against real content, diff as usual.
  const firstEdit = !editedFields.has(el);
  editedFields.add(el);
  editChain = editChain.then(async () => {
    await waitReady(el);
    sendEdit(before, after, firstEdit);
  });
  scheduleReconcile();
  noteActivity();
});

// Read-back reconcile. iOS reports no keyboard focus, so a keystroke sent
// before a focus tap has landed bleeds into the previously focused field —
// "devicelab" then a password reaching the device as "devicelabr" — and no
// wall-clock prevents it on a loaded device (the guess is browser time, the
// device runs on its own). So do not guess when to type; verify what
// landed. After typing settles, check the device echoed each field's
// intended value and retype any it did not — device truth, not a timer, so
// it holds under any load. Bounded, so a field the device will never accept
// does not spin forever.
const editedFields = new Set();
const RECONCILE_MS = 250;
const MAX_REPAIRS = 4;
let reconcileTimer = 0;

// fieldMatches reports whether the device holds what the caller asked for.
// A secure field only reports bullets, so it is matched by length.
// isMasked reports whether a device value is a secure field's bullets and
// nothing else, so it can only be compared by length. iOS marks the field
// type SecureTextField (ddSecure); Android reports a password EditText as a
// plain TextField but still masks the text, so the value itself is the only
// signal there — without this the read-back compares bullets against the
// plaintext, never matches, and "repairs" the password until it is empty.
function isMasked(s) {
  return s.length > 0 && [...s].every((c) => c === "•");
}

function fieldMatches(el) {
  const intended = el.dataset.ddIntended ?? "";
  const device = el.dataset.ddDeviceValue ?? "";
  const secure = el.dataset.ddSecure === "true" || isMasked(device);
  return secure ? device.length === intended.length : device === intended;
}

// scheduleReconcile queues one verify pass for after typing goes quiet, so
// a burst of keystrokes reconciles once rather than once per key.
function scheduleReconcile() {
  clearTimeout(reconcileTimer);
  reconcileTimer = setTimeout(() => {
    editChain = editChain.then(reconcileFields);
    noteActivity();
  }, RECONCILE_MS);
}

// reconcileFields brings every edited field to the caller's value, re-
// tapping and retyping any the device did not echo. It waits on a settled
// tree so it reads fresh device contents, and gives up after MAX_REPAIRS.
// fieldsSignature is the edited fields' device values joined by a space, so
// one settled read can be compared against the next.
function fieldsSignature() {
  return [...editedFields].map((el) => el.dataset.ddDeviceValue ?? "").join(" ");
}

// settledFields waits for a settled screen whose edited-field values have
// also stopped changing. Keystrokes are sent fire-and-forget and the device
// applies them one ~100ms HID hold at a time, so the first settled tree
// after a burst can arrive with the value still climbing — the last keys
// still in the pipe. Judging drift then reads a half-typed field and
// retypes onto keys not yet applied, doubling it ("robustest" as eighteen
// bullets). Requiring the value to repeat across settled reads waits the
// pipe out on device truth, not a wall-clock guess. Bounded: a value the
// device keeps changing on its own does not wedge the loop.
const STABLE_READS = 5;
async function settledFields() {
  let prev = null;
  for (let i = 0; i < STABLE_READS; i++) {
    // Trigger the held poll settledRender waits on, rather than relying on
    // one already being in flight — a focus switch produces no tree change,
    // so the scheduling poll can settle and drain before this runs, and the
    // await would then hang, blocking the whole edit chain until the next
    // unrelated action happened to note activity.
    noteActivity();
    await settledRender();
    const sig = fieldsSignature();
    if (sig === prev) return;
    prev = sig;
  }
}

async function reconcileFields() {
  for (let attempt = 0; attempt < MAX_REPAIRS; attempt++) {
    await settledFields();
    const drifted = [...editedFields].filter((el) => el.isConnected && !fieldMatches(el));
    for (const el of editedFields) {
      if (!el.isConnected) editedFields.delete(el);
    }
    if (!drifted.length) return;
    for (const el of drifted) {
      await tapField(el);
      await waitReady(el);
      await repairField(el);
    }
  }
}

// repairField clears whatever the device currently holds in el and types
// the caller's value fresh — a rewrite, not a diff, because a secure
// field's device contents are bullets that cannot be diffed against. The
// clear is confirmed against the device before anything is typed: a
// backspace can bleed under load too, and a value typed on top of one the
// clear failed to remove doubles the field ("devicelabdevicelab"). If the
// clear did not take, this returns and the reconcile loop retries the tap.
async function repairField(el) {
  const deviceLen = (el.dataset.ddDeviceValue ?? "").length;
  for (let n = 0; n < deviceLen + 2; n++) input.send(keyFrame(0, KEY_USAGE.Backspace));
  noteActivity();
  await settledRender();
  if ((el.dataset.ddDeviceValue ?? "").length > 0) return;
  for (const ch of el.dataset.ddIntended ?? "") {
    const frame = keyFrameForChar(ch);
    if (frame) input.send(frame);
  }
}

// sendEdit turns a value change into the keystrokes that would produce
// it: backspaces for what was removed, characters for what was added.
// fill() replaces the whole value in a single event, so diffing is the
// only way to know what the device actually has to be told.
function sendEdit(before, after, firstEdit) {
  // Shared leading characters are already on the device and are not retyped —
  // except on a field's first edit, where `before` may be a placeholder hint
  // rather than real content, so nothing is treated as shared.
  let shared = 0;
  if (!firstEdit) {
    while (shared < before.length && shared < after.length &&
           before[shared] === after[shared]) {
      shared++;
    }
  }
  for (let n = before.length - shared; n > 0; n--) {
    input.send(keyFrame(0, KEY_USAGE.Backspace));
  }
  for (const ch of after.slice(shared)) {
    const frame = keyFrameForChar(ch);
    if (frame) input.send(frame);
  }
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

window.devicedeck = {
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
