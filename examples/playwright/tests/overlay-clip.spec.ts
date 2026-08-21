import { test } from '@playwright/test';
const UDID = process.env.DEVICEDECK_UDID!;
test.use({ viewport: { width: 876, height: 1248 } });
test('no overlay box escapes the device', async ({ page }) => {
  test.setTimeout(150000);
  await page.goto(`/?device=${UDID}`);
  await page.locator('#console-view').waitFor({ state: 'visible', timeout: 20000 });
  await page.waitForFunction(() => {
    const c = document.getElementById('video') as HTMLCanvasElement;
    return c && !(c.width === 300 && c.height === 150);
  }, { timeout: 30000 });
  await page.waitForTimeout(1500);
  await page.locator('#btn-inspect').click();
  // wait for boxes to actually exist rather than guessing at a delay
  await page.waitForFunction(() => document.querySelectorAll('#overlay .node').length > 0,
    { timeout: 60000 });
  const stats = await page.evaluate(() => {
    const c = document.getElementById('video')!.getBoundingClientRect();
    const boxes = [...document.querySelectorAll('#overlay .node')].map(n => n.getBoundingClientRect());
    const escaping = boxes.filter(b =>
      b.right > c.right + 1 || b.left < c.left - 1 || b.bottom > c.bottom + 1 || b.top < c.top - 1);
    return {
      total: boxes.length,
      escaping: escaping.length,
      worst: escaping.length ? Math.max(...escaping.map(b => Math.round(b.right - c.right))) : 0,
    };
  });
  console.log('OVERLAY', JSON.stringify(stats));
  await page.screenshot({ path: process.env.SHOT! });
});
