# DeviceLab driver binaries license

maestro-runner's source code is licensed under the Apache License 2.0 (see [../LICENSE](../LICENSE)).
The prebuilt on-device agents listed below are **not** open source. They are proprietary software
of DeviceLab, shipped as binaries only, and are covered by this license instead of the Apache License.

## What this covers

- `drivers/android/devicelab-android-driver.apk` and `drivers/android/devicelab-android-driver-test.apk`:
  the DeviceLab Android agent (`--driver devicelab` on Android).
- `drivers/ios/devicelab-ios-agent/simulator/` and `drivers/ios/devicelab-ios-agent/device/`:
  the DeviceLab iOS agent (`--driver devicelab` on iOS simulators and real iPhones).

Everything else in `drivers/` keeps its own license. In particular, `drivers/ios/devicelab-ios-agent/signing-stub/`
and `drivers/ios/DevicelabIOSRunner/` are source code under the Apache License 2.0, and
`drivers/ios/WebDriverAgent/` and the Appium UiAutomator2 server APKs are under their upstream licenses.

## What you may do

- **Use** the agents, free of charge, with maestro-runner, for any purpose, including commercial testing,
  on your own devices, simulators, emulators, CI machines and device clouds.
- **Redistribute** the agents **unmodified and only as part of maestro-runner**, for example in a fork of
  maestro-runner, a Docker image, a CI cache or a package mirror, as long as this license file goes with them.

## What you may not do

- Modify the agents, or distribute modified copies.
- Distribute the agents separately from maestro-runner, or use them with other software.
- Reverse-engineer, decompile or disassemble the agents, **except as permitted by applicable law**
  (for example, where the law allows decompilation to achieve interoperability) and then only to that extent.
- Remove or change any notices in them.

## No warranty

The agents are provided "as is", without warranty of any kind, express or implied, including fitness for a
particular purpose. In no event is DeviceLab liable for any claim or damages arising from their use.

Copyright © DeviceLab. All rights not expressly granted here are reserved.
