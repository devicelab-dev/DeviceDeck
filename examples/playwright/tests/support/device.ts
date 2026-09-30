import { expect, type APIRequestContext, type Locator, type Page } from '@playwright/test';

// Shared setup for the device-backed specs.
//
// These used to carry readiness probes and typing retries, because a
// freshly launched app swallowed the first tap and a dropped keystroke
// surfaced a minute later as "Sign In is not enabled". Both are now the
// product's job — launch returns when the app is taking input, and the
// tree poll after an action is held until the screen settles — so the
// helpers here are the ones a web test would have anyway.

export const UDID = process.env.DEVICEDECK_UDID || 'booted';
// TestHive's id on the device's platform (an emulator's serial is
// emulator-NNNN); DEVICEDECK_APP overrides it for another app.
export const ANDROID = /^emulator-\d+$/.test(UDID);
export const APP = process.env.DEVICEDECK_APP || (ANDROID ? 'com.testhiveapp' : 'dev.devicelab.testhive');

/** launchApp puts the app in the foreground, taking input. */
export async function launchApp(request: APIRequestContext, app = APP) {
  const res = await request.post(`/api/devices/${UDID}/app/launch`, { data: { app } });
  expect(res.ok(), `launch ${app}: ${res.status()}`).toBe(true);
}

/** openDevice loads the automation page and fails loudly if input is refused. */
export async function openDevice(page: Page, app = APP) {
  await page.goto(`/device/${UDID}?app=${app}`);
  await page.locator('#mirror').waitFor({ timeout: 30_000 });
  // Reading is allowed while another client drives, so a refused claim
  // shows up as input silently going nowhere. Say so here instead.
  const refused = await page.locator('#mirror').getAttribute('data-dd-input-refused');
  expect(refused, 'another client is driving this device').toBeNull();
}

/**
 * openReady opens the device page with setup options (reset, appearance,
 * grant, link…) and waits until the page has applied them and shows the
 * app — `#mirror[data-dd-ready]`; the load event fires before that. A setup
 * step the device refused is shown in the page's notice, and fails here.
 */
export async function openReady(page: Page, device: string, query: string) {
  await page.goto(`/device/${device}?${query}`);
  await expect(page.locator('#mirror[data-dd-ready]')).toBeAttached({ timeout: 60_000 });
  expect(await page.locator('#notice').getAttribute('data-reason'), 'device setup failed').toBeNull();
}

/**
 * clearField empties a field. Relaunching does not reset app state — a
 * field can still hold what a previous session typed.
 */
export async function clearField(page: Page, field: Locator) {
  await field.fill('');
  await expect(field).toHaveValue('');
}

/** fillField types text into a field and confirms the mirror took it. */
export async function fillField(page: Page, field: Locator, text: string) {
  await field.fill(text);
  await expect(field).toHaveValue(text);
}

/** deviceValue reads what the device itself holds, not what the mirror shows. */
export async function deviceValue(request: APIRequestContext, identifier: string) {
  const tree = await (await request.get(`/api/devices/${UDID}/tree?app=${APP}`)).json();
  return tree.nodes.find((n: any) => n.identifier === identifier)?.value;
}
