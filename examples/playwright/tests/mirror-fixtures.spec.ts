import { test, expect, type Page } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

// What an agent reads of a real screen, tested without a device.
//
// The tree payloads in fixtures/ were recorded from TestHive on a booted
// simulator, so these are real screens with their real quirks — the
// keyboard's wrapper chains, a control split across a twin pair, a field
// mid-edit. The page itself is the one that ships: device.html and
// device.js are served from disk and the tree endpoint is stubbed, so
// the whole read path runs unchanged with nothing mocked inside it.
//
// The device-backed specs prove the mirror matches a live app. These
// prove what the mirror makes of a given tree, which is the part that
// has to keep working for every agent, and the part no simulator is
// needed to check.

const STATIC = path.join(__dirname, '../../../internal/web/static');
const FIXTURES = path.join(__dirname, 'fixtures');
const UDID = '00000000-0000-0000-0000-000000000000';

const fixture = (name: string) =>
  JSON.parse(fs.readFileSync(path.join(FIXTURES, `${name}.json`), 'utf8'));

const TYPES: Record<string, string> = {
  '.js': 'application/javascript', '.html': 'text/html', '.svg': 'image/svg+xml',
};

/** serveMirror runs the real page against a recorded tree. */
async function serveMirror(page: Page, name: string) {
  const tree = fixture(name);

  // The sockets carry video and input; neither is under test, and left
  // unrouted they retry on a timer and fill the log with noise.
  await page.routeWebSocket(/\/(video|input)$/, () => {});

  await page.route('**/api/devices', (r) =>
    r.fulfill({ json: { devices: [{ udid: UDID, name: 'Fixture', booted: true }] } }));
  await page.route('**/api/devices/*/tree*', (r) => r.fulfill({ json: tree }));
  await page.route(/\/device\/[0-9A-Fa-f-]+/, (r) =>
    r.fulfill({ contentType: 'text/html', body: fs.readFileSync(path.join(STATIC, 'device.html'), 'utf8') }));
  await page.route(/\.(js|svg)$/, (r) => {
    const file = path.join(STATIC, path.basename(new URL(r.request().url()).pathname));
    if (!fs.existsSync(file)) return r.fulfill({ status: 404, body: '' });
    return r.fulfill({ contentType: TYPES[path.extname(file)], body: fs.readFileSync(file, 'utf8') });
  });

  await page.goto(`http://127.0.0.1:8787/device/${UDID}?app=dev.devicelab.testhive`);
  await expect(page.locator('#mirror [data-dd-node]').first()).toBeVisible({ timeout: 15_000 });
  return tree;
}

const agentView = (page: Page) => page.locator('#stage').ariaSnapshot({ mode: 'ai' });

test('a control carries its identifier where an agent can read it', async ({ page }) => {
  await serveMirror(page, 'login');
  const snap = await agentView(page);
  expect(snap).toContain('textbox "Username (username-input)"');
  expect(snap).toContain('button "Sign In (login-button)"');
  // Native enablement, not just a drawn button.
  expect(snap).toMatch(/button "Sign In \(login-button\)" \[disabled\]/);
});

test('every Add button on the products screen is distinguishable', async ({ page }) => {
  await serveMirror(page, 'products');
  const snap = await agentView(page);
  const adds = snap.split('\n').filter((l) => /button "Add \(/.test(l)).map((l) => l.trim());
  expect(adds.length).toBeGreaterThan(1);
  // The whole point: an agent asked for one product must not be choosing
  // between identical names.
  expect(new Set(adds).size).toBe(adds.length);
});

test('the wrapper chains of a real keyboard do not repeat themselves', async ({ page }) => {
  await serveMirror(page, 'login');
  const snap = await agentView(page);
  const named = snap.split('\n')
    .map((l) => (l.match(/generic "([^"]+)"/) || [])[1])
    .filter(Boolean) as string[];
  const repeated = named.filter((n, i) => named.indexOf(n) !== i);
  expect(repeated, `these names print more than once: ${JSON.stringify(repeated)}`).toEqual([]);
});

test('the screen fingerprint names the root and changes with the screen', async ({ page }) => {
  const login = await serveMirror(page, 'login');
  expect(await page.getAttribute('#mirror', 'aria-label'))
    .toBe(`device screen ${login.hash.slice(0, 8)}`);

  const products = fixture('products');
  expect(products.hash).not.toBe(login.hash);
  await page.route('**/api/devices/*/tree*', (r) => r.fulfill({ json: products }));
  await expect(page.locator('#mirror'))
    .toHaveAttribute('aria-label', `device screen ${products.hash.slice(0, 8)}`, { timeout: 15_000 });
});

test('a text field mirrors as a real input an agent can type into', async ({ page }) => {
  await serveMirror(page, 'login-typed');
  const username = page.getByTestId('username-input');
  await expect(username).toHaveJSProperty('tagName', 'INPUT');
  // Named by what it asks for, never by what has been entered.
  expect(await agentView(page)).toContain('textbox "Username (username-input)"');
  expect(await username.inputValue()).toBe('devicelab');
});

test('a contested identifier lands on the element you would act on', async ({ page }) => {
  await serveMirror(page, 'products-cart-full');
  // The app puts cart-button on the icon and on the badge counting the
  // cart's contents, so this query matched two elements and failed as a
  // strict-mode violation the moment anything was in the cart.
  const cart = page.getByTestId('cart-button');
  await expect(cart).toHaveCount(1);
  // It has to be the icon, not the badge: the badge sits inside it and
  // is not what a click is aimed at.
  expect(await cart.getAttribute('aria-label')).toBe('Shopping Cart');
  // The badge is still there to be read, just not by that identifier.
  expect(await agentView(page)).toContain('1');
});

test('a field beside a same-named icon keeps the identifier', async ({ page }) => {
  await serveMirror(page, 'products');
  // search-bar is on both the search glass and the field next to it.
  // Neither contains the other, so the control wins.
  const search = page.getByTestId('search-bar');
  await expect(search).toHaveCount(1);
  await expect(search).toHaveJSProperty('tagName', 'INPUT');
});

test('an identifier the app genuinely repeats is left alone', async ({ page }) => {
  await serveMirror(page, 'products');
  // Six product rows each reuse star.fill for their favourite icon.
  // These are six real controls on six different products: picking one
  // would hide five elements that exist. The ambiguity is the app's,
  // and the mirror must not paper over it.
  await expect(page.getByTestId('star.fill')).toHaveCount(6);
});

test('a row below the fold is in the mirror but clipped, not clickable off the device', async ({ page }) => {
  await serveMirror(page, 'products');
  // XCUITest reports rows below the screen with real frames. Unclipped,
  // this one was hit-testable below the device and a click on it tapped
  // the device's bottom edge instead.
  const below = page.getByTestId('product-name-6');
  await expect(below).toBeAttached();
  const mirror = (await page.locator('#mirror').boundingBox())!;
  const box = (await below.boundingBox())!;
  expect(box.y).toBeGreaterThan(mirror.y + mirror.height);
  // The mirror clips it: nothing at that point belongs to the mirror.
  const hit = await page.evaluate(([x, y]) =>
    document.elementFromPoint(x, y)?.closest('#mirror') !== null, [box.x + 1, box.y + 1] as [number, number]);
  expect(hit).toBe(false);
});

test('the mirror really scrolls, and forgets the offset when the screen changes', async ({ page }) => {
  const products = await serveMirror(page, 'products');
  const mirror = page.locator('#mirror');
  const below = page.getByTestId('product-name-6');
  const box = (await mirror.boundingBox())!;
  expect((await below.boundingBox())!.y).toBeGreaterThan(box.y + box.height);

  // What scrollIntoView and cy.scrollTo do: write the offset. The
  // mirror keeps it — it is the tool's view transform — and the row
  // below the fold is now inside the box, where a tool can hit-test it.
  await below.scrollIntoViewIfNeeded();
  expect(await page.evaluate(() => document.getElementById('mirror')!.scrollTop)).toBeGreaterThan(0);
  const scrolled = (await below.boundingBox())!;
  expect(scrolled.y + scrolled.height).toBeLessThanOrEqual(box.y + box.height + 1);

  // A new screen arrives laid out against the real device, so the
  // offset goes back to zero rather than shifting every click on it.
  const cart = fixture('cart');
  expect(cart.hash).not.toBe(products.hash);
  await page.route('**/api/devices/*/tree*', (r) => r.fulfill({ json: cart }));
  await expect.poll(() => page.evaluate(() => document.getElementById('mirror')!.scrollTop), { timeout: 15_000 }).toBe(0);
});

test('nothing takes a click while the barrier is open', async ({ page }) => {
  await serveMirror(page, 'login');
  const signIn = page.getByTestId('forgot-password-button');
  await page.evaluate(() => document.getElementById('mirror')!.setAttribute('data-dd-settled', 'false'));
  // Playwright refuses to click an element whose box is moving and
  // retries until it holds still — which is the whole mechanism. The
  // snapshot is untouched: the element keeps its ref throughout.
  await expect(signIn.click({ timeout: 800 })).rejects.toThrow(/not stable/);
  expect(await page.locator('#stage').ariaSnapshot({ mode: 'ai' })).toMatch(/button "Forgot Password[^"]*" \[ref=/);
  await page.evaluate(() => document.getElementById('mirror')!.setAttribute('data-dd-settled', 'true'));
  await signIn.click({ timeout: 2000 });
});
