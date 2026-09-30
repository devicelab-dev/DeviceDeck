# A shared device farm

Because the device is a web page, the machine that *runs* the simulators and the machine that
*drives* them don't have to be the same. `devicedeck` listens on your network by default
(`0.0.0.0:8787`), and its startup guide prints the address to share:

```
OPEN
  Console    http://127.0.0.1:8787
  Network    http://10.0.4.21:8787
```

One Mac then becomes a **shared farm**: teammates on Linux, Windows or another Mac open the network
address and drive devices from their own browser, tests or agent — no Xcode, no Android Studio, no
local simulator on the client at all.

- **A device serves one driver at a time**, so it is a farm of *N* devices for *N* people or test
  workers. Someone else's session shows on your page as a red card naming who holds the device —
  see [the console](console.md#one-device-one-driver).
- **Automation is cheap over the network** — a test reads the lightweight DOM tree, not video; the
  pixel stream is only for a human watching.
- **Open unless you set a token.** By default anyone who can reach the address can view and drive
  the devices — fine on a trusted office or home network. Start with `devicedeck --token <secret>`
  (or `DEVICEDECK_TOKEN`) to require it: share the links the startup guide prints, which carry the
  token; a browser opens one once and is remembered. Tests add `?token=…` to the first page they
  open or send `Authorization: Bearer …`, and `devicedeck mcp --token …` for agents. It is a shared
  secret over plain HTTP, so still do not put it on the open internet (see
  [Known limits](../README.md#known-limits)).
