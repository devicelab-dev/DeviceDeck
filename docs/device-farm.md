# A shared device farm

Because the device is a web page, the machine that *runs* the simulators and the machine that
*drives* them don't have to be the same. Bind the server to your network:

```bash
devicedeck serve --addr 0.0.0.0:8787
```

One Mac then becomes a **shared farm**: teammates on Linux, Windows, or another Mac open the URL and
drive devices from their own browser, tests, or agent — no Xcode, no Android Studio, no local
simulator on the client at all.

- **A device serves one driver at a time**, so it is a farm of *N* devices for *N* people.
- **Automation is cheap over the network** — a test reads the lightweight DOM tree, not video; the
  pixel stream is only for a human watching.
- **No auth yet.** The server is unauthenticated. Sharing over `--addr` is fine on a trusted network,
  but do not hang it on the open internet as-is (see [Known limits](../README.md#known-limits)).
