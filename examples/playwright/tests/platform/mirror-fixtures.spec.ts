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

const STATIC = path.join(__dirname, '../../../../internal/web/static');
const FIXTURES = path.join(__dirname, '../fixtures');
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
  // Attached — a scroll-to can still find it — but past the mirror's
  // clip: its top sits below the mirror's own bottom edge.
  await expect(below).toBeAttached();
  const mirror = (await page.locator('#mirror').boundingBox())!;
  const box = (await below.boundingBox())!;
  expect(box.y).toBeGreaterThan(mirror.y + mirror.height);
  // And the mirror clips: overflow hidden means nothing past its bottom
  // is drawn or hit-testable, so the row cannot be clicked off-device.
  expect(await page.locator('#mirror').evaluate((m) => getComputedStyle(m).overflow)).toBe('hidden');
});

test('the mirror really scrolls, and forgets the offset when the screen changes', async ({ page }) => {
  const products = await serveMirror(page, 'products');
  const mirror = page.locator('#mirror');
  const below = page.getByTestId('product-name-6');
  const box = (await mirror.boundingBox())!;
  expect((await below.boundingBox())!.y).toBeGreaterThan(box.y + box.height);

  // The view rests one gutter in — a screen's height — so there is room
  // to scroll up as well as down.
  const scrollTop = () => page.evaluate(() => document.getElementById('mirror')!.scrollTop);
  const rest = Math.round(box.height);
  expect(Math.abs((await scrollTop()) - rest)).toBeLessThanOrEqual(1);

  // What scrollIntoView and cy.scrollTo do: write the offset. The
  // mirror keeps it — it is the tool's view transform — and the row
  // below the fold is now inside the box, where a tool can hit-test it.
  await below.scrollIntoViewIfNeeded();
  expect(await scrollTop()).toBeGreaterThan(rest + 1);
  const scrolled = (await below.boundingBox())!;
  expect(scrolled.y + scrolled.height).toBeLessThanOrEqual(box.y + box.height + 1);

  // A new screen arrives laid out against the real device, so the
  // offset goes back to rest rather than shifting every click on it.
  const cart = fixture('cart');
  expect(cart.hash).not.toBe(products.hash);
  await page.route('**/api/devices/*/tree*', (r) => r.fulfill({ json: cart }));
  await expect.poll(async () => Math.abs((await scrollTop()) - rest) <= 1, { timeout: 15_000 }).toBe(true);
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

test('the canvas takes the device dimensions from the tree, with no video', async ({ page }) => {
  const tree = await serveMirror(page, 'products');
  const app = tree.nodes[0].frame;
  // No video stream is opened on the automation page; the mirror still
  // needs a correctly-proportioned canvas, and the tree already carries
  // the device's size. Backing store matches the app frame.
  const size = await page.locator('#video').evaluate((c) => ({ w: c.width, h: c.height }));
  expect(size.w).toBe(Math.round(app.width));
  expect(size.h).toBe(Math.round(app.height));
});

test('a backgrounded app is announced in the root name, not hidden', async ({ page }) => {
  const tree = await serveMirror(page, 'products');
  // The same screen, now reported backgrounded. iOS keeps a backgrounded
  // app's whole tree, so every control is still here — the mirror must
  // annotate, not hide, so an agent knows the screen is not on the device.
  await page.route('**/api/devices/*/tree*', (r) =>
    r.fulfill({ json: { ...tree, foreground: false, appState: 'runningBackground' } }));
  await expect(page.locator('#mirror')).toHaveAttribute('aria-label', /app not in foreground/, { timeout: 5_000 });
  await expect(page.locator('#mirror')).toHaveAttribute('data-dd-foreground', 'false');
  // Annotated, never hidden: the controls are all still present and clickable.
  await expect(page.locator('#mirror [data-dd-node]').first()).toBeVisible();
});

// States: the attributes ARIA carries for a role, read from the device.
// Without them a switch reads as a bare button and a slider as text; an
// agent can find the control but not tell what it is set to.
test('checkable, range and selected states reach the snapshot on the roles that own them', async ({ page }) => {
  await serveMirror(page, 'states');
  const snap = await agentView(page);
  expect(snap).toMatch(/switch "Wi-Fi \(wifi-switch\)" \[checked\]/);
  expect(snap).toMatch(/switch "Bluetooth \(bt-switch\)"(?! \[checked\])/);
  await expect(page.getByTestId('volume')).toHaveAttribute('aria-valuenow', '50');
  await expect(page.getByTestId('volume')).toHaveAttribute('aria-valuetext', '50%');
  await expect(page.getByTestId('qty')).toHaveAttribute('role', 'spinbutton');
  await expect(page.getByTestId('qty')).toHaveAttribute('aria-valuenow', '3');
  // A selected segment is a Button on iOS; ARIA has no "selected" for a
  // button, so it is exposed as pressed, which is what a snapshot prints.
  expect(snap).toMatch(/button "Weekly" \[pressed\]/);
  expect(snap).toMatch(/button "Monthly"(?! \[pressed\])/);
  expect(snap).toMatch(/tab "Home \(tab-home\)" \[selected\]/);
  await expect(page.getByTestId('period')).toHaveAttribute('role', 'radiogroup');
  // A checkbox reporting a parsed state gets aria-checked; one whose value
  // is its own text (Android does not surface the boolean yet) must not —
  // a spurious [checked] would lie about a control the device never set.
  expect(snap).toMatch(/checkbox "Remember me \(remember\)" \[checked\]/);
  expect(snap).toMatch(/checkbox "Newsletter \(newsletter\)"(?! \[checked\])/);
  await expect(page.getByTestId('newsletter')).not.toHaveAttribute('aria-checked', /.*/);
});

// The audit names what no agent can address. It reports the app as it
// is: an icon-only button with no label, and two controls that read the
// same, are the app's facts, not the mirror's — but an agent must not
// discover them by acting on the wrong one.
test('the audit reports unnamed and duplicate controls, and nothing else', async ({ page }) => {
  await serveMirror(page, 'states');
  const audit = await page.evaluate(() => (window as any).devicedeck.audit());
  // A control with an identifier is named by it; only one with neither
  // label nor identifier is out of reach, and the audit says which.
  expect(audit.unnamed).toHaveLength(1);
  expect(audit.unnamed[0]).toMatchObject({ role: 'button', testid: '' });
  expect(audit.unnamed[0].key).toMatch(/Button/);
  expect(audit.duplicates).toEqual([{ role: 'button', name: 'Delete', count: 2 }]);
});

test('every real TestHive screen recorded here is fully addressable', async ({ page }) => {
  for (const name of ['login', 'login-typed', 'products', 'products-cart-full', 'cart']) {
    await serveMirror(page, name);
    const audit = await page.evaluate(() => (window as any).devicedeck.audit());
    expect(audit, name).toEqual({ unnamed: [], duplicates: [] });
  }
});

// An action finishes on the device, not on the page. The device page acts
// through a synchronous /act request and applies the settled tree it
// returns before its event handler returns — and Playwright's click() and
// fill() resolve only once the page has handled the event. So the call
// takes as long as the device does, and the next line reads the device's
// new screen with no waiting of its own.
test.describe('an action returns with the device screen after it', () => {
  const DEVICE_MS = 1500;

  async function stubAct(page: Page, answer: string) {
    const acts: any[] = [];
    await page.route('**/api/devices/*/act', async (route) => {
      acts.push(route.request().postDataJSON());
      await new Promise((r) => setTimeout(r, DEVICE_MS));
      await route.fulfill({ json: { ...fixture(answer), act: { kind: 'x', timedOut: false } } });
    });
    return acts;
  }

  test('click() waits for the tap and returns on the next screen', async ({ page }) => {
    await serveMirror(page, 'login');
    const acts = await stubAct(page, 'products');
    const started = Date.now();
    await page.getByTestId('forgot-password-button').click();
    expect(Date.now() - started).toBeGreaterThanOrEqual(DEVICE_MS - 50);
    // No auto-wait: count() reads the DOM as it is at this instant.
    expect(await page.getByTestId('add-to-cart-1').count()).toBe(1);
    expect(acts).toHaveLength(1);
    expect(acts[0]).toMatchObject({ kind: 'tap', app: 'dev.devicelab.testhive' });
    expect(acts[0].x).toBeGreaterThan(0);
    expect(acts[0].x).toBeLessThan(1);
  });

  test('fill() waits for the device to hold the value', async ({ page }) => {
    await serveMirror(page, 'login');
    const acts = await stubAct(page, 'login-typed');
    const started = Date.now();
    await page.getByTestId('username-input').fill('devicelab');
    expect(Date.now() - started).toBeGreaterThanOrEqual(DEVICE_MS - 50);
    expect(await page.getByTestId('username-input').getAttribute('data-dd-device-value')).toBe('devicelab');
    expect(acts).toHaveLength(1);
    expect(acts[0]).toMatchObject({ kind: 'fill', field: 'username-input', text: 'devicelab' });
  });

  // A refresh re-places every element; a field it moved lost focus, and an
  // unfocused secure field showed the device's bullets, not the text filled.
  test('a filled secure field keeps focus and its text across the refresh', async ({ page }) => {
    await serveMirror(page, 'login');
    const typed = fixture('login-typed');
    typed.nodes.find((n: any) => n.identifier === 'password-input').value = '•••••••••';
    await page.route('**/api/devices/*/act', (route) =>
      route.fulfill({ json: { ...typed, act: { kind: 'fill', timedOut: false } } }));
    const password = page.getByTestId('password-input');
    await password.fill('robustest');
    expect(await password.getAttribute('data-dd-device-value')).toBe('•••••••••');
    await expect(password).toBeFocused();
    await expect(password).toHaveValue('robustest');
  });

  // A horizontal row scrolled so a chip sits past the screen's left edge:
  // a scroll box cannot scroll to a negative offset, so Playwright could
  // never bring the chip into view and its click timed out. The mirror's
  // gutter makes it scrollable; the click then swipes the row and taps.
  test('a chip past the left edge is revealed and tapped', async ({ page }) => {
    await serveMirror(page, 'products');
    const products = fixture('products');
    const shifted = fixture('products');
    const row = new Set([22, 23, 24, 25, 26]);
    for (const n of shifted.nodes) if (row.has(n.index)) n.frame.x -= 120;
    await page.route('**/api/devices/*/tree*', (r) => r.fulfill({ json: shifted }));
    const acts: any[] = [];
    await page.route('**/api/devices/*/act', async (route) => {
      acts.push(route.request().postDataJSON());
      await route.fulfill({ json: { ...products, act: { kind: 'x', timedOut: false } } });
    });
    const all = page.getByRole('button', { name: 'All', exact: true });
    await expect.poll(async () => (await all.evaluate((el) => el.getBoundingClientRect().right))
      < (await page.locator('#mirror').evaluate((el) => el.getBoundingClientRect().left))).toBe(true);
    await all.click({ timeout: 5_000 });
    expect(acts.map((a) => a.kind)).toEqual(['swipe', 'tap']);
    const [swipe, tap] = acts;
    expect(swipe.toX).toBeGreaterThan(swipe.x); // finger moves right: the row scrolls left into view
    expect(Math.abs(swipe.y - (210.67 + 20.5) / 874)).toBeLessThan(0.02); // along the chip row
    expect(Math.abs(tap.x - (16 + 29) / 402)).toBeLessThan(0.02);
  });

  test('a key press waits for the device too', async ({ page }) => {
    await serveMirror(page, 'login');
    const acts = await stubAct(page, 'login');
    const started = Date.now();
    await page.getByTestId('username-input').press('Enter');
    expect(Date.now() - started).toBeGreaterThanOrEqual(DEVICE_MS - 50);
    expect(acts.map((a) => a.kind)).toContain('key');
  });
});
