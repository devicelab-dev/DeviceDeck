# Driving DeviceDeck with an agent

DeviceDeck mirrors a simulator or emulator's native accessibility tree as a real
browser DOM: an element's type becomes an ARIA role, its accessibility label
becomes the accessible name, its accessibility identifier becomes `data-testid`.
Every web agent that acts by reading a page's accessibility tree therefore drives
a device the same way it drives a website — by role and name, no plugin, no
coordinates, no vision model.

## Set up your agent

Every agent needs two things: the `devicedeck` binary installed and running (`devicedeck` in a
terminal), and a browser tool — [Playwright MCP](https://github.com/microsoft/playwright-mcp). The
`devicedeck mcp` server adds device tools (list, boot, launch, UI tree) and briefs the agent when it
connects; the three skills (authoring, flows, triage) teach it the rest.

**Claude Code** — the plugin brings the skills and the `devicedeck` MCP server:

```bash
claude mcp add playwright npx @playwright/mcp@latest
claude plugin marketplace add devicelab-dev/DeviceDeck
claude plugin install devicedeck@devicedeck-marketplace
```

**Gemini CLI** — one extension brings both MCP servers and the skills:

```bash
gemini extensions install https://github.com/devicelab-dev/DeviceDeck
```

**Codex CLI**

```bash
codex mcp add playwright -- npx @playwright/mcp@latest
codex mcp add devicedeck -- devicedeck mcp
npx skills add devicelab-dev/DeviceDeck
```

**VS Code / GitHub Copilot**

```bash
code --add-mcp '{"name":"playwright","command":"npx","args":["@playwright/mcp@latest"]}'
code --add-mcp '{"name":"devicedeck","command":"devicedeck","args":["mcp"]}'
npx skills add devicelab-dev/DeviceDeck
```

**Cursor** — add both servers to `~/.cursor/mcp.json` (or `.cursor/mcp.json` in a project), then
`npx skills add devicelab-dev/DeviceDeck`:

```json
{
  "mcpServers": {
    "playwright": { "type": "stdio", "command": "npx", "args": ["@playwright/mcp@latest"] },
    "devicedeck": { "type": "stdio", "command": "devicedeck", "args": ["mcp"] }
  }
}
```

**Anything else** (Windsurf/Devin, Cline, JetBrains AI, …) — the same two servers in that
client's MCP settings, in its own format; `npx skills add devicelab-dev/DeviceDeck` installs the
skills for most agents. Clients that install [Agent Plugins](https://agent-plugins.org) can install
this repository directly: `plugin.json`, `mcp.json` and `skills/` at its root are that bundle.

## Tested with

This page records that claim being tested. Each tool below is a **stock release**,
pointed at a device URL, driving TestHive on an iOS 26.2 simulator. Nothing about
any of them knows what a simulator is.

## Results

| Tool | Version | Reads the tree | Acts by | Full login | Leaves an artifact |
|---|---|---|---|---|---|
| [agent-browser](https://github.com/vercel-labs/agent-browser) | 0.37.1 | accessibility snapshot, `@e` refs | ref | ✓ end to end to the catalog | no (imperative CLI) |
| [playwright-cli](https://github.com/microsoft/playwright-cli) | 0.1.19 | aria snapshot, `e` refs | ref / `getByRole` / `getByTestId` | ✓ | **yes — records a Playwright spec** |
| [browser-harness](https://github.com/browser-use/browser-harness) | 0.1.13 | `Accessibility.getFullAXTree` over CDP | coordinate + `insertText` | reads fully; typing fights the soft keyboard (see below) | agent-written helpers, not a flow |
| Playwright (library) | 1.x | aria snapshot | `getByTestId` / `getByRole` | ✓ (the committed specs) | yes — the spec you write |
| [Stagehand](https://github.com/browserbase/stagehand) LOCAL | 4.0.2 | trimmed accessibility tree | `act()` / `observe()` | not run here — needs an LLM key | cached deterministic action |

Transcripts are under [`examples/agents/`](../examples/agents/); the recorded
Playwright spec is [`examples/playwright/tests/recorded/login-recorded.spec.ts`](../examples/playwright/tests/recorded/login-recorded.spec.ts).

## What each proof shows

**agent-browser** — `snapshot -i` lists the four login controls with their roles
and names; `fill @e1` / `fill @e2` reach the device, Sign In enables, `click @e3`
lands on the catalog where every product's Add button is a distinct, addressable
control. This is the plain case: the tool that 42k projects use to drive a browser
drives a simulator with zero changes.

**playwright-cli** — the same login by ref, wrapped in `recording-start` /
`recording-stop`. The recorder emitted:

```js
await page.getByTestId('username-input').fill('devicelab');
await page.getByTestId('password-input').fill('robustest');
await page.getByTestId('login-button').click();
```

Those three lines are a durable, reviewable mobile test, produced by a web
recorder that never knew it was looking at a phone. The committed spec replays
them green on a live simulator.

**browser-harness** — `Accessibility.getFullAXTree` returns the mirror's real
tree, roles and names intact, which is the substrate claim: browser-harness reads
exactly what DeviceDeck manufactures. Its typing path is the one caveat. It drives
by coordinate and types with CDP `insertText` into whatever the DOM has focused, so
once the on-screen keyboard opens over the next field, a coordinate tap can miss it.
Tools that focus a field by selector before typing (agent-browser, Playwright) do
not hit this. It is a property of coordinate-first driving, not of the mirror.

**Stagehand** — its Playwright substrate is the same one proven above, so the
deterministic half drives the mirror. Its `act()` / `observe()` need a model key,
which this machine does not have, so that half is not exercised here.

## The one thing every agent needs to know

A DeviceDeck field's `fill()` returns when the **mirror** has the text; the
keystrokes are still travelling to the device behind it. Before an action that
depends on the typed value having landed — clicking a submit button the app
enables only when the field is filled — wait for the device to echo it. Agents
that re-snapshot until the control is ready (the normal "ref not found, snapshot
again" loop) get this for free; a script polls the device value. See
[behaviors.md](behaviors.md).

## Coordinate agents still get a selector

Some agents (the Anthropic and OpenAI and Google computer-use tools) act by
pixel, not by ref. A pixel click on the mirror's video resolves to the DOM
element under the point, so Flow Capture still records it as a durable `tapOn: id`
step, not a coordinate. A coordinate agent driving DeviceDeck produces a
selector-based artifact anyway.
