import { defineConfig } from 'cypress'
const fs = require('fs');
const path = require('path');

export default defineConfig({
  screenshotsFolder: './gui_test_screenshots/cypress/screenshots',
  screenshotOnRunFailure: true,
  trashAssetsBeforeRuns: true,
  videosFolder: './gui_test_screenshots/cypress/videos',
  video: true,
  reporter: './node_modules/cypress-multi-reporters',
  reporterOptions: {
    configFile: 'reporter-config.json',
  },
  env: {
    grepFilterSpecs: true,
    'HOST_API': process.env.CYPRESS_BASE_URL.replace(/console-openshift-console.apps/, 'api').concat(':6443')
  },
  fixturesFolder: 'fixtures',
  defaultCommandTimeout: 120000,
  pageLoadTimeout: 120000,
  retries: {
    runMode: 0,
    openMode: 0,
  },
  viewportWidth: 1440,
  viewportHeight: 900,
  e2e: {
    setupNodeEvents(on, config) {
      on('before:browser:launch', (browser = {
        name: "",
        family: "chromium",
        channel: "",
        displayName: "",
        version: "",
        majorVersion: "",
        path: "",
        isHeaded: false,
        isHeadless: false
      }, launchOptions) => {
        if (browser.family === 'chromium' && browser.name !== 'electron') {
          // auto open devtools
          launchOptions.args.push('--enable-precise-memory-info')
        }

        return launchOptions

      });
      // `on` is used to hook into various events Cypress emits
      on('task', {
        log(message) {
          console.log(message);
          return null;
        },
        logError(message) {
          console.error(message);
          return null;
        },
        logTable(data) {
          console.table(data);
          return null;
        },
        readFileIfExists(filename) {
          if (fs.existsSync(filename)) {
            return fs.readFileSync(filename, 'utf8');
          }
          return null;
        },
      });
      on('after:screenshot', (details) => {
        // Prepend "1_", "2_", etc. to screenshot filenames because they are sorted alphanumerically in CI's artifacts dir
        const pathObj = path.parse(details.path);
        fs.readdir(pathObj.dir, (error, files) => {
          const newPath = `${pathObj.dir}${path.sep}${files.length}_${pathObj.base}`;
          return new Promise((resolve, reject) => {
            // eslint-disable-next-line consistent-return
            fs.rename(details.path, newPath, (err) => {
              if (err) return reject(err);
              // because we renamed and moved the image, resolve with the new path
              // so it is accurate in the test results
              resolve({ path: newPath });
            });
          });
        });
      });
      require('@cypress/grep/src/plugin')(config);
      return config;
    },
    supportFile: './cypress/support/e2e.js',
    specPattern: 'tests/**/*.cy.{js,jsx,ts,tsx}',
    numTestsKeptInMemory: 5,
    testIsolation: false,
    experimentalMemoryManagement: true
  },
})
