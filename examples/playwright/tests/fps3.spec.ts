import { test } from '@playwright/test';
test('android fps after downscale', async ({ page }) => {
  test.setTimeout(180000);
  const frames: { type: number; len: number }[] = [];
  page.on('websocket', ws => {
    if (!ws.url().includes('/video')) return;
    ws.on('framereceived', f => { const b = f.payload as Buffer; frames.push({ type: b[0], len: b.length }); });
  });
  await page.goto('/?device=emulator-5554');
  await page.locator('#console-view').waitFor({ state: 'visible', timeout: 20000 });
  await page.waitForFunction(() => {
    const c = document.getElementById('video') as HTMLCanvasElement;
    return c && !(c.width === 300 && c.height === 150);
  }, { timeout: 40000 });
  await page.waitForTimeout(1500);
  console.log('CANVAS backing:', JSON.stringify(await page.evaluate(() => {
    const c = document.getElementById('video') as HTMLCanvasElement; return { w: c.width, h: c.height };
  })));

  frames.length = 0;
  await page.waitForTimeout(5000);
  console.log(`IDLE: ${(frames.length/5).toFixed(1)} fps, avg ${frames.length?Math.round(frames.reduce((a,f)=>a+f.len,0)/frames.length/1024):0} KB`);

  const box = (await page.locator('#video').boundingBox())!;
  const mid = { x: box.x + box.width/2, y: box.y + box.height/2 };
  frames.length = 0;
  const t0 = Date.now();
  for (let i = 0; i < 6; i++) {
    await page.mouse.move(mid.x, mid.y + 150); await page.mouse.down();
    for (let s = 0; s < 6; s++) await page.mouse.move(mid.x, mid.y + 150 - s*40, { steps: 2 });
    await page.mouse.up(); await page.waitForTimeout(300);
  }
  await page.waitForTimeout(1500);
  const secs = (Date.now()-t0)/1000;
  console.log(`MOTION: ${(frames.length/secs).toFixed(1)} fps over ${secs.toFixed(1)}s, avg ${frames.length?Math.round(frames.reduce((a,f)=>a+f.len,0)/frames.length/1024):0} KB`);
});
