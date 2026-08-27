# DeviceDeck skills — index

DeviceDeck mirrors an iOS Simulator / Android emulator as a **real-DOM web page** at
`http://127.0.0.1:8787/device/{udid}` (use `booted` for the udid when one device is up).
Drive it with **Playwright MCP** (`@playwright/mcp`) — the same browser tool you already use
for the web. Select native elements by `data-testid` (the app's accessibility id).

Read the guide that matches the task:

| Task | Guide |
|---|---|
| Author a Playwright mobile test | [`devicedeck-authoring`](devicedeck-authoring/SKILL.md) |
| Diagnose a failing or odd mobile test | [`devicedeck-triage`](devicedeck-triage/SKILL.md) |
| Capture or replay a Maestro flow | [`devicedeck-flows`](devicedeck-flows/SKILL.md) |

## Using these outside Claude Code

In Claude Code these load as plugin skills and fire automatically by task. In any other agent
(Cursor, etc.), point it here with one line in your project rules:

> For DeviceDeck mobile work, read `skills/INDEX.md` and follow the matching guide. The device
> is a real-DOM web page at `http://127.0.0.1:8787/device/booted`; drive it with Playwright MCP,
> select by `data-testid`, and wait for the device to echo a typed value before submitting.
