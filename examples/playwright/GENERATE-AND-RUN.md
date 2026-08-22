# Generate a test by agent, then run it

The product loop a user follows: ask an agent to write a test for an app
on the device, then run it. Two phases, one device at a time.

```sh
# 1. The agent drives the app over MCP and writes the test from the
#    durable locators playwright-mcp emits for each action it takes.
DEVICEDECK_UDID=<udid> node tests/support/generate.mjs

# 2. Playwright runs the generated test against the device.
DEVICEDECK_UDID=<udid> npx playwright test generated-checkout
```

Phase 1 needs `devicedeck serve` running with the app installed on a
booted simulator; it closes its MCP session before exiting, so phase 2
has the device to itself. The generated test carries no sleeps: the tree
poll after each action is held until the screen settles, so the locators
resolve without timing hacks.
