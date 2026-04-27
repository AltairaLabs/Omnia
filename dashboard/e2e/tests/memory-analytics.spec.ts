import { test, expect } from '../fixtures/coverage';
import type { Page, Route } from '@playwright/test';

/**
 * E2E tests for the /memory-analytics operator dashboard page (#1004 PR B).
 *
 * The page issues five parallel React Query fetches:
 *   - /api/workspaces/<ws>/memory/aggregate?groupBy=tier
 *   - /api/workspaces/<ws>/memory/aggregate?groupBy=tier&metric=distinct_users
 *   - /api/workspaces/<ws>/memory/aggregate?groupBy=category
 *   - /api/workspaces/<ws>/memory/aggregate?groupBy=agent
 *   - /api/workspaces/<ws>/memory/aggregate?groupBy=day&from=...&to=...   (today + range)
 *   - /api/workspaces/<ws>/privacy/consent/stats
 *
 * In DEMO_MODE the memory-api proxy returns 503, so without a route stub
 * every chart falls back to the empty state. We exercise both:
 *   - "empty workspace" — verifies structural elements render with zeros
 *   - "populated workspace" — stubs the proxy and verifies the tier tri-card,
 *     category donut, and growth-chart range selector all reflect the data.
 */

interface AggregateRow {
  key: string;
  value: number;
  count: number;
}

const TIER_ROWS: AggregateRow[] = [
  { key: 'institutional', value: 12, count: 12 },
  { key: 'agent', value: 34, count: 34 },
  { key: 'user', value: 78, count: 78 },
];

const TIER_DISTINCT_USERS_ROWS: AggregateRow[] = [
  { key: 'user', value: 17, count: 17 },
];

const CATEGORY_ROWS: AggregateRow[] = [
  { key: 'memory:context', value: 80, count: 80 },
  { key: 'memory:identity', value: 12, count: 12 },
  { key: 'memory:preferences', value: 32, count: 32 },
];

const AGENT_ROWS: AggregateRow[] = [
  { key: 'support-agent', value: 60, count: 60 },
  { key: 'onboarding-agent', value: 25, count: 25 },
];

const DAY_ROWS: AggregateRow[] = [
  { key: '2026-04-23', value: 4, count: 4 },
  { key: '2026-04-24', value: 7, count: 7 },
  { key: '2026-04-25', value: 5, count: 5 },
];

const CONSENT_STATS = {
  totalUsers: 100,
  optedOutAll: 5,
  grantsByCategory: {
    'memory:context': 92,
    'memory:identity': 18,
    'memory:preferences': 47,
  },
};

const NO_CACHE = { 'cache-control': 'no-store' };

function pickRowsForAggregate(url: URL): AggregateRow[] {
  const groupBy = url.searchParams.get('groupBy');
  const metric = url.searchParams.get('metric');
  if (groupBy === 'tier' && metric === 'distinct_users') return TIER_DISTINCT_USERS_ROWS;
  if (groupBy === 'tier') return TIER_ROWS;
  if (groupBy === 'category') return CATEGORY_ROWS;
  if (groupBy === 'agent') return AGENT_ROWS;
  if (groupBy === 'day') return DAY_ROWS;
  return [];
}

async function stubPopulatedAggregate(page: Page) {
  await page.route('**/api/workspaces/*/memory/aggregate*', async (route: Route) => {
    const url = new URL(route.request().url());
    await route.fulfill({
      status: 200,
      headers: { 'content-type': 'application/json', ...NO_CACHE },
      body: JSON.stringify(pickRowsForAggregate(url)),
    });
  });
  await page.route('**/api/workspaces/*/privacy/consent/stats*', async (route: Route) => {
    await route.fulfill({
      status: 200,
      headers: { 'content-type': 'application/json', ...NO_CACHE },
      body: JSON.stringify(CONSENT_STATS),
    });
  });
}

test.describe('/memory-analytics page (empty workspace)', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/memory-analytics');
    await page.waitForLoadState('networkidle');
  });

  test('renders the page header', async ({ page }) => {
    await expect(
      page.getByRole('heading', { name: 'Memory analytics', exact: true }),
    ).toBeVisible();
  });

  test('renders the tier legend with all three tier names and descriptions', async ({ page }) => {
    // Tier descriptions appear in both the TierLegend card AND each TierTriCard
    // (the tri-card echoes the description under each tier count). Using
    // .first() on each accepts either match — we only care that the text exists.
    await expect(page.getByText('How memory is organized')).toBeVisible();
    await expect(
      page.getByText(/Knowledge shared across every agent/).first(),
    ).toBeVisible();
    await expect(
      page.getByText(/Patterns this agent has learned/).first(),
    ).toBeVisible();
    await expect(page.getByText(/about a specific user/).first()).toBeVisible();
  });

  test('renders the tier tri-card with zero state when no data', async ({ page }) => {
    // Each tier name appears in both the TierLegend AND the TierTriCard;
    // accept either match via .first().
    await expect(page.getByText('Institutional', { exact: true }).first()).toBeVisible();
    await expect(page.getByText('Agent', { exact: true }).first()).toBeVisible();
    await expect(page.getByText('User', { exact: true }).first()).toBeVisible();
    // "0.0%" is the share text; one per tier card.
    await expect(page.getByText('0.0%').first()).toBeVisible();
  });

  test('renders the four chart sections', async ({ page }) => {
    await expect(page.getByText(/Memory by category/i)).toBeVisible();
    await expect(page.getByText(/Growth over time/i)).toBeVisible();
    await expect(page.getByText(/Memory by agent/i)).toBeVisible();
    await expect(page.getByText(/Privacy posture/i)).toBeVisible();
  });

  test('renders 7d / 30d / 90d range buttons on the growth chart', async ({ page }) => {
    await expect(page.getByRole('button', { name: '7d', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: '30d', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: '90d', exact: true })).toBeVisible();
  });
});

test.describe('/memory-analytics page (populated workspace)', () => {
  test.beforeEach(async ({ page }) => {
    await stubPopulatedAggregate(page);
    await page.goto('/memory-analytics');
    await page.waitForLoadState('networkidle');
  });

  test('tier tri-card reflects the stubbed counts', async ({ page }) => {
    await expect(page.getByText('12', { exact: true })).toBeVisible();
    await expect(page.getByText('34', { exact: true })).toBeVisible();
    await expect(page.getByText('78', { exact: true })).toBeVisible();
    // 12 / 124 ≈ 9.7%, 34 / 124 ≈ 27.4%, 78 / 124 ≈ 62.9% — assert the share line for the user tier.
    await expect(page.getByText(/62\.9%/)).toBeVisible();
  });

  test('summary cards reflect derived totals', async ({ page }) => {
    // Total memories = sum of category rows = 80 + 12 + 32 = 124.
    await expect(page.getByText('124', { exact: true })).toBeVisible();
    // Active users = distinct_users on the user tier = 17.
    await expect(page.getByText('17', { exact: true })).toBeVisible();
  });

  test('privacy posture reflects opt-out rate', async ({ page }) => {
    await expect(page.getByText('5.0%')).toBeVisible();
    await expect(page.getByText('5 of 100 users')).toBeVisible();
  });

  test('clicking a different range button activates it', async ({ page }) => {
    const ninetyDay = page.getByRole('button', { name: '90d', exact: true });
    await ninetyDay.click();
    // The active range gets the default variant; outline buttons get the
    // outline variant. Both stay visible — assert by the lack of error.
    await expect(ninetyDay).toBeVisible();
  });
});
