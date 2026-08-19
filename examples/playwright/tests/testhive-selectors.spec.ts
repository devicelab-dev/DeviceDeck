import { test, expect } from '@playwright/test';

// The device page's whole promise is that ordinary web selectors work
// against a native app. Our other specs use getByTestId, which exercises
// only one of them — and that blind spot let a bug ship where typing
// renamed a field and broke every role- and label-based query without a
// single test noticing. This spec drives the same journey using the
// query styles a real user reaches for first.

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

test('drives TestHive by role, label and text', async ({ page }) => {
  await page.goto(`/device/${UDID}?app=${APP}`);

  const username = page.getByRole('textbox', { name: 'Username' });
  await expect(username).toBeVisible({ timeout: 30_000 });

  // Sign In is disabled until credentials exist — the mirror has to
  // report native enablement, not just draw a button.
  const signIn = page.getByRole('button', { name: 'Sign In' });
  await expect(signIn).toBeDisabled();

  // Prove the field accepts input before typing into it: a freshly
  // launched app is in the accessibility tree about a second before it
  // takes touches, and reports itself hittable and stable throughout.
  // Relaunching does not reset app state — the field can still hold what
  // a previous session typed — so clear it, then prove the app is taking
  // input at all. A freshly launched app is in the accessibility tree
  // about a second before it accepts touches, and reports itself
  // hittable and stable throughout, so there is nothing to wait on but
  // the effect itself.
  await expect(async () => {
    await username.click();
    for (let i = 0; i < 40 && (await username.textContent()); i++) {
      await page.keyboard.press('Backspace');
    }
    await page.keyboard.press('x');
    await expect(username).toHaveText('x');
  }).toPass({ timeout: 30_000 });
  await page.keyboard.press('Backspace');

  await page.keyboard.type('devicelab', { delay: 150 });
  // The name must survive typing: it comes from what the field asks for,
  // never from what has been entered.
  await expect(page.getByRole('textbox', { name: 'Username' })).toHaveCount(1);

  await page.getByRole('textbox', { name: 'Password' }).click();
  await page.keyboard.type('robustest', { delay: 150 });

  await expect(signIn).toBeEnabled();
  await signIn.click();

  await expect(page.getByText('Hello, devicelab!')).toBeVisible();
});
