# Behaviors & things to know

Quirks of driving a native app through the DOM mirror that a web test wouldn't expect.

- **Only the screen you are on is mirrored.** iOS keeps a screen in the hierarchy after the app
  navigates away, so a stale login form — credentials still in its fields — would otherwise show up on
  every following screen. Those are culled: a selector can't resolve to an invisible screen and fail
  silently.
- **Text fields are real `<input>` elements**, so `fill()`, `inputValue()`, and `toHaveValue()` work as
  on any page — as does the `browser_type` an agent reaches for. A field's contents live in its
  *value*, not its text — the one place the mirror departs from "every native node is a div".
- **Typing is verified.** `fill()` sets the text through the device's driver and returns once the
  device holds the value, so the next action can submit straight away. The device's read-back is on
  `data-dd-device-value` (a secure field reports bullets). `keyboard.type` still works, key by key, but
  on Android it raises the soft keyboard and the next tap can be spent closing it — prefer `fill()`.
- **A fresh launch is the clean slate.** A native app stays logged in across a raw relaunch;
  `POST /app/launch` wipes app data by default (pass `?reset=no` to resume where it was left).
- **Device-only controls** live on `window.devicedeck`: `gesture('home' | 'appSwitcher' | …)`,
  `swipe`, `button`, `key`, and `screenshot()` (the device screen as a PNG data URL). Call them via
  `page.evaluate` — the device page `<title>` lists the gesture set.
