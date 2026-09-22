---
name: devicedeck-flows
description: Use when working with DeviceDeck Flow Capture — recording a manual session on a simulator/emulator into a Maestro flow with durable selectors, reviewing a captured flow, or replaying it with maestro-runner so it runs unchanged on real hardware.
metadata:
  short-description: Capture, review and replay DeviceDeck Maestro flows
---

# Flow Capture and replay

Record a manual session and it becomes a **Maestro flow with durable selectors**, so the same file
replays on real hardware — the path from "I did this by hand" to "this is a test" with no rewrite.

## Capturing

Recording happens in the console, not through an MCP tool: open `http://127.0.0.1:8787`, pick a
device, put the app's bundle id (or pick a `--app` build) in the sidebar, press **Record**, use the
app, press **Stop**. The flow appears in the right-hand panel with **Copy** and **Download**.

- The flow starts with `launchApp: clearState: true`, so a replay begins from the app's first-run
  screen, as the capture did.
- Each tap is addressed by the most durable selector on screen: an accessibility **id** first, then
  **text**, narrowed with `childOf` or `index` when several elements match. A tap on nothing
  addressable falls back to a coordinate `tapOn: point`, marked low confidence — a sign the screen
  needs an identifier, not something to keep.
- Every step is preceded by a `# devicedeck:` comment naming its selector and its confidence
  (`high` id, `medium` text or `childOf`, `low` index or coordinate). Maestro ignores comments.
- **Right-click an element while recording** to add a check instead of a step: **Assert visible**
  (`assertVisible`) or **Wait until visible** (`extendedWaitUntil`, 10 s — for spinners and slow
  network). A check on nothing addressable is refused rather than recorded as a coordinate.
- Text typed into a **password field** is never written into the flow: the step reads
  `inputText: ${PASSWORD_INPUT}` (named after the field's id), and a comment at the top lists the
  `-e` values a replay needs.
- Screens with actionable elements and no durable ids are flagged in a `# desert:` comment at the
  top, with how to add identifiers for the app's framework.

## Reviewing

Before trusting a flow, read the `# devicedeck:` grades. `high` steps will survive the next build;
`low` ones (index, coordinate) will not — add an accessibility identifier in the app and re-capture
rather than hand-editing the selector.

## Replaying

A captured flow is a stock Maestro flow. Replay it with maestro-runner and its `devicelab` driver,
supplying any secrets the header lists:

```bash
maestro-runner --driver devicelab test flow.yaml -e PASSWORD_INPUT=…
```

## The contract

A captured flow must run **unchanged** on devicelab.dev real devices — identical format, same
selectors. If a flow needs editing to replay elsewhere, the selectors were not durable: re-capture
against elements that carry accessibility identifiers.
