import { test, expect } from '../fixtures/coverage';

/**
 * E2E tests for the Memories page.
 *
 * The E2E suite runs with OMNIA_AUTH_ANONYMOUS_ROLE=editor, so the session is
 * anonymous. With the device ID feature, anonymous users get a per-browser
 * identity stored in localStorage, which enables memory scoping. The page
 * shows the normal memory UI (toolbar, empty state or graph) rather than a
 * sign-in notice.
 *
 * If localStorage is unavailable (SSR, incognito), the anonymous notice would
 * still appear — but in a real browser E2E environment, localStorage is always
 * available, so hasMemoryIdentity is true.
 */
test.describe('Memories Page (anonymous with device ID)', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/memories');
    // Wait for the page to render (auth context resolves, memory query fires)
    await page.waitForLoadState('networkidle');
  });

  test('should not show the sign-in notice', async ({ page }) => {
    const notice = page.locator('[data-testid="memory-anonymous-notice"]');
    await expect(notice).not.toBeVisible({ timeout: 5000 });
  });

  test('should not show an error alert', async ({ page }) => {
    const errorAlert = page.locator('[data-testid="memory-error"]');
    await expect(errorAlert).not.toBeVisible({ timeout: 5000 });
  });

  test('should show the toolbar for anonymous users with device ID', async ({ page }) => {
    await expect(page.locator('[data-testid="memories-toolbar"]')).toBeVisible({ timeout: 10000 });
  });

  test('should show the empty state when no memories exist', async ({ page }) => {
    // With device ID, the memory fetch fires but returns empty results
    await expect(page.locator('[data-testid="empty-state"]')).toBeVisible({ timeout: 10000 });
  });

  test('should still show the consent banner', async ({ page }) => {
    const banner = page.locator('[data-testid="consent-banner"]');
    await expect(banner).toBeVisible({ timeout: 5000 });
  });
});
