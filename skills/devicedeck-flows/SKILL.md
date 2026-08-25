---
name: devicedeck-flows
description: Use when working with DeviceDeck Flow Capture — recording a manual session on a simulator/emulator into a Maestro flow with durable selectors, or replaying a captured flow so it runs unchanged on real hardware.
metadata:
  short-description: Capture and replay DeviceDeck Maestro flows
---

# Flow Capture and replay

DeviceDeck's differentiator: record a manual session and it becomes a **Maestro flow with durable
selectors** (accessibility identifiers, never coordinates), so the same flow replays on real
hardware — the funnel from "I did this by hand" to "this is a test" with no rewrite.

## Capturing

Recording is driven from the console UI (open `http://127.0.0.1:8787`, pick a device, **Record**),
not from an MCP tool. While recording:

- Every tap resolves to a durable `tapOn: id="..."` selector. A tap that resolves to nothing
  addressable is refused rather than recorded as a coordinate — do not force it; find the
  element that carries the identifier.
- Arm **Assert** to record an assertion instead of a tap (the next tap becomes `assertVisible`).
- The output is a YAML flow under `examples/captured/`.

## Replaying

A captured flow is a stock Maestro flow — replay it through the `devicelab` driver:

```
maestro-runner --driver devicelab <flow.yaml>
```

For a sustained replay (a soak test), use `scripts/flow-gate.sh <flow.yaml> <count> <udid>`, which
batches runs under the simulator's render-server crash ceiling and resets the CoreSimulator
subsystem between batches.

## The contract

A captured flow must run **unchanged** on devicelab.dev real devices — identical format, same
selectors. If a flow needs editing to replay elsewhere, the selectors were not durable: re-capture
against elements that carry accessibility identifiers.
