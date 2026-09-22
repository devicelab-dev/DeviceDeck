// Colouring for the Maestro YAML that Flow Capture writes. It is not a
// general YAML highlighter: DeviceDeck writes this file itself, so its shape
// is small and fixed — comments, provenance notes, keys, quoted strings,
// ${VAR} secrets, numbers, list steps — and every line of it is coloured
// exactly. It also colours what only this format carries: each step's
// selector confidence, graded as the recorder grades it, and the secrets a
// replay has to be given.
"use strict";

// One line of the flow: indent, an optional list dash, an optional key,
// then the value.
const FLOW_LINE = /^(\s*)(- )?(?:([A-Za-z_][\w.-]*)(:))?(.*)$/;
const FLOW_SECRET = /\$\{[A-Za-z_]\w*\}/g;

// flowTokens splits one line into [class, text] pairs; class "" is plain.
function flowTokens(line) {
  if (/^\s*#/.test(line)) return commentTokens(line);
  if (line === "---") return [["y-rule", line]];
  const [, indent, dash, key, colon, rest] = FLOW_LINE.exec(line);
  const out = [["", indent]];
  if (dash) out.push(["y-dash", dash]);
  // A key right after a dash is the step's command: tapOn, assertVisible.
  if (key) out.push([dash ? "y-command" : "y-key", key], ["y-punct", colon]);
  return out.concat(valueTokens(rest));
}

// commentTokens colours a comment: a provenance note's confidence by its
// grade, a selector desert as a warning, anything else dimmed.
function commentTokens(line) {
  if (/^\s*# desert/.test(line)) return [["y-desert", line]];
  const grade = /confidence=(high|medium|low)/.exec(line);
  if (!grade) return [["y-comment", line]];
  const at = grade.index + "confidence=".length;
  return [
    ["y-comment", line.slice(0, at)],
    [`y-grade-${grade[1]}`, grade[1]],
    ["y-comment", line.slice(at + grade[1].length)],
  ];
}

// valueTokens colours a value: a quoted string, a number or boolean, and
// any ${VAR} secret wherever it appears.
function valueTokens(value) {
  const lead = value.match(/^\s*/)[0];
  const body = value.slice(lead.length);
  let kind = "";
  if (body.startsWith('"')) kind = "y-string";
  else if (/^(-?\d+(\.\d+)?|true|false)$/.test(body)) kind = "y-const";
  return [["", lead], ...splitSecrets(body, kind)];
}

function splitSecrets(text, kind) {
  const out = [];
  let last = 0;
  for (const m of text.matchAll(FLOW_SECRET)) {
    out.push([kind, text.slice(last, m.index)], ["y-secret", m[0]]);
    last = m.index + m[0].length;
  }
  out.push([kind, text.slice(last)]);
  return out.filter(([, t]) => t);
}

// startsStep reports whether a line opens a step: its provenance note, or
// the step itself when it has none. The panel spaces steps apart there.
function startsStep(line, prev) {
  if (/^# devicedeck: /.test(line)) return true;
  return /^- /.test(line) && !/^# devicedeck: /.test(prev);
}

// highlightFlow renders a whole flow as coloured lines, ready for a <pre>.
function highlightFlow(yaml) {
  const frag = document.createDocumentFragment();
  let desert = false;
  let prev = "";
  for (const line of yaml.replace(/\n$/, "").split("\n")) {
    // A wrapped comment continues on "#   " lines; a desert warning stays
    // a warning to its last line.
    desert = /^\s*# desert/.test(line) || (desert && /^\s*# {3}/.test(line));
    const row = Object.assign(document.createElement("span"), { className: "y-line" + (startsStep(line, prev) ? " y-step" : "") });
    for (const [cls, text] of desert ? [["y-desert", line]] : flowTokens(line)) {
      if (!text) continue;
      row.appendChild(cls ? Object.assign(document.createElement("span"), { className: cls, textContent: text })
        : document.createTextNode(text));
    }
    // An empty line still takes its height; blocks carry the line breaks.
    if (!row.childNodes.length) row.textContent = " ";
    frag.appendChild(row);
    prev = line;
  }
  return frag;
}

if (typeof module !== "undefined") module.exports = { flowTokens };
