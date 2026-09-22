# CLI reference

## Commands

| Command | What it does |
|---|---|
| `devicedeck` | Start the server and console — the same as `devicedeck serve`. |
| `devicedeck serve [flags]` | Start the server and console (flags below). |
| `devicedeck mcp [--server URL]` | Run the MCP server on stdin/stdout for an AI agent. It drives a running server: list devices and app builds, boot, install and launch, read the UI tree, act. |
| `devicedeck doctor` | Check Xcode and the iOS runtime, `adb` and an Android emulator, Node.js, Claude Code and maestro-runner, and say how to fix what is missing. |
| `devicedeck version` | Print the version and commit. |
| `devicedeck help` | Print the usage. `devicedeck serve --help` lists the server's flags. |

## `serve` flags

| Flag | Default | |
|---|---|---|
| `--addr` | `0.0.0.0:8787` | Listen address. The default accepts other machines on your network; `127.0.0.1:8787` keeps it to this Mac. |
| `--app` | — | An app build to install on a device the first time it is launched there: a simulator `.app`, an `.apk`, or a folder of them. Repeat for both platforms. With `--ready`, a bundle id also works. |
| `--ready` | off | Pick a device (a booted one, else the newest iOS runtime 26.2 or later), bring it up, launch the first `--app`, and print a machine-readable status object — for CI and scripts. |
| `--keep-devices` | off | Leave Android emulators DeviceDeck started running on exit. iOS simulators are shut down by the device runner either way. |
| `--fps` | `30` | Video frame rate. |
| `--sidecar` | found beside the binary | Path to `devicedeck-hid`. |
| `--video-sidecar` | found beside the binary | Path to `devicedeck-video`. |

## Environment

| Variable | |
|---|---|
| `DEVICEDECK_HOME` | Where DeviceDeck keeps its binaries, drivers, build cache and logs. Default `~/.devicedeck`. |
| `DEVICEDECK_LOG` | What the terminal shows: `error`, `warn` (default), `info` or `debug`. The run's log files always record everything. |
| `DEVICEDECK_URL` | The server `devicedeck mcp` drives, when `--server` is not given. Default `http://127.0.0.1:8787`. |
| `DEVICEDECK_HID`, `DEVICEDECK_VIDEO` | Sidecar paths, as the flags above. |
| `DEVICEDECK_APP_SKILLS` | Folder of per-app notes the MCP server returns with a launch. Default `app-skills`. |

## Logs

Every run writes a folder under `~/.devicedeck/logs` — its path is printed at startup — with
`devicedeck.log` (every request, device event and tool call), `runner.log` from the device driver,
one log per sidecar and device, and `crash.log` if the process panics. The last 20 runs are kept.

## HTTP API

Everything the console, the device page and the MCP server do goes through this API on the server's
address. `{udid}` is a simulator's UDID or an emulator's serial. Coordinates are normalized: `0`–`1`
across the screen.

**Devices and apps**

| Request | |
|---|---|
| `GET /api/devices` | Every simulator and emulator, running or not. |
| `POST /api/devices/{udid}/boot` | Boot a device. |
| `POST /api/devices/{udid}/shutdown` | End its session: stop DeviceDeck's engine and sidecars for it and shut it down. |
| `GET /api/apps` | The builds registered with `--app`, and any skipped with why. |
| `GET /api/devices/{udid}/apps` | The builds that suit this device, marked installed or launched. |
| `POST /api/devices/{udid}/app/install` | `{"appFile": "/path/MyApp.app"}` |
| `POST /api/devices/{udid}/app/launch` | `{"app": "com.example.app"}` (optionally `"appFile"` to install first). Returns once the app is taking input. Clears the app's data first; add `?reset=no` to resume instead. |
| `POST /api/devices/{udid}/openurl` | `{"url": "myapp://checkout"}` — a link or deep link. |

**Screen and input**

| Request | |
|---|---|
| `GET /api/devices/{udid}/tree` | The UI tree as JSON. `?after=<interaction>` waits for the screen to change and settle first; `?app=` scopes it to one app. |
| `GET /api/devices/{udid}/screenshot` | A PNG of the screen. |
| `POST /api/devices/{udid}/tap` | `{"x": 0.5, "y": 0.5}`, optionally `"durationMs"` for a long press. |
| `POST /api/devices/{udid}/swipe` | `{"x1": 0.5, "y1": 0.8, "x2": 0.5, "y2": 0.2}`, optionally `"durationMs"`. |
| `POST /api/devices/{udid}/gesture` | `{"kind": "home"}` — or `appSwitcher`, `notificationCenter`, `lockScreen`. |
| `POST /api/devices/{udid}/button` | `{"button": "lock"}` or `"home"`. |
| `POST /api/devices/{udid}/key` | `{"usage": 40}` — a USB HID keyboard usage, optionally `"modifiers"`. |

**Recording a flow**

| Request | |
|---|---|
| `POST /api/devices/{udid}/capture/start` | `{"app": "com.example.app"}` |
| `POST /api/devices/{udid}/capture/assert` | `{"x": 0.5, "y": 0.4}` records an assertion on the element there; `"check": "wait"` records a wait for it instead. |
| `POST /api/devices/{udid}/capture/stop` | Stop, and return the flow's YAML and steps. |
| `GET /api/devices/{udid}/capture` | Whether it is recording, and the steps so far. |

**Engine**

| Request | |
|---|---|
| `POST /api/devices/{udid}/engine` | Start DeviceDeck's engine for the device ahead of first use. |
| `GET /api/devices/{udid}/engine` | Its state: `starting`, `ready` or `failed`, with what it is doing. |

The device page itself is `/device/{udid}?app={bundleId}`; the console is `/`.
