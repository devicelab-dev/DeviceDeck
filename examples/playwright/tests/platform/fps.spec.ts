import { test } from '@playwright/test';
test('android fps', async ({ page }) => {
  test.setTimeout(120000);
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
  await page.waitForTimeout(1000);
  frames.length = 0;
  const box = (await page.locator('#video').boundingBox())!;
  const mid = { x: box.x + box.width/2, y: box.y + box.height/2 };
  for (let i = 0; i < 4; i++) {
    await page.mouse.move(mid.x, mid.y + 150); await page.mouse.down();
    for (let s = 0; s < 8; s++) await page.mouse.move(mid.x, mid.y + 150 - s*30, { steps: 2 });
    await page.mouse.up(); await page.waitForTimeout(400);
  }
  await page.waitForTimeout(1000);
  const secs = 4 * 0.4 + 1 + 1.5;
  const types = frames.reduce((m: any, f) => { m['0x'+f.type.toString(16)] = (m['0x'+f.type.toString(16)]||0)+1; return m; }, {});
  console.log(`RESULT frames=${frames.length} over ~${secs}s => ${(frames.length/secs).toFixed(1)} fps | avgKB=${frames.length?Math.round(frames.reduce((a,f)=>a+f.len,0)/frames.length/1024):0} | types=${JSON.stringify(types)}`);
});
