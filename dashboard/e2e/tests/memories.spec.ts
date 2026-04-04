import { test, expect } from '../fixtures/coverage';

/**
 * E2E tests for the Memories page.
 *
 * These tests verify the full data flow: dashboard → proxy route → memory-api.
 * They fail if the proxy routes are missing, env vars aren't set, or the
 * memory-api is unreachable — which is the point.
 */
test.describe('Memories Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/memories');
    // Wait for the page to finish loading (toolbar is always rendered)
    await page.waitForSelector('[data-testid="memories-toolbar"]', { timeout: 10000 });
  });

  test('should load without API errors', async ({ page }) => {
    // The error alert should NOT be visible — if it is, the proxy/backend is broken
    const errorAlert = page.locator('[data-testid="memory-error"]');
    await expect(errorAlert).not.toBeVisible({ timeout: 5000 });
  });

  test('should show graph or empty state (not error)', async ({ page }) => {
    // Must show one of these — NOT the error state
    const graph = page.locator('[data-testid="memory-graph"]');
    const emptyState = page.locator('[data-testid="empty-state"]');
    const errorAlert = page.locator('[data-testid="memory-error"]');

    // Wait for content to settle
    await page.waitForTimeout(2000);

    const hasGraph = await graph.isVisible().catch(() => false);
    const hasEmpty = await emptyState.isVisible().catch(() => false);
    const hasError = await errorAlert.isVisible().catch(() => false);

    expect(hasError).toBe(false);
    expect(hasGraph || hasEmpty).toBe(true);
  });

  test('should show consent banner without errors', async ({ page }) => {
    const banner = page.locator('[data-testid="consent-banner"]');
    await expect(banner).toBeVisible({ timeout: 5000 });

    // Banner should show toggle switches, not a loading skeleton after 3s
    await page.waitForTimeout(3000);
    const switches = banner.locator('[role="switch"]');
    const switchCount = await switches.count();
    // Should have at least the 3 non-PII default toggles
    expect(switchCount).toBeGreaterThanOrEqual(3);
  });

  test('should have functional search input', async ({ page }) => {
    const searchInput = page.locator('[data-testid="memory-search"]');
    await expect(searchInput).toBeVisible();
    await expect(searchInput).toBeEnabled();
    await searchInput.fill('test query');
    await expect(searchInput).toHaveValue('test query');
  });

  test('should have functional category filter', async ({ page }) => {
    const filter = page.locator('[data-testid="category-filter"]');
    await expect(filter).toBeVisible();
    await filter.click();
    // Dropdown should show category options
    const options = page.locator('[role="option"]');
    const optionCount = await options.count();
    // "All Categories" + 6 category options = 7
    expect(optionCount).toBeGreaterThanOrEqual(7);
  });

  test('should intercept memory API requests through proxy', async ({ page }) => {
    // Verify the proxy route is wired by intercepting the network request
    const apiResponse = await page.waitForResponse(
      (resp) => resp.url().includes('/api/workspaces/') && resp.url().includes('/memory'),
      { timeout: 10000 }
    );

    // The proxy should return a valid response (not 404, not 503)
    const status = apiResponse.status();
    expect(status).not.toBe(404); // proxy route exists
    expect(status).not.toBe(503); // MEMORY_API_URL is configured
    // 200 = success, 502 = memory-api unreachable (acceptable in some CI envs)
    expect([200, 502]).toContain(status);
  });
});
