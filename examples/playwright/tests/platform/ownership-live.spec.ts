import { test, expect } from '@playwright/test';
const UDID = process.env.DEVICEDECK_UDID!;
const APP = 'dev.devicelab.testhive';

// Two browser contexts, one device: the second must be refused with an
// explanation rather than silently interleaving its touches.
test('a second driver is refused on a real device', async ({ browser, request }) => {
  test.setTimeout(120000);
  await request.post(`/api/devices/${UDID}/app/launch`, { data: { app: APP } });
  await new Promise((r) => setTimeout(r, 3000));
  const first = await browser.newPage();
  await first.goto(`/device/${UDID}?app=${APP}`);
  await first.getByTestId('username-input').waitFor({ state: 'visible', timeout: 30000 });
  // Drive it so the first page unambiguously owns input.
  await first.getByTestId('username-input').click();
  await first.waitForTimeout(1500);

  const second = await browser.newPage();
  await second.goto(`/device/${UDID}?app=${APP}`);
  await second.getByTestId('username-input').waitFor({ state: 'visible', timeout: 30000 });
  // The mirror still renders — reading is allowed — but input is refused.
  await expect(second.locator('#mirror')).toHaveAttribute('data-dd-input-refused', /driven by/, { timeout: 20000 });
  const reason = await second.locator('#mirror').getAttribute('data-dd-input-refused');
  console.log('SECOND PAGE REFUSED WITH:', reason);
  console.log('SECOND PAGE TITLE:', await second.title());

  // The refusal must not have cost the first page its device. Proven by
  // driving it, not by assuming: a freshly launched app takes a moment
  // to accept touches, so retry until the effect appears.
  const field = first.getByTestId('username-input');
  await expect(async () => {
    await field.click();
    await first.keyboard.press('q');
    await expect(field).toHaveValue(/q/);
  }).toPass({ timeout: 25_000 });
  console.log('FIRST PAGE still drives, field =', JSON.stringify(await field.inputValue()));

  await second.close();
  await first.close();
});
