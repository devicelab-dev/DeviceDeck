import { test } from '@playwright/test';
const UDID = process.env.DEVICEDECK_UDID!;
test.use({ viewport: { width: 806, height: 1246 } });
test('console layout at a narrow width', async ({ page }) => {
  test.setTimeout(120000);
  await page.goto(`/?device=${UDID}`);
  await page.locator('#console-view').waitFor({ state: 'visible', timeout: 20000 });
  await page.waitForTimeout(2000);
  console.log('BEFORE inspect', JSON.stringify(await page.evaluate(() => ({
    scrollX: window.scrollX,
    bodyScrollLeft: document.body.scrollLeft,
    headerWidth: document.querySelector('header')!.scrollWidth,
    viewport: window.innerWidth,
    stageX: document.getElementById('stage')!.getBoundingClientRect().x,
  }))));
  await page.locator('#btn-inspect').click();
  await page.waitForTimeout(4000);
  console.log('AFTER inspect ', JSON.stringify(await page.evaluate(() => ({
    scrollX: window.scrollX,
    bodyScrollLeft: document.body.scrollLeft,
    headerWidth: document.querySelector('header')!.scrollWidth,
    viewport: window.innerWidth,
    stageX: document.getElementById('stage')!.getBoundingClientRect().x,
  }))));
});
