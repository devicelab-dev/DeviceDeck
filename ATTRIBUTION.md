# Attribution

DeviceDeck is licensed under the Apache License 2.0 (see `LICENSE`), and
includes work derived from the following open-source projects.

The DeviceLab device agents DeviceDeck embeds and installs — the Android
agent APKs (`internal/home/android/`) and the iOS agent
(`internal/home/ios/devicelab-ios-agent/`), shipped with its maestro-runner
dependency — are proprietary binaries, not covered by the Apache License:
free to use, redistributed unmodified, under `LICENSE-BINARIES.md`, which
ships with them.
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
- The frame-callback capture recipe (baguette commit 41648ad): registering
  `registerScreenCallbacksWithUUID:callbackQueue:frameCallback:surfacesChangedCallback:propertiesChangedCallback:`
  on the framebuffer display descriptor instead of polling
  `framebufferSurface`, the `PendingCapture` coalescer that keeps at most one
  capture queued, the idle-floor timer, and the per-capture autorelease-pool
  drain — `Sources/devicedeck-video/ScreenCallbacks.swift`, `Capture.swift`,
  `Sources/VideoCore/PendingCapture.swift`.

## tapflow — MIT License

https://github.com/jo-duchan/tapflow

Derived material in `sidecar/`:

- The sidecar architecture: one small Swift binary fed a compact binary
  frame protocol over stdin — `main.swift`, `HIDProtocol/Frame.swift`
  (frame layout redesigned, shape retained).
- SimulatorKit/CoreSimulator loading with Xcode-discovery fallback and the
  keyboard-service primary / HIDArbitrary-fallback key path —
  `SimKit.swift`, `Injector.swift`.

## idb — MIT License

https://github.com/facebook/idb — Copyright (c) Meta Platforms, Inc. and affiliates.

Derived material in `sidecar/`:

- The Xcode 27 `dtuhidd` input transport: the digitizer service name, the
  `DTUHIDMessage` envelope and the `IndigoDigitizerEvent` /
  `IndigoKeyboardButtonEvent` / `IndigoButtonEvent` payload shapes and value
  types, the CoreSimulator 1155.4 version gate, the barrier liveness probe
  and its timings, and the simulator-to-host XPC connection recipe
  (`-[SimDevice lookup:error:]`, `xpc_endpoint_create_mach_port_4sim`,
  `xpc_connection_enable_sim2host_4sim`) — `Sources/DTUHID/DTUHIDWire.swift`,
  `Sources/devicedeck-hid/DTUHIDClient.swift`.

## Android Open Source Project — Apache License 2.0

https://android.googlesource.com/platform/external/qemu/

Derived material in `internal/emugrpc/`:

- `emulator_controller.pb.go` and `emulator_controller_grpc.pb.go` are Go
  code generated from the Android Emulator's `emulator_controller.proto`
  (the gRPC control interface Android Studio's device streaming uses). We
  use it as a client to pull the emulator's framebuffer over
  `streamScreenshot` for Android video capture — `internal/video/emugrpc.go`.
  The generated files retain the upstream AOSP copyright header.

## maestro-runner — Apache License 2.0

https://github.com/devicelab-dev/maestro-runner

DeviceDeck imports maestro-runner as a Go library for its device drivers, and
the binary embeds the devicelab Android driver APKs that maestro-runner
distributes (`internal/home/android/`), copied unmodified from the pinned
module version. They are installed onto Android emulators to read the screen
and inject input.

These licences permit this use with attribution.

Upstream copyright notices, retained here as those licences ask:

- baguette — Copyright the baguette authors, Apache License 2.0.
  https://github.com/tddworks/baguette/blob/main/LICENSE
  (checked 2026-08-19: baguette ships no NOTICE file, so Apache-2.0
  section 4(d) adds nothing further to propagate.)
- tapflow — Copyright (c) 2026 tapflow contributors, MIT License.
  https://github.com/jo-duchan/tapflow/blob/main/LICENSE
- maestro-runner and its devicelab Android driver — Copyright 2024 DeviceLab,
  Apache License 2.0.
  https://github.com/devicelab-dev/maestro-runner/blob/main/LICENSE
- Android Emulator gRPC — Copyright (C) 2018 The Android Open Source
  Project, Apache License 2.0. Header retained in
  `internal/emugrpc/emulator_controller.pb.go`.

DeviceDeck itself is Apache-2.0; see `LICENSE`.
