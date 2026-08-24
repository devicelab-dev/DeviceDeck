# Attribution

DeviceDeck is licensed under the Apache License 2.0 (see `LICENSE`), and
includes work derived from the following open-source projects.
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
- The VideoToolbox H.264 low-latency encoder (session tuning, AVCC
  extraction, avcC parameter-set blob) and the framebuffer IOSurface
  discovery recipe (`deviceIOPorts` → framebuffer display port →
  descriptor surface, with the one-shot `updateIOPorts` materialization) —
  `Sources/devicedeck-video/H264Encoder.swift`, `Framebuffer.swift`.

## tapflow — MIT License

https://github.com/jo-duchan/tapflow

Derived material in `sidecar/`:

- The sidecar architecture: one small Swift binary fed a compact binary
  frame protocol over stdin — `main.swift`, `HIDProtocol/Frame.swift`
  (frame layout redesigned, shape retained).
- SimulatorKit/CoreSimulator loading with Xcode-discovery fallback and the
  keyboard-service primary / HIDArbitrary-fallback key path —
  `SimKit.swift`, `Injector.swift`.

## Android Open Source Project — Apache License 2.0

https://android.googlesource.com/platform/external/qemu/

Derived material in `internal/emugrpc/`:

- `emulator_controller.pb.go` and `emulator_controller_grpc.pb.go` are Go
  code generated from the Android Emulator's `emulator_controller.proto`
  (the gRPC control interface Android Studio's device streaming uses). We
  use it as a client to pull the emulator's framebuffer over
  `streamScreenshot` for Android video capture — `internal/video/emugrpc.go`.
  The generated files retain the upstream AOSP copyright header.

These licences permit this use with attribution.

Upstream copyright notices, retained here as those licences ask:

- baguette — Copyright the baguette authors, Apache License 2.0.
  https://github.com/tddworks/baguette/blob/main/LICENSE
  (checked 2026-08-19: baguette ships no NOTICE file, so Apache-2.0
  section 4(d) adds nothing further to propagate.)
- tapflow — Copyright (c) 2026 tapflow contributors, MIT License.
  https://github.com/jo-duchan/tapflow/blob/main/LICENSE
- Android Emulator gRPC — Copyright (C) 2018 The Android Open Source
  Project, Apache License 2.0. Header retained in
  `internal/emugrpc/emulator_controller.pb.go`.

DeviceDeck itself is Apache-2.0; see `LICENSE`.
