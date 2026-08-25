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
| `launch_app` | Launch an app (fresh by default; `fresh:false` resumes) |
| `ui_tree` | The device's current UI tree as JSON — what to act on |
| `tap` | Tap an element by its testid — resolved against the tree, a durable selector, not a coordinate |
| `assert_visible` | Check an element is on screen, by testid or by visible text |
| `screenshot` | Capture the device screen as a PNG image, for reasoning over pixels |
| `device_page_url` | The automation-page URL to open with the agent's own browser tools and drive by selector |

The pattern: `list_devices` → `boot_device` → `launch_app` → read `ui_tree` to
see what's on screen → `tap` and `assert_visible` by testid to act and check.
Everything is selector-based — the same durable identifiers a captured flow
uses — so what an agent does here is reviewable and runs on real hardware
unchanged.

**Typing** goes through the device page, not a tool: tap the field, then open
`device_page_url` and drive the keyboard with your browser tools (Playwright,
Puppeteer). They verify each keystroke against the device's own read-back —
the reliability that lives in the mirror — so typing is not reimplemented, and
not made flaky, on the server side.
