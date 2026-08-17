# Playwright against a real simulator

The device page (`/device/{udid}?app={bundleId}`) renders the simulator's
video with the native UI tree mirrored as real DOM — `data-testid` from
accessibility identifiers, ARIA roles from element types, accessible text
from labels. Playwright drives it like any webpage; no Appium, no
WebDriverAgent, no new API.

```bash
# terminal 1: a booted simulator with TestHive installed
devicedeck serve

# terminal 2:
npm install
npx playwright install chromium
npm test
```

Notes:

- Native gestures DOM events can't express are available from scripts:
  `page.evaluate(() => devicedeck.swipe(0.5, 0.8, 0.5, 0.2))`.
- Typing forwards keys as real HID presses; use `{ delay: 150 }` with
  `page.keyboard.type`.
- The mirror refreshes from the native tree (~300ms while active), so
  Playwright's auto-waiting works — new screens appear on the next
  refresh rather than instantly.
