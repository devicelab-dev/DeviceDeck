# The console

The console is DeviceDeck's own page for using devices by hand — open it at the address
`devicedeck` prints (`http://127.0.0.1:8787` on this Mac, or the network address from another
machine).

## The device list

**Devices** shows every simulator and emulator on the Mac, running ones first. Each card has the
model and the name you gave it, the runtime, and whether it is running. Click a card — or its
**Use** / **Boot** button — to open it; a stopped device boots first.

**Apps** lists the builds you passed with `devicedeck --app`: name, version, bundle id, the minimum
OS and size (hover a card for its architectures, build date and file). **Launch** opens a running
device of the build's platform and launches it, installing it first if the device does not have it.
With no matching device running, the card says what to boot. Builds a folder held that cannot run
on a simulator — a device build, say — are listed under the cards with the reason.

## A device

The page shows the device's screen. It appears once the device is ready to use — video flowing,
touch connected, and DeviceDeck's engine (which reads the UI) started — with each stage shown while
it loads. If you started DeviceDeck with `--app`, the matching build is launched the first time you
open a device.

**Drive it** with your mouse: click to tap, drag to swipe. Click the screen once, then type — keys go
to the device, including Return and Backspace.

**The left sidebar**

- **Device** — its name, runtime and id (**Copy** for tests), and **End session**, which stops
  DeviceDeck on this device and shuts it down.
- **App** — pick a registered build and **Launch** it (a fresh start: its data is cleared first), or
  type a bundle id for Record and Inspect.
- **Use it** — the device page URL for your tests, and a ready-made prompt to paste into Claude Code.

**Controls** (right panel): **Home**, **Switcher** (the app switcher), **Lock**, **Shot** (a
screenshot in a new tab), **Inspect** and **Record**.

**Inspect** outlines every element on screen. Hover one to see its id, role and text — exactly what
a test selects it by (`getByTestId`, `getByRole`). Clicking an outline taps that element. The outlines
follow the screen as it changes.

**Record** captures what you do as a Maestro flow — see [Recording a flow](flows.md).

## One device, one driver

A device takes input from one place at a time, so two people or a person and a test cannot tap over
each other.

- **Tabs in your browser hand over:** the tab you are using drives the device; a tab in the
  background lets go, and clicking back into it takes the device back.
- **Take over** — when someone else holds the device, the red card offers it: it disconnects them,
  and their page says so.

## When the screen turns red

The console says what is wrong instead of failing quietly.

| Message | What it means | What to do |
|---|---|---|
| **Another client is driving this device** | A test, an agent or another browser holds it. The card names who. | Stop that client, or **Take over**. |
| **This device is open in another tab** | Another tab in your browser took it. | **Use it here**, or click into this tab. |
| **Another page took this device over** | Someone pressed Take over elsewhere. | **Take over** to take it back. |
| **This device's session was ended** | Someone pressed End session; the device is off. | **Device list**, and boot it again. |
| **DeviceDeck is not reachable** | The `devicedeck` server stopped. | Start it again; the page reconnects on its own. |
| **The device engine did not start** / **is not answering** | DeviceDeck cannot read the device's UI. | **Retry**. If it keeps failing, the run's log folder says why. |
| **The screen stream stopped** | Video from the device ended — often because it shut down. | **Reconnect**. |

## Sharing with your team

`devicedeck` listens on your network, and its startup guide prints the address teammates open — see
[A shared device farm](device-farm.md).

## Logs

Every run writes a folder under `~/.devicedeck/logs` (the path is printed at startup), with
`devicedeck.log`, the device driver's `runner.log`, and one log per device. Attach it to an issue.
