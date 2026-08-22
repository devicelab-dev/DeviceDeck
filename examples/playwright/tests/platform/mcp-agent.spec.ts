import { test, expect } from '@playwright/test';
import { APP, UDID, launchApp } from '../support/device';
import { McpSession, refFor } from '../support/mcp';

// The journey an agent failed at before the mirror named its controls:
// "add Maestro to the cart", on a screen with several buttons all
// labelled Add. Everything here goes through MCP tools — snapshot, read
// a ref, act — so it fails if an agent loses the ability to read the
// screen, address an element, or type into a field, none of which the
// Playwright-driven specs would notice.

const PAGE = `http://127.0.0.1:8787/device/${UDID}?app=${APP}`;
const screenOf = (snap: string) => (snap.match(/device screen ([0-9a-f]+)/) || [])[1];

test.describe('an agent driving the device over MCP', () => {
  let mcp: McpSession;

  test.beforeEach(async ({ request }) => {
    await launchApp(request);
    mcp = new McpSession();
    await mcp.start();
  });

  test.afterEach(() => mcp?.close());

  test('logs in and adds the product it was asked for', async () => {
    test.setTimeout(300_000);
    const nav = await mcp.call('browser_navigate', { url: PAGE });
    // The hardware keys have no buttons on the page; the title is the
    // only channel that tells an agent they can be driven at all.
    expect(nav).toContain('window.devicedeck.gesture');

    const login = await mcp.snapshotUntil(/textbox "Username/);
    // Refs are minted only for nodes that are visible and receive pointer
    // events. A mirror that renders but is not hit-testable hands the
    // agent a full tree with nothing clickable, and it fails silently —
    // so count the refs, not just the nodes.
    const refs = (login.match(/\[ref=e\d+\]/g) || []).length;
    expect(refs, 'the login screen should offer several actionable refs').toBeGreaterThanOrEqual(6);
    await mcp.call('browser_type', {
      element: 'Username field', target: refFor(login, /textbox "Username/), text: 'devicelab',
    });
    await mcp.call('browser_wait_for', { time: 2 });
    const withUser = await mcp.call('browser_snapshot', {});
    await mcp.call('browser_type', {
      element: 'Password field', target: refFor(withUser, /textbox "Password/), text: 'robustest',
    });

    const ready = await mcp.snapshotUntil(/button "Sign In[^"]*"(?! \[disabled\])/);
    await mcp.call('browser_click', { element: 'Sign In', target: refFor(ready, /button "Sign In/) });

    const products = await mcp.snapshotUntil(/add-to-cart/);
    // The screen fingerprint is how an agent knows the action landed.
    expect(screenOf(products)).not.toBe(screenOf(login));

    // Every Add must be distinguishable, or the choice below is a guess.
    const adds = products.split('\n').filter((l) => /button "Add \(/.test(l)).map((l) => l.trim());
    expect(adds.length).toBeGreaterThan(1);
    expect(new Set(adds).size).toBe(adds.length);

    // Pair the product with the Add button that follows it — the agent's
    // own reasoning step, checkable only because the ids are visible.
    const lines = products.split('\n');
    const from = lines.findIndex((l) => /generic "Maestro"/.test(l));
    expect(from, 'Maestro is on the products screen').toBeGreaterThan(-1);
    const addLine = lines.slice(from).find((l) => /button "Add \(/.test(l))!;
    await mcp.call('browser_click', {
      element: 'Add Maestro', target: refFor(addLine, /button "Add \(/),
    });

    const withCart = await mcp.snapshotUntil(/cart-button/);
    await mcp.call('browser_click', {
      element: 'Shopping Cart', target: refFor(withCart, /cart-button/),
    });

    const cart = await mcp.snapshotUntil(/Total:|My Cart/);
    expect(cart).toContain('Maestro');
    expect(cart).not.toContain('Appium');
  });
});
