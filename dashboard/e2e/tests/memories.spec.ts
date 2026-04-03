import { test, expect } from '../fixtures/coverage';

test.describe('Memories Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/memories');
  });

  test('should display page with toolbar', async ({ page }) => {
    await page.waitForSelector('[data-testid="memories-toolbar"]', { timeout: 10000 });
    await expect(page.locator('[data-testid="memories-toolbar"]')).toBeVisible();
  });

  test('should show consent banner', async ({ page }) => {
    await expect(page.locator('[data-testid="consent-banner"]')).toBeVisible();
  });

  test('should show graph or empty state', async ({ page }) => {
    // One of these should be visible depending on whether memories exist
    const graph = page.locator('[data-testid="memory-graph"]');
    const emptyState = page.locator('[data-testid="empty-state"]');
    await expect(graph.or(emptyState).first()).toBeVisible({ timeout: 10000 });
  });

  test('should have search input', async ({ page }) => {
    await expect(page.locator('[data-testid="memory-search"]')).toBeVisible();
  });

  test('should have category filter', async ({ page }) => {
    await expect(page.locator('[data-testid="category-filter"]')).toBeVisible();
  });

  test('should have forget all button', async ({ page }) => {
    await expect(page.locator('[data-testid="forget-all-button"]')).toBeVisible();
  });

  test('should open detail panel on bubble click', async ({ page }) => {
    const node = page.locator('[data-testid="memory-node"]').first();
    // Only test if bubbles exist (may be empty in demo mode)
    if (await node.isVisible({ timeout: 5000 }).catch(() => false)) {
      await node.click();
      await expect(page.locator('[data-testid="memory-detail-panel"]')).toBeVisible();
    }
  });

  test('should search memories', async ({ page }) => {
    const searchInput = page.locator('[data-testid="memory-search"]');
    await searchInput.fill('test query');
    // Just verify no crash — actual filtering depends on data
    await page.waitForTimeout(500);
  });
});
