const { defineConfig } = require('cypress')

module.exports = defineConfig({
  video: false,
  screenshotOnRunFailure: false,
  defaultCommandTimeout: 20000,
  chromeWebSecurity: false,
  e2e: {
    setupNodeEvents(on, config) {},
    supportFile: false,
  },
})
