---
name: devicedeck-flows
description: Use when working with DeviceDeck Flow Capture — a person recording a manual session on a simulator/emulator in the DeviceDeck console into a Maestro flow, reviewing and improving a captured flow, or replaying it with maestro-runner so it runs unchanged on real hardware. For a flow the agent writes itself, use devicedeck-maestro.
metadata:
  short-description: Capture, review and replay DeviceDeck Maestro flows
---

# Flow Capture and replay

Record a manual session and it becomes a **Maestro flow with durable selectors**, so the same file
replays on real hardware — the path from "I did this by hand" to "this is a test" with no rewrite.

## Capturing

A person records in the console: open `http://127.0.0.1:8787`, pick a device, put the app's
bundle id (or pick a `--app` build) in the sidebar, press **Record**, use the app, press **Stop**.
The flow appears in the right-hand panel with **Copy** and **Download**. There is no MCP tool to
record; an agent that should produce a flow itself writes it step by step with
`devicedeck-maestro`.

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
- A **location** set from the sidebar's **Settings** while recording (or through the API or the
  `device_settings` tool) is recorded as `setLocation`. Dark mode and permissions are not
  recorded — a Maestro flow has no step for them — so note them for whoever runs the flow.

## Reviewing

Before trusting a flow, read the `# devicedeck:` grades. `high` steps will survive the next build;
`low` ones (index, coordinate) will not.

To improve a weak step, open the device at that screen and `snapshot` it:
- the element does carry an id or unique text the recording missed → change the step to it, and
  check it with `find_element` (one match) before saving;
- it has neither → the app needs an accessibility identifier on that element (the `# desert:`
  comment says how for its framework); add it and re-capture that part.

Never swap a weak selector for a coordinate, and keep `# devicedeck:` comments in step with any
step you change.

## Replaying

A captured flow is a stock Maestro flow. Run it with maestro-runner exactly as
`devicedeck-maestro` → *Save and run* describes: check `maestro-runner --version` (ask the user to
install it if it is missing), free the device from DeviceDeck, and pass `--driver devicelab`,
supplying any secrets the header lists with `-e`, before the flow path:

```bash
maestro-runner --platform ios --device <udid> --driver devicelab test -e PASSWORD_INPUT=… flow.yaml
```

## The contract

A captured flow must run **unchanged** on devicelab.dev real devices — identical format, same
selectors. If a flow needs editing to replay elsewhere, the selectors were not durable: re-capture
against elements that carry accessibility identifiers.
