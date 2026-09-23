---
name: devicedeck-triage
description: Use when a mobile test driven through DeviceDeck failed or is behaving unexpectedly — to see the device's actual state, confirm what is and is not on screen, and classify the failure with evidence rather than guessing.
metadata:
  short-description: Diagnose a failing DeviceDeck mobile test
---

# Triaging a failing mobile test

Decide from the device's own state, not the test log alone. The device is a web page, so
inspect it with the same browser tools you drove it with — but the failure is almost always a
**device/app behaviour, not a web one**, so weigh it against the device facts below.

## Workflow

1. **Reproduce to the failing point**, navigate to the device page, then look before concluding:
   - **Snapshot** the page — the exact elements on screen, each with its `data-testid`, role,
     and value.
   - **Screenshot** it — for anything structure can't show (a spinner, a rendered image, an
     off-screen layout).
2. **Confirm the expectation** — look in the snapshot for the element or text the test waited
   on. Absent → the app never reached that state; present → a timing or selector problem.
3. **Classify — these are the device/app failures a web test never hits:**
   - *App never advanced* — a control was disabled, or a step's input did not land. Check the
     field's `data-dd-device-value` in the snapshot (the device's read-back): is it what the test
     filled? `fill()` returns only once the device holds the value, so a mismatch means the app
     changed it — a length limit, an input mask, autocorrect — not a race. A test that uses
     `keyboard.type` instead can lose its next tap to Android's soft keyboard: switch it to `fill()`.
   - *Element not found* — a selector that no longer resolves (identifier changed, or the wrong
     screen), or, on Android, an intermittent driver-start crash (retry the launch).
   - *Wrong screen* — the app resumed a logged-in session instead of a first-run screen;
     relaunch fresh (a fresh launch wipes app data by default).
   - *Timing* — the device is right but the test checked once, too early (an app that updates on
     a timer, such as a debounced search). Assert with `expect`, which retries, not a one-shot read
     or a `waitForTimeout`.

## Report

State the failure with the evidence that decided it — the snapshot fragment or screenshot, the
element's presence, the field's device value — so the fix targets the real cause, not a symptom.

## Optional: DeviceDeck MCP

If you are driving through DeviceDeck's MCP rather than browser tools, the same read-only
inspection is `mcp__devicedeck__ui_tree` (structure), `mcp__devicedeck__screenshot` (pixels),
and `mcp__devicedeck__assert_visible` (an expectation).
