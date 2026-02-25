import { test as base } from '@playwright/test';

export const test = base.extend({
  page: async ({ page }, use) => {
    await page.addInitScript({ path: 'e2e/setup/wails-mock.js' });
    await use(page);
  },
});

export { expect } from '@playwright/test';