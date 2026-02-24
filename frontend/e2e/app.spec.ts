import { test, expect } from '@playwright/test';

// ─── Docker Tab ───────────────────────────────────────────────────────────────

test.describe('Docker tab', () => {

  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    // Wait for the app to finish initial load
    await page.waitForSelector('.tab.active', { timeout: 10_000 });
  });

  test('shows Docker tab active by default', async ({ page }) => {
    const dockerTab = page.locator('.tab', { hasText: 'Docker' });
    await expect(dockerTab).toHaveClass(/active/);
  });

  test('Docker status dot shows connected', async ({ page }) => {
    // The status dot next to Docker tab should be green (connected class)
    const dockerTab = page.locator('.tab', { hasText: 'Docker' });
    const dot = dockerTab.locator('.status-dot');
    await expect(dot).toHaveClass(/connected/, { timeout: 10_000 });
  });

  test('container list loads and shows running containers', async ({ page }) => {
    // Wait for at least one container row to appear
    await page.waitForSelector('tbody tr', { timeout: 15_000 });

    const rows = page.locator('tbody tr');
    const count = await rows.count();
    expect(count).toBeGreaterThan(0);
  });

  test('container count badge shows correct number', async ({ page }) => {
    await page.waitForSelector('tbody tr', { timeout: 15_000 });

    const rows = page.locator('tbody tr');
    const rowCount = await rows.count();

    const badge = page.locator('.count');
    await expect(badge).toContainText(`${rowCount} running`);
  });

  test('all visible containers show RUNNING status badge', async ({ page }) => {
    await page.waitForSelector('tbody tr', { timeout: 15_000 });

    const badges = page.locator('.badge');
    const count = await badges.count();
    expect(count).toBeGreaterThan(0);

    for (let i = 0; i < count; i++) {
      await expect(badges.nth(i)).toContainText('running', { ignoreCase: true });
    }
  });

  test('clicking container name opens log viewer', async ({ page }) => {
    await page.waitForSelector('tbody tr', { timeout: 15_000 });

    // Click the first container name
    const firstNameBtn = page.locator('.name-btn').first();
    const containerName = await firstNameBtn.textContent();
    await firstNameBtn.click();

    // Log panel should appear
    await expect(page.locator('.log-panel')).toBeVisible({ timeout: 5_000 });

    // Panel header should show the container name
    await expect(page.locator('.container-name')).toContainText(containerName?.trim() ?? '');

    // LIVE badge should be visible
    await expect(page.locator('.live-badge')).toBeVisible();
  });

  test('log viewer receives log lines', async ({ page }) => {
    await page.waitForSelector('tbody tr', { timeout: 15_000 });

    await page.locator('.name-btn').first().click();
    await expect(page.locator('.log-panel')).toBeVisible({ timeout: 5_000 });

    // Wait for xterm to render at least one row
    await expect(page.locator('.xterm-rows')).toBeVisible({ timeout: 10_000 });
    const rows = page.locator('.xterm-rows > div');
    await expect(rows.first()).not.toBeEmpty({ timeout: 10_000 });
  });

  test('closing log viewer hides the panel', async ({ page }) => {
    await page.waitForSelector('tbody tr', { timeout: 15_000 });
    await page.locator('.name-btn').first().click();
    await expect(page.locator('.log-panel')).toBeVisible({ timeout: 5_000 });

    // Click the close button
    await page.locator('.icon-btn.close-btn').click();
    await expect(page.locator('.log-panel')).not.toBeVisible({ timeout: 3_000 });
  });

});

// ─── Kubernetes Tab ───────────────────────────────────────────────────────────

test.describe('Kubernetes tab', () => {

  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await page.waitForSelector('.tab', { timeout: 10_000 });

    // Switch to K8s tab
    await page.locator('.tab', { hasText: 'Kubernetes' }).click();
  });

  test('switches to Kubernetes tab', async ({ page }) => {
    const k8sTab = page.locator('.tab', { hasText: 'Kubernetes' });
    await expect(k8sTab).toHaveClass(/active/);
  });

  test('pod list loads with pods', async ({ page }) => {
    await page.waitForSelector('tbody tr', { timeout: 15_000 });

    const rows = page.locator('tbody tr');
    const count = await rows.count();
    expect(count).toBeGreaterThan(0);
  });

  test('namespace selector shows default namespace', async ({ page }) => {
    const selector = page.locator('.ns-select');
    await expect(selector).toBeVisible({ timeout: 10_000 });

    await expect(selector.locator('option').nth(1)).not.toBeEmpty({ timeout: 10_000 });
    const options = selector.locator('option');
    const texts = await options.allTextContents();
    expect(texts.some(t => t.includes('default'))).toBeTruthy();
  });

  test('pods show Ready status', async ({ page }) => {
    await page.waitForSelector('tbody tr', { timeout: 15_000 });

    const readyBadges = page.locator('.ready-badge.ready');
    const count = await readyBadges.count();
    expect(count).toBeGreaterThan(0);
  });

  test('clicking logs button opens log viewer for pod', async ({ page }) => {
    await page.waitForSelector('tbody tr', { timeout: 15_000 });

    // Click the logs button on the first pod
    await page.locator('.action-btn.logs').first().click();

    await expect(page.locator('.log-panel')).toBeVisible({ timeout: 5_000 });
    await expect(page.locator('.live-badge')).toBeVisible();
  });

});

// ─── Navigation ───────────────────────────────────────────────────────────────

test.describe('Navigation', () => {

  test('can switch between Docker and Kubernetes tabs', async ({ page }) => {
    await page.goto('/');
    await page.waitForSelector('.tab', { timeout: 10_000 });

    // Switch to K8s
    await page.locator('.tab', { hasText: 'Kubernetes' }).click();
    await expect(page.locator('.tab', { hasText: 'Kubernetes' })).toHaveClass(/active/);

    // Switch back to Docker
    await page.locator('.tab', { hasText: 'Docker' }).click();
    await expect(page.locator('.tab', { hasText: 'Docker' })).toHaveClass(/active/);
  });

  test('switching tabs closes open log viewer', async ({ page }) => {
    await page.goto('/');
    await page.waitForSelector('tbody tr', { timeout: 15_000 });

    // Open logs on Docker tab
    await page.locator('.name-btn').first().click();
    await expect(page.locator('.log-panel')).toBeVisible({ timeout: 5_000 });

    // Switch to K8s tab
    await page.locator('.tab', { hasText: 'Kubernetes' }).click();

    // Log panel from Docker should be gone
    await expect(page.locator('.log-panel')).not.toBeVisible({ timeout: 3_000 });
  });

});