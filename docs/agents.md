# Driving with an AI agent

An agent drives the device with the **browser tools it already uses for the web** — it snapshots the
DOM mirror, reasons over the tree, and acts by ref. Nothing DeviceDeck-specific to learn.

## Setup (Claude Code)

```bash
claude mcp add playwright npx @playwright/mcp@latest    # the driver — the browser MCP your agent uses
claude plugin marketplace add devicelab-dev/DeviceDeck  # DeviceDeck's skills (+ optional MCP)
```

Then describe the test in plain language — *"log into my app and confirm the home screen."* The agent
navigates to the device page, snapshots, types, and clicks.

- **Playwright MCP is the driver** ([`@playwright/mcp`](https://github.com/microsoft/playwright-mcp)).
  Any browser MCP works; Playwright MCP is the mature, accessibility-snapshot-based one, which fits
  the DOM mirror best. There is no "Cypress MCP" — Cypress/Puppeteer are *output* frameworks, not
  drivers.
- **The plugin's skills** teach the agent the *device layer* a web agent wouldn't know: durable
  `data-testid` selectors, waiting for the device to echo a typed value, and the device-only controls
  on `window.devicedeck` (`gesture`, `swipe`, `screenshot`, …). See [`skills/`](../skills/).

## Any agent

Works with **any MCP agent** (Cursor, Windsurf, …). In Claude Code the skills load automatically;
elsewhere, point the agent at [`skills/`](../skills/) with one line in your project rules — see
[`skills/INDEX.md`](../skills/INDEX.md).

## When the agent manages devices itself

To have the agent *pick, boot, and launch* devices (not a pre-booted one), DeviceDeck ships its own
MCP — `devicedeck mcp` — with `list_devices`, `boot_device`, `launch_app`, and `device_page_url`
(which returns the page to hand to the browser MCP). It's optional: driving a booted device needs
only your browser MCP. See [`examples/mcp/`](../examples/mcp/).
