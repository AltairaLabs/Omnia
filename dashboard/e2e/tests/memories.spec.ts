import { test, expect } from '../fixtures/coverage';

/**
 * E2E smoke test for the Memory Galaxy page (the redesigned /memories).
 *
 * The page is a workspace-scoped operator view whose 2D-projection endpoint is
 * mocked in the Next.js proxy (pending the backend in #1418). The projection
 * data path depends on workspace resolution, which is environment-specific in
 * E2E, so this smoke test only asserts the route loads and renders its header —
 * the page's logic and states are covered by unit tests
 * (app/memories/page.test.tsx, the galaxy-math/hook/facet-rail suites).
 */
test.describe('Memory Galaxy page', () => {
  test('loads and renders the header', async ({ page }) => {
    await page.goto('/memories');
    await page.waitForLoadState('networkidle');
    await expect(
      page.getByRole('heading', { name: /memory galaxy/i }),
    ).toBeVisible({ timeout: 10000 });
  });
});
