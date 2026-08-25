// Checkout end to end: log in, add a product, fill the shipping form, reach
// the confirmation — all through the DOM mirror with Puppeteer's standard
// page API. Same contract as testhive-login.test.js; requires `devicedeck
// serve` and a booted simulator with TestHive freshly launched.

const { test, before, after } = require("node:test");
const assert = require("node:assert");
const puppeteer = require("puppeteer");

const BASE = process.env.DEVICEDECK_URL || "http://127.0.0.1:8787";
const UDID = process.env.DEVICEDECK_UDID || "booted";
const APP = "dev.devicelab.testhive";

let browser;
before(async () => { browser = await puppeteer.launch(); });
after(async () => { await browser.close(); });

// Puppeteer's page.click queries the selector once and does not wait — but
// a new screen appears on the mirror's next poll (~300ms after the action
// that triggered it), so a click aimed at a just-navigated screen has to
// wait for the element first. (Playwright and Cypress fold this wait into
// their actions; Puppeteer asks for it explicitly.)
const clickId = async (page, id, timeout = 20_000) => {
  const el = await page.waitForSelector(`[data-testid="${id}"]`, { visible: true, timeout });
  await el.click();
};

// Keys reach the device as real HID presses a moment after the DOM has
// them; wait for the device to echo a field (its read-back,
// data-dd-device-value) before a submit that depends on it.
const echoed = (page, id, expected) =>
  page.waitForFunction(
    (id, expected) => {
      const v = document.querySelector(`[data-testid="${id}"]`)?.dataset.ddDeviceValue || "";
      return typeof expected === "number" ? v.length === expected : v === expected;
    },
    { timeout: 10_000 }, id, expected,
  );

async function type(page, id, text, delay = 120) {
  await clickId(page, id);
  await page.keyboard.type(text, { delay });
}

test("checks out through the shipping form", async () => {
  const page = await browser.newPage();
  await page.goto(`${BASE}/device/${UDID}?app=${APP}`);
  await page.waitForSelector('[data-testid="username-input"]', { visible: true, timeout: 30_000 });

  // Log in.
  await type(page, "username-input", "devicelab");
  await type(page, "password-input", "robustest");
  await echoed(page, "username-input", "devicelab");
  await echoed(page, "password-input", "robustest".length);
  await clickId(page, "login-button");

  // Add a product and open the cart.
  await clickId(page, "add-to-cart-2");
  await clickId(page, "cart-button-cart-button");
  await clickId(page, "checkout-button");

  // Shipping form.
  await type(page, "address-input", "1 Market St", 80);
  await type(page, "city-input", "San Francisco", 80);
  await type(page, "zip-input", "94105", 80);
  await echoed(page, "zip-input", "94105"); // last field echoed ⇒ the form has landed

  await clickId(page, "next-payment-button");
  const done = await page.waitForSelector('[data-testid="continue-shopping-button"]', { timeout: 20_000 });
  assert.ok(done, "expected the checkout confirmation screen");
});
