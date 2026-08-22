import { test } from '@playwright/test';
const UDID = process.env.DEVICEDECK_UDID!;
test.use({ viewport: { width: 876, height: 1248 } });
test('inspector on the products screen', async ({ page, request }) => {
  test.setTimeout(150000);
  await request.post(`/api/devices/${UDID}/app/launch`, { data: { app: 'dev.devicelab.testhive' } });
  await page.waitForTimeout(3000);
  await page.goto(`/?device=${UDID}`);
  await page.locator('#console-view').waitFor({ state: 'visible', timeout: 20000 });
  await page.waitForFunction(() => {
    const c = document.getElementById('video') as HTMLCanvasElement;
    return c && !(c.width === 300 && c.height === 150);
  }, { timeout: 30000 });
  await page.waitForTimeout(1500);
  // log in so the inspector runs against the scrollable product list
  const u = page.locator('#video');
  await u.click({ position: { x: 200, y: 430 } });
  await page.waitForTimeout(1200);
  await page.keyboard.type('devicelab', { delay: 60 });
  await u.click({ position: { x: 200, y: 505 } });
  await page.waitForTimeout(1000);
  await page.keyboard.type('robustest', { delay: 60 });
  await u.click({ position: { x: 200, y: 585 } });
  await page.waitForTimeout(4000);
  await page.locator('#btn-inspect').click();
  await page.waitForTimeout(6000);
  const stats = await page.evaluate(() => {
    const c = document.getElementById('video')!.getBoundingClientRect();
    const boxes = [...document.querySelectorAll('#overlay .node')].map(n => n.getBoundingClientRect());
    const outside = boxes.filter(b => b.right > c.right + 1 || b.left < c.left - 1 || b.bottom > c.bottom + 1);
    return { total: boxes.length, escapingDevice: outside.length };
  });
  console.log('OVERLAY', JSON.stringify(stats));
  await page.screenshot({ path: process.env.SHOT! });
});
