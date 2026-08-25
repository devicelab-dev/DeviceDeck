---
name: devicedeck-triage
description: Use when a mobile test driven through DeviceDeck failed or is behaving unexpectedly — to see the device's actual state, confirm what is and is not on screen, and classify the failure with evidence rather than guessing.
metadata:
  short-description: Diagnose a failing DeviceDeck mobile test
---

# Triaging a failing mobile test

Decide from the device's own state, not from the test log alone. DeviceDeck exposes both the
structure and the pixels.

## Workflow

1. **Reproduce to the failing point**, then look before concluding:
   - `mcp__devicedeck__ui_tree` — the exact elements on screen, with identifiers and values.
   - `mcp__devicedeck__screenshot` — the rendered screen, for anything structure can't show
     (a spinner, a rendered image, an off-screen layout).
2. **Confirm the expectation** — `mcp__devicedeck__assert_visible` for the element or text the
   test waited on. "Not visible" tells you the app never reached that state; "visible" points at
   a timing or selector problem instead.
3. **Classify:**
   - *App never advanced* — a control was disabled, or a previous step's input did not land
     (check the field's `value` in the tree — is it what the test typed?).
   - *Element not found* — a selector that no longer resolves (identifier changed, or the wrong
     screen), or, on Android, an intermittent driver-start crash (retry the launch).
   - *Wrong screen* — the app resumed a logged-in session; relaunch fresh with `launch_app`.
   - *Timing* — the value is right on the device but the test asserted too early; wait for the
     device to echo it.

## Report

State the failure with the evidence that decided it — the tree fragment or screenshot, the
assertion result, the field value seen — so the fix targets the real cause, not a symptom.
