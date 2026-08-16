# DeviceDeck

**Your simulators and emulators in a browser — and what you do by hand becomes a test your existing framework can run.**

A self-hosted console for iOS Simulators and Android emulators. See a device in the browser,
drive it with your mouse and keyboard, and turn that session into a deterministic test — replayable
by maestro-runner, or by any WebDriver/Appium-speaking suite you already have.

Not started yet. Read `PROJECT-BRIEF.md` first (local-only, not committed) — it carries the
architecture, the measurements behind each decision, and the questions still open.

---

## Status

Pre-code. The brief exists so implementation does not re-litigate decisions that were already
settled with evidence, or re-run experiments that were already run.

## Scope

Simulators and emulators only. Real hardware is deliberately **out of scope** — that is
[devicelab.dev](https://devicelab.dev)'s job, and the boundary is what keeps the two from
competing with each other.

## Why it exists

Three tools occupy this space — tapflow, baguette, sim-use — and none of them can run the test
suite a team already owns. That gap is the product.

Built by [DeviceLab.dev](https://devicelab.dev)
