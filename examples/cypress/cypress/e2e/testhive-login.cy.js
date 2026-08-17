// The device is a normal webpage: Cypress drives the simulator through
// the DOM mirror with its standard commands. Requires `devicedeck serve`
// and a booted simulator with TestHive installed (freshly launched).

const UDID = Cypress.env("DEVICEDECK_UDID") || "booted";
const APP = "dev.devicelab.testhive";

describe("TestHive login", () => {
  it("logs in on a real simulator", () => {
    cy.visit(`/device/${UDID}?app=${APP}`);

    // First render waits on the tree engine warm-up.
    cy.get('[data-testid="username-input"]', { timeout: 30000 })
      .should("be.visible")
      .click();
    // Keys forward as real HID presses (~100ms hold) — pace the typing.
    cy.get("body").type("devicelab", { delay: 150 });

    cy.get('[data-testid="password-input"]').click();
    cy.get("body").type("robustest", { delay: 150 });

    cy.get('[data-testid="login-button"]').click();

    cy.contains("Hello, devicelab!", { timeout: 20000 }).should("exist");
  });
});
