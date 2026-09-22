# Recording a flow

Use your app by hand in the [console](console.md) and DeviceDeck writes down what you did as a
[Maestro](https://maestro.dev) flow — a plain YAML file you can review, keep in your repository,
replay with [maestro-runner](https://github.com/devicelab-dev/maestro-runner), and run unchanged on
real devices at [devicelab.dev](https://devicelab.dev).

## Record

1. Open a device in the console.
2. Pick your app in the sidebar (or type its bundle id).
3. Press **Record**, then use the app: tap, type, swipe.
4. Press **Stop**. The flow appears in the right-hand panel, with **Copy** and **Download** (which
   asks where to save it and suggests a name like `testhive-2026-09-23-0107.yaml`).

## Add checks

A flow of taps proves the steps ran, not that the app worked. While recording, **right-click an
element** to add a check instead of a step:

- **Assert visible** — the flow fails if the element is not on screen (`assertVisible`).
- **Wait until visible** — the flow waits up to 10 seconds for it, for spinners and slow network
  (`extendedWaitUntil`).

A check needs an element with an id or text; a right-click on empty space is refused rather than
recorded as a coordinate.

## Read a flow

```yaml
# devicedeck secrets: supply at replay with -e PASSWORD_INPUT=…
- launchApp:
    clearState: true
# devicedeck: tapOn selector=id confidence=high
- tapOn:
    id: "username-input"
    label: "username-input"
- inputText: "devicelab"
# devicedeck: inputText secret=${PASSWORD_INPUT}
- inputText: ${PASSWORD_INPUT}
# devicedeck: tapOn selector=id confidence=high
- tapOn:
    id: "login-button"
    label: "login-button"
# devicedeck: extendedWaitUntil visible selector=id confidence=high
- extendedWaitUntil:
    visible:
      id: "products-screen"
    timeout: 10000
    label: "wait for products-screen"
```

- **It starts from a clean app** (`launchApp` with `clearState`), as the recording did.
- **Each tap uses the most durable selector on screen** — the element's accessibility id first, then
  its text, narrowed by a parent (`childOf`) or a position (`index`) when several match.
- **Every step is graded** in the `# devicedeck:` comment above it. Maestro ignores comments; they
  are for whoever reviews the flow.

| Grade | Selector | Survives the next build? |
|---|---|---|
| `high` | an accessibility id | yes |
| `medium` | visible text, or an id-bearing parent | usually — breaks if the wording changes |
| `low` | a position (`index`) or a screen coordinate | rarely — fix it (below) |

## Passwords stay out of the file

Text typed into a password field is never written into the flow. The step reads
`${PASSWORD_INPUT}` — named after the field's id — and the comment at the top lists what a replay
must supply. The same flow then runs with a different account per environment.

## Screens without ids

When a screen has buttons and fields with no accessibility id, the flow opens with a `# desert:`
warning saying how many, and how to add ids in the app's framework: `accessibilityIdentifier` on
iOS, a resource id or Compose `testTag` on Android, `Semantics.identifier` in Flutter, `testID` in
React Native. Add them, record again, and those steps come out `high`.

## Replay

With [maestro-runner](https://github.com/devicelab-dev/maestro-runner) installed, on a booted
simulator or emulator:

```bash
maestro-runner --driver devicelab test flow.yaml -e PASSWORD_INPUT=…
```

The same file runs on real iPhones and Android phones at [devicelab.dev](https://devicelab.dev) —
same format, same selectors. If a flow needs editing to replay somewhere else, a selector was not
durable: add an id and record again rather than editing the flow by hand.

Recorded examples are in [`examples/captured/`](../examples/captured/).
