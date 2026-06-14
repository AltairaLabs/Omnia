import { test, expect } from '../fixtures/coverage';

/**
 * E2E tests for the Memory Galaxy page (the redesigned /memories).
 *
 * The E2E suite runs anonymous (editor role) with a device ID, and the
 * workspace context auto-selects the first workspace, so `hasWorkspace` is true
 * and the galaxy renders. The 2D-projection endpoint is mocked in the Next.js
 * proxy route (pending the backend in #1418), so points are always present —
 * there is no empty state, sign-in notice, or consent banner on this page
 * anymore (it's a workspace-scoped operator view).
 */
test.describe('Memory Galaxy page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/memories');
    await page.waitForLoadState('networkidle');
  });

  test('does not show an error alert', async ({ page }) => {
    await expect(page.locator('[data-testid="memory-error"]')).not.toBeVisible({ timeout: 5000 });
  });

  test('does not show the no-workspace notice', async ({ page }) => {
    await expect(page.locator('[data-testid="no-workspace-notice"]')).not.toBeVisible({ timeout: 5000 });
  });

  test('shows the toolbar', async ({ page }) => {
    await expect(page.locator('[data-testid="memories-toolbar"]')).toBeVisible({ timeout: 10000 });
  });

  test('shows the tier/category filter rail', async ({ page }) => {
    await expect(page.locator('[data-testid="facet-rail"]')).toBeVisible({ timeout: 10000 });
  });

  test('shows the projection summary once points load', async ({ page }) => {
    // The summary line ("N memories · semantic|lexical clustering") renders in
    // the page once the (mocked) projection returns points — independent of the
    // lazily-imported canvas, so it's a stable signal the galaxy data loaded.
    await expect(page.getByText(/clustering/i)).toBeVisible({ timeout: 15000 });
  });
});
