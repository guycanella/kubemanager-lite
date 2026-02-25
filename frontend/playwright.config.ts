import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  retries: 1,
  use: {
    // Wails dev server runs on this port during `wails dev`
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? 'http://localhost:34115',
    headless: process.env.CI ? true : false,
    viewport: { width: 1280, height: 800 },
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  reporter: [['list'], ['html', { open: 'never' }]],
});