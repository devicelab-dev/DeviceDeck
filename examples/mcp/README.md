# Drive devices from an AI agent (MCP)

`devicedeck mcp` is a [Model Context Protocol](https://modelcontextprotocol.io)
server: it gives an agent (Claude, Cursor, any MCP client) the same device
tools the server exposes over HTTP, spoken over stdin/stdout. It is a thin
adapter — it drives a running `devicedeck serve`, so there is one core, not two.

## Run it

```bash
devicedeck serve                 # in one terminal — the device server
DEVICEDECK_URL=http://127.0.0.1:8787 devicedeck mcp   # the agent spawns this
```

`--server <url>` (or `DEVICEDECK_URL`) points it at the server; the default is
`http://127.0.0.1:8787`.

## Wire it into a client

Most MCP clients take a command and args. For example, in a client's config:

```json
{
  "mcpServers": {
    "devicedeck": { "command": "devicedeck", "args": ["mcp"] }
  }
}
```

## Tools

| Tool | What it does |
| --- | --- |
| `list_devices` | Every simulator/emulator, with udid/serial, name, and boot state |
| `boot_device` | Boot one and wait for it to be ready |
| `install_app` | Install a `.app` (Simulator) or `.apk` (emulator) from a path — not a device `.ipa` |
| `launch_app` | Launch an app (fresh by default; `fresh:false` resumes; `appFile` installs first) |
| `ui_tree` | The device's current UI tree as JSON — what to act on |
| `tap` | Tap an element by its testid — resolved against the tree, a durable selector, not a coordinate |
| `assert_visible` | Check an element is on screen, by testid or by visible text |
| `screenshot` | Capture the device screen as a PNG image, for reasoning over pixels |
| `device_page_url` | The automation-page URL to open with the agent's own browser tools and drive by selector |

The pattern: `list_devices` → `boot_device` → `install_app` (if needed) → `launch_app` → read `ui_tree` to
see what's on screen → `tap` and `assert_visible` by testid to act and check.
Everything is selector-based — the same durable identifiers a captured flow
uses — so what an agent does here is reviewable and runs on real hardware
unchanged.

**Typing** goes through the device page, not a tool: tap the field, then open
`device_page_url` and drive the keyboard with your browser tools (Playwright,
Puppeteer). They verify each keystroke against the device's own read-back —
the reliability that lives in the mirror — so typing is not reimplemented, and
not made flaky, on the server side.

## Listing on registries

`glama.json` at the repo root marks the server for [Glama](https://glama.ai/mcp)'s
registry, which indexes it from the public repository. The listing metadata a
registry needs:

- **Name** — `dev.devicelab/devicedeck` (reverse-DNS; DeviceLab owns the domain)
- **Repository** — `https://github.com/devicelab-dev/DeviceDeck`
- **Run** — `devicedeck mcp` (stdio); install the release binary, no package manager
- **Tools** — the eight above
- **Category** — mobile / device automation / testing

DeviceDeck ships as a single Go binary, not an npm/pypi package, so a registry
entry points at the release archive and the `devicedeck mcp` command rather than
`npx`. Submitting the listing is a launch step — it needs the repository public;
the marker and this metadata are ready for it.
