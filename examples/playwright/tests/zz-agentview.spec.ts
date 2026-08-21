import { test } from '@playwright/test';
const UDID = '56B22693-2704-4861-8DCF-9DD2ED0483FA';
test('what an agent now sees', async ({ page }) => {
  await page.goto(`/device/${UDID}?app=dev.devicelab.testhive`);
  await page.waitForTimeout(6000);
  console.log('TITLE: ' + await page.title());
  console.log('mirror aria-label: ' + await page.getAttribute('#mirror','aria-label'));
  const t = await (await page.request.get(`/api/devices/${UDID}/tree`)).json();
  console.log('tree payload has hash: ' + JSON.stringify(t.hash));
  const snap = await page.locator('#stage').ariaSnapshot();
  console.log('--- snapshot (controls only) ---');
  for (const l of snap.split('\n')) {
    if (/application |button |textbox |searchbox |link /.test(l)) console.log('  ' + l.trim());
  }
});
