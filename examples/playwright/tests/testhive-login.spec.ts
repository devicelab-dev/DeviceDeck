import { test, expect } from '@playwright/test';

// The device is a normal webpage: locate native elements with standard
// Playwright selectors against the DOM mirror, and drive the simulator
// through them. Requires `devicedeck serve` running with a booted
// simulator that has TestHive installed.

const UDID = process.env.DEVICEDECK_UDID || 'booted';
const APP = 'dev.devicelab.testhive';

// Each spec begins from the app's first screen. Without this they share
// one device and the second to run inherits wherever the first left it —
// which is exactly how running the suite together failed while each spec
// passed alone.
test.beforeEach(async ({ request }) => {
  await request.post(`/api/devices/${UDID}/app/launch`, { data: { app: APP } });
  await new Promise((r) => setTimeout(r, 2500));
});

test('logs into TestHive on a real simulator', async ({ page }) => {
  await page.goto(`/device/${UDID}?app=${APP}`);

  // First mirror render needs the tree engine warm; give it time.
  const username = page.getByTestId('username-input');
  await expect(username).toBeVisible({ timeout: 30_000 });

  // A freshly launched app enters the accessibility tree about a second
  // before it starts accepting touches: XCUITest reports the field
  // hittable, enabled and geometrically stable that whole time, so there
  // is no device-side state to wait on. Prove the field really takes
  // input, then clear the probe character and type for real.
  // Relaunching does not reset app state — the field can still hold what
  // a previous session typed — so clear it, then prove the app is taking
  // input at all. A freshly launched app is in the accessibility tree
  // about a second before it accepts touches, and reports itself
  // hittable and stable throughout, so there is nothing to wait on but
  // the effect itself.
  await expect(async () => {
    await username.click();
    for (let i = 0; i < 40 && (await username.inputValue()); i++) {
      await page.keyboard.press('Backspace');
    }
    await page.keyboard.press('x');
    await expect(username).toHaveValue('x');
  }).toPass({ timeout: 30_000 });
  await page.keyboard.press('Backspace');

  // Keys forward as real HID presses (~100ms hold each) — pace the typing.
  await page.keyboard.type('devicelab', { delay: 150 });

  await page.getByTestId('password-input').click();
  await page.keyboard.type('robustest', { delay: 150 });

  await page.getByTestId('login-button').click();

  // Post-login home screen: mirror text comes from native labels.
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();
});
