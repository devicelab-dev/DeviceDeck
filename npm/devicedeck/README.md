# devicedeck

Automate your iOS and Android app like a web app. DeviceDeck serves an iOS Simulator or Android
emulator as a web page, with the app's native UI mirrored as real DOM — so Playwright tests and AI
agents drive the native app by selector (`getByRole`, `getByTestId`).

```bash
npx devicedeck --app path/to/MyApp.app      # the console at http://127.0.0.1:8787
```

Or add it to your test project, so every machine and CI job runs the same version:

```bash
npm install --save-dev devicedeck
```

## Why npm

DeviceDeck is one binary with two small sidecars, and also installs with a shell script. This
package exists because Playwright projects already live in npm: pinning DeviceDeck in
`package.json` needs no separate bootstrap step.

There is no postinstall script and nothing is downloaded when you install. The binaries for your Mac
arrive as an ordinary optional dependency that npm selects by `os` and `cpu`, so installs work
offline, behind a proxy, and in CI that blocks postinstall network access.

## Platforms

The host runs on **macOS** (Apple Silicon and Intel) — it hosts the iOS Simulator. Tests, agents
and browsers on any OS drive it over the network; installing this package on Linux is harmless, it
just has nothing to run there.

## Documentation

Getting started, agents, tests and the console: https://github.com/devicelab-dev/DeviceDeck#readme

## License

Apache-2.0
