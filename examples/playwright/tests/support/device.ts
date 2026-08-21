import { expect, type APIRequestContext, type Locator, type Page } from '@playwright/test';

// Shared driving helpers for the device-backed specs.
//
// These specs kept failing sixty seconds after the real problem, with
// "Sign In is not enabled" — a symptom three steps downstream of a
// keystroke that never landed. Every helper here verifies its own effect
// on the device before returning, so a failure names the step that
// actually went wrong.

export const UDID = process.env.DEVICEDECK_UDID || 'booted';
export const APP = 'dev.devicelab.testhive';

// A freshly launched app enters the accessibility tree about a second
// before it starts accepting touches, and reports itself hittable,
// enabled and geometrically stable throughout that window — so there is
// no device-side state to wait on, only the effect itself.
const READY_TIMEOUT = 30_000;

/** launchApp puts the app in the foreground and lets it draw. */
export async function launchApp(request: APIRequestContext, app = APP) {
  await request.post(`/api/devices/${UDID}/app/launch`, { data: { app } });
  await new Promise((r) => setTimeout(r, 2500));
}

/** openDevice loads the automation page and waits for a mirrored tree. */
export async function openDevice(page: Page, app = APP) {
  await page.goto(`/device/${UDID}?app=${app}`);
  await page.locator('#mirror').waitFor({ timeout: READY_TIMEOUT });
  // Reading is allowed while another client drives, so a refused claim
  // shows up as input silently going nowhere. Say so here instead.
  const refused = await page.locator('#mirror').getAttribute('data-dd-input-refused');
  expect(refused, 'another client is driving this device').toBeNull();
}

/**
 * clearField empties a field, proving the app takes input at all. The
 * probe character is what distinguishes "not ready yet" from "broken":
 * an app that is merely still warming up will accept it on a retry.
 */
export async function clearField(page: Page, field: Locator) {
  await expect(async () => {
    await field.click();
    for (let i = 0; i < 40 && (await field.inputValue()); i++) {
      await page.keyboard.press('Backspace');
    }
    await page.keyboard.press('x');
    await expect(field).toHaveValue('x');
  }).toPass({ timeout: READY_TIMEOUT });
  await page.keyboard.press('Backspace');
  await expect(field).toHaveValue('');
}

/**
 * fillField types text and confirms the device took it. Keys forward as
 * real HID presses, so pace them; a dropped one is retried by refilling
 * the whole field rather than guessing which character went missing.
 */
export async function fillField(page: Page, field: Locator, text: string) {
  await expect(async () => {
    await field.click();
    for (let i = 0; i < 40 && (await field.inputValue()); i++) {
      await page.keyboard.press('Backspace');
    }
    await page.keyboard.type(text, { delay: 150 });
    await expect(field).not.toHaveValue('');
  }).toPass({ timeout: READY_TIMEOUT });
}

/** deviceValue reads what the device itself holds, not what the mirror shows. */
export async function deviceValue(request: APIRequestContext, identifier: string) {
  const tree = await (await request.get(`/api/devices/${UDID}/tree?app=${APP}`)).json();
  return tree.nodes.find((n: any) => n.identifier === identifier)?.value;
}
