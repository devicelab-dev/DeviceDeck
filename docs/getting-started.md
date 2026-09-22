# Getting started

From nothing to a running test. The short version is the [README](../README.md#get-started).

## Install

```bash
curl -fsSL https://open.devicelab.dev/install/devicedeck | bash

# A specific version
curl -fsSL https://open.devicelab.dev/install/devicedeck | bash -s -- --version 0.1.1
```

It installs into `~/.devicedeck/bin` and adds that folder to your `PATH` — open a new terminal, or
run `export PATH="$HOME/.devicedeck/bin:$PATH"`. No sudo; the Android driver ships inside the binary.

**With npm**, in a Playwright project:

```bash
npm install --save-dev devicedeck
npx devicedeck --version
```

The host is a Mac (Apple Silicon or Intel). Installing the npm package on Linux — a CI job that drives
a remote Mac — is harmless.

## Check your setup

```bash
devicedeck doctor
```

It checks Xcode and the iOS runtime, `adb` and an Android emulator, Node.js, Claude Code and
maestro-runner, and says how to fix anything missing. Only Xcode is required for iOS; only the
Android SDK for Android.

## Your app build

DeviceDeck installs your app from a **simulator build** (`.app`) or an **APK**. An App Store or
TestFlight `.ipa` is a device build and will not run on a simulator.

| App | Build | Lands in |
|---|---|---|
| iOS (Xcode) | `xcodebuild -scheme MyApp -sdk iphonesimulator -configuration Debug -derivedDataPath build` | `build/Build/Products/Debug-iphonesimulator/MyApp.app` |
| Android | `./gradlew assembleDebug` | `app/build/outputs/apk/debug/app-debug.apk` |
| React Native, Expo | `npx react-native run-ios` or `npx expo run:ios` / `run-android` or `run:android` | iOS: `~/Library/Developer/Xcode/DerivedData/<App>-*/Build/Products/Debug-iphonesimulator/<App>.app`; Android: `android/app/build/outputs/apk/debug/app-debug.apk` |
| Flutter | `flutter build ios --simulator` / `flutter build apk` | `build/ios/iphonesimulator/Runner.app` / `build/app/outputs/flutter-apk/app-release.apk` |

## Start DeviceDeck

```bash
devicedeck --app path/to/MyApp.app --app path/to/app-debug.apk
```

`--app` takes `.app` and `.apk` files, or a folder of them. Each build is installed on a device the
first time it is launched there — by you, a test or an agent. On start, DeviceDeck prints the console
address (and the network address teammates use), your registered apps, and how to connect Claude
Code.

## Your first test

You don't need a mobile test suite: let your agent write the first one.

```bash
claude mcp add playwright npx @playwright/mcp@latest
claude plugin marketplace add devicelab-dev/DeviceDeck
claude plugin install devicedeck@devicedeck-marketplace
```

Then ask Claude: *"Write a Playwright test that logs in to my app."* It finds your registered build,
boots a simulator, launches the app, drives it through the device page and writes the spec. Run it:

```bash
npx playwright test
```

Using another agent? See [AI Agents](agents.md). Prefer to write tests yourself? See
[Writing Tests](testing.md).

## Use a device by hand

Open the console at `http://127.0.0.1:8787`, pick a device, and drive it with your mouse and keyboard.
See [The Console](console.md).

## Uninstall

Delete `~/.devicedeck` and the `# DeviceDeck` line from your shell profile — or `npm uninstall
devicedeck`.
