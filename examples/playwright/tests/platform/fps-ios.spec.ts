import { test } from '@playwright/test';
const UDID = process.env.DEVICEDECK_UDID!;
test('ios fps timeline', async ({ page }) => {
  test.setTimeout(240000);
  const start = Date.now();
  const frames: { t: number; type: number; len: number }[] = [];
  page.on('websocket', ws => {
    if (!ws.url().includes('/video')) return;
    ws.on('framereceived', f => { const b = f.payload as Buffer; frames.push({ t: Date.now()-start, type: b[0], len: b.length }); });
  });
  await page.goto(`/?device=${UDID}`);
  await page.locator('#console-view').waitFor({ state: 'visible', timeout: 20000 });
  await page.waitForTimeout(3000);
  const box = (await page.locator('#video').boundingBox())!;
  const mid = { x: box.x + box.width/2, y: box.y + box.height/2 };
  frames.length = 0;
  const t0 = Date.now();
  const deadline = Date.now() + 20000;
  while (Date.now() < deadline) {
    await page.mouse.move(mid.x, mid.y + 120);
    await page.mouse.down();
    for (let s = 0; s < 5; s++) await page.mouse.move(mid.x, mid.y + 120 - s*40, { steps: 2 });
    await page.mouse.up();
    await page.waitForTimeout(150);
  }
  const secs = (Date.now()-t0)/1000;
  const types = frames.reduce((m: any, f) => { const k='0x'+f.type.toString(16); m[k]=(m[k]||0)+1; return m; }, {});
  console.log(`RESULT ${frames.length} frames / ${secs.toFixed(1)}s => ${(frames.length/secs).toFixed(1)} fps | types=${JSON.stringify(types)} | avgKB=${frames.length?Math.round(frames.reduce((a,f)=>a+f.len,0)/frames.length/1024):0}`);
});
