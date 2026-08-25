// Checkout end to end: log in, add a product, fill the shipping form, reach
// the confirmation — all through the DOM mirror with Cypress's standard
// commands. Same contract as testhive-login.cy.js; requires `devicedeck
// serve` and a booted simulator with TestHive freshly launched.

const UDID = Cypress.env("DEVICEDECK_UDID") || "booted";
const APP = "dev.devicelab.testhive";

// Keys reach the device as real HID presses a moment after the DOM has
// them, so wait for the device to echo a field (its read-back,
// data-dd-device-value) before acting on a submit that depends on it.
const echoes = (id, expected) =>
  cy.get(`[data-testid="${id}"]`).should(($el) => {
    const v = $el[0].dataset.ddDeviceValue || "";
    typeof expected === "number"
      ? expect(v).to.have.length(expected)
      : expect(v).to.eq(expected);
  });

describe("TestHive checkout", () => {
  it("checks out through the shipping form", () => {
    cy.visit(`/device/${UDID}?app=${APP}`);

    // Log in.
    cy.get('[data-testid="username-input"]', { timeout: 30000 }).should("be.visible").click();
    cy.get("body").type("devicelab", { delay: 150 });
    cy.get('[data-testid="password-input"]').click();
    cy.get("body").type("robustest", { delay: 150 });
    echoes("username-input", "devicelab");
    echoes("password-input", "robustest".length);
    cy.get('[data-testid="login-button"]').click();

    // Add a product and open the cart.
    cy.get('[data-testid="add-to-cart-2"]', { timeout: 20000 }).click();
    cy.get('[data-testid="cart-button-cart-button"]').click();
    cy.get('[data-testid="checkout-button"]').click();

    // Shipping form.
    cy.get('[data-testid="address-input"]').click();
    cy.get("body").type("1 Market St", { delay: 80 });
    cy.get('[data-testid="city-input"]').click();
    cy.get("body").type("San Francisco", { delay: 80 });
    cy.get('[data-testid="zip-input"]').click();
    cy.get("body").type("94105", { delay: 80 });
    echoes("zip-input", "94105"); // last field echoed ⇒ the form has landed

    cy.get('[data-testid="next-payment-button"]').click();
    cy.get('[data-testid="continue-shopping-button"]', { timeout: 20000 }).should("be.visible");
  });
});
