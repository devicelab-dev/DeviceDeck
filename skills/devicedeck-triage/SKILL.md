---
name: devicedeck-triage
description: Use when a mobile test driven through DeviceDeck — a Playwright spec or a Maestro flow — failed or is behaving unexpectedly, to see the device's actual state, confirm what is and is not on screen, and classify the failure with evidence rather than guessing.
metadata:
  short-description: Diagnose a failing DeviceDeck mobile test or Maestro flow
---

# Triaging a failing mobile test

Decide from the device's own state, not the test log alone. The failure is almost always a
**device or app behaviour, not a web one**, so weigh it against the device facts below.

## Look before concluding

Reproduce to the failing point on the device, then read it. With DeviceDeck's MCP tools:

| Question | Tool |
|---|---|
| What is on screen, and how is each element addressed? | `snapshot` (`mode: "full"` adds plain text) |
| What changed since the last look? | `snapshot` with `mode: "diff"` |
| Does this selector match, and how many elements? | `find_element` (by `testid`, `text`, `role` + `name`) |
| Does it appear or go away if I wait? | `wait_for` with `state: "visible"` / `"gone"` / `"settled"` |
| What does it look like (spinner, image, layout)? | `screenshot` |
| Every attribute of every element | `ui_tree` (large; saved to a file when it is too big) |

With Playwright MCP on the device page, the same facts are in the page snapshot: each element's
`data-testid`, role and name, and a field's device value in `data-dd-device-value`.

## Classify — the failures a web test never hits

- **Element not found.** Check with `find_element` whether the selector matches anything:
  - no match on this screen → the app is on a different screen (look at the snapshot) or the
    identifier changed in the app;
  - several matches → the test acted on the wrong one; narrow the selector;
  - a Maestro `id:` / `text:` is a **whole-value regular expression**: an unescaped `.`, `(`, `$`
    or `?` in the label, or a partial label, will not match;
  - on Android, an intermittent driver-start crash also reads as "not found" — relaunch and retry.
- **"Could not read the device's screen."** The engine is busy or restarting. It is not the
  element being absent — retry before concluding anything.
- **Present but disabled.** The screen was still settling (a Sign In button enabled only once both
  fields are filled). Fill the form, then look again.
- **Off screen or under the keyboard.** The element exists but a tap on it lands elsewhere. Scroll
  it into view, or press Enter / dismiss the keyboard first.
- **Under the status bar (Android 15+).** The app draws the element behind the system's status bar
  (an edge-to-edge layout it does not inset), and the system takes every tap there — a finger could
  not tap it either. Report it as an app bug: the layout needs the status-bar inset.
- **A system alert over the app.** An `alertdialog` in the snapshot (a permission prompt) blocks
  every action behind it. Grant the permission up front (`device_settings`, or `?grant=` on the
  device page), or answer it in the test (`page.addLocatorHandler`).
- **Input never landed, or changed.** Compare the field's device value with what the test typed.
  `fill()` returns only once the device holds the value, so a mismatch after it means the app
  changed it — a length limit, an input mask, autocorrect — not a race. A test that uses
  `keyboard.type` instead can lose its next tap to Android's soft keyboard: switch it to `fill()`.
- **Wrong screen at the start.** The app resumed a logged-in session. Launch it fresh — a launch
  clears its data by default; a Maestro flow starts with `launchApp: clearState: true`.
- **Device held by another client.** Input is refused while another tab, test run or agent drives
  the device (the page says who). One driver per device: close the other, or use another device.
- **Timing.** The value is right on the device but the test asserted too early. Assert with
  `expect` (Playwright) or `extendedWaitUntil` (Maestro), which retry — never a fixed wait.

## A failing Maestro flow

maestro-runner prints its report folder; the failing step and a screenshot of the screen at the
failure are there. Reproduce up to that step on the device, then check the step's selector with
`find_element` as above. Fix the step, not the timing: a flow that needs a longer sleep is
waiting on something `extendedWaitUntil` should name.

## Report

State the failure with the evidence that decided it — the snapshot lines, the `find_element`
result, the screenshot, the field's device value — so the fix targets the real cause, not a
symptom.
