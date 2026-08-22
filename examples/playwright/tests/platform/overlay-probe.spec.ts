import { test } from '@playwright/test';
const UDID = process.env.DEVICEDECK_UDID!;
test.use({ viewport: { width: 806, height: 1246 } });

test('inspector overlay geometry', async ({ page, request }) => {
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

  const info = await page.evaluate(() => {
    const c = document.getElementById('video') as HTMLCanvasElement;
    const o = document.getElementById('overlay')!;
    const cr = c.getBoundingClientRect(), or = o.getBoundingClientRect();
    const node = document.querySelector('#overlay .node[data-testid="username-input"]') as HTMLElement;
    return {
      canvasCSS: { x: +cr.x.toFixed(1), y: +cr.y.toFixed(1), w: +cr.width.toFixed(1), h: +cr.height.toFixed(1) },
      canvasBacking: { w: c.width, h: c.height },
      overlayCSS: { x: +or.x.toFixed(1), y: +or.y.toFixed(1), w: +or.width.toFixed(1), h: +or.height.toFixed(1) },
      nodeBox: node ? (() => { const r = node.getBoundingClientRect();
        return { x: +r.x.toFixed(1), y: +r.y.toFixed(1), w: +r.width.toFixed(1), h: +r.height.toFixed(1),
                 styleLeft: node.style.left, styleWidth: node.style.width }; })() : null,
      overlayChildren: document.querySelectorAll('#overlay .node').length,
    };
  });
  console.log('GEOM', JSON.stringify(info, null, 1));

  const tree = await (await request.get(`http://127.0.0.1:8787/api/devices/${UDID}/tree`)).json();
  const app = tree.nodes[0].frame;
  const u = tree.nodes.find((n: any) => n.identifier === 'username-input');
  console.log('APP FRAME', JSON.stringify(app));
  console.log('USERNAME FRAME', JSON.stringify(u?.frame));
  if (u) {
    console.log('EXPECTED pct  left', (u.frame.x / app.width * 100).toFixed(1) + '%',
                'width', (u.frame.width / app.width * 100).toFixed(1) + '%');
  }
});
