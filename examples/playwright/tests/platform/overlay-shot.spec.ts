import { test } from '@playwright/test';
const UDID = process.env.DEVICEDECK_UDID!;
test.use({ viewport: { width: 806, height: 1246 } });
test('inspector overlay screenshot', async ({ page, request }) => {
  test.setTimeout(120000);
  await request.post(`/api/devices/${UDID}/app/launch`, { data: { app: 'dev.devicelab.testhive' } });
  await page.waitForTimeout(3000);
  await page.goto(`/?device=${UDID}`);
  await page.locator('#console-view').waitFor({ state: 'visible', timeout: 20000 });
  await page.waitForFunction(() => {
    const c = document.getElementById('video') as HTMLCanvasElement;
    return c && !(c.width === 300 && c.height === 150);
  }, { timeout: 30000 });
  await page.waitForTimeout(1500);
  await page.locator('#btn-inspect').click();
  await page.waitForTimeout(6000);
  await page.screenshot({ path: process.env.SHOT!, fullPage: false });
  const g = await page.evaluate(() => {
    const s = document.getElementById('stage')!.getBoundingClientRect();
    const p = document.getElementById('panel')!.getBoundingClientRect();
    const c = document.getElementById('video')!.getBoundingClientRect();
    return { stage: [s.x, s.width], panel: [p.x, p.width], canvas: [c.x, c.width] };
  });
  console.log('LAYOUT', JSON.stringify(g));
});
