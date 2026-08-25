import { test, expect } from '@playwright/test';

// Same journey, same selectors, other platform: the device page mirrors
// the Android emulator's native tree, so this reads identically to the
// iOS spec — React Native's testID reaches both mirrors as data-testid.
// Requires `devicedeck serve`, a running emulator with TestHive
// installed, and DEVICEDECK_ANDROID_SERIAL (e.g. emulator-5554).

const SERIAL = process.env.DEVICEDECK_ANDROID_SERIAL;
const APP = 'com.testhiveapp';

test.skip(!SERIAL, 'set DEVICEDECK_ANDROID_SERIAL to run the Android variant');

// Start from the app's first screen rather than inheriting whatever the
// device was last left showing — which, after a reboot, is the launcher.
test.beforeEach(async ({ request }) => {
  await request.post(`/api/devices/${SERIAL}/app/launch`, { data: { app: APP } });
  await new Promise((r) => setTimeout(r, 3000));
});

// deviceValue reads what the emulator itself holds, not what the mirror
// shows — the check that catches the mirror claiming text the device never
// received.
async function deviceValue(request: any, identifier: string) {
  const tree = await (await request.get(`/api/devices/${SERIAL}/tree?app=${APP}`)).json();
  return tree.nodes.find((n: any) => n.identifier === identifier)?.value;
}

test('logs into TestHive on an Android emulator', async ({ page, request }) => {
  await page.goto(`/device/${SERIAL}`);

  const username = page.getByTestId('username-input');
  await expect(username).toBeVisible({ timeout: 30_000 });

  await username.fill('devicelab');
  await page.getByTestId('password-input').fill('robustest');

  // Assert against the device, not the mirror: fill() returns once the
  // mirror has the text, but the keystrokes are still on their way to the
  // emulator behind it. Submitting before they land clicks Sign In on an
  // empty form. Poll the device until it echoes the value.
  await expect.poll(() => deviceValue(request, 'username-input')).toBe('devicelab');

  await page.getByTestId('login-button').click();

  await expect(page.getByText('Hello, devicelab!')).toBeVisible({ timeout: 15_000 });
});
