# Attribution

DeviceDeck includes work derived from the following open-source projects.
Their reverse-engineering of SimulatorKit's private HID pipeline is what
makes host-side input injection possible; we gratefully build on it.

## baguette — Apache License 2.0

https://github.com/tddworks/baguette

Derived material in `sidecar/Sources/devicedeck-hid/`:

- The IOHIDEvent digitizer dispatch recipe (parent + finger child events,
  the `IndigoHIDMessageForTrackpadEventFromHIDEventRef` wrapper, and the
  routing-target / edge-bitmask byte patches) — `Digitizer.swift`.
- Function signatures for `IndigoHIDMessageForMouseNSEvent` (9-arg two-finger
  form), `IndigoHIDMessageForHIDArbitrary` (iOS 26 argument order), and
  `IndigoHIDMessageForButton` — `SimKit.swift`.
- System-gesture timing recipes (swipe-to-home, app switcher, notification
  center, lock screen pulls) and the two-finger settle-window retry —
  `Injector.swift`.

## tapflow — MIT License

https://github.com/jo-duchan/tapflow

Derived material in `sidecar/`:

- The sidecar architecture: one small Swift binary fed a compact binary
  frame protocol over stdin — `main.swift`, `HIDProtocol/Frame.swift`
  (frame layout redesigned, shape retained).
- SimulatorKit/CoreSimulator loading with Xcode-discovery fallback and the
  keyboard-service primary / HIDArbitrary-fallback key path —
  `SimKit.swift`, `Injector.swift`.

Both licenses permit this use with attribution; this file, alongside the
upstream license texts in their repositories, satisfies that requirement.
