const { defineConfig } = require("cypress");

module.exports = defineConfig({
  e2e: {
    baseUrl: process.env.DEVICEDECK_URL || "http://127.0.0.1:8787",
    supportFile: false,
    video: false,
    // The mirror tracks the native tree on a short poll; allow for
    // engine warm-up on first load.
    defaultCommandTimeout: 15000,
  },
});
