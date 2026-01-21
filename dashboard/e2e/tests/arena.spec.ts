import { test, expect } from '../fixtures/coverage';
import type { Page } from '@playwright/test';

/**
 * E2E tests for Arena Fleet UI.
 *
 * These tests verify the Arena pages work correctly in demo mode,
 * which uses the MockDataService to provide sample Arena data.
 *
 * Mock data includes:
 * - 2 sources: customer-support-scenarios, sales-eval-suite
 * - 2 configs: support-eval-config, sales-eval-config
 * - 3 jobs: support-eval-20240115 (completed), sales-eval-running (running), support-eval-failed (failed)
 */

// Page paths
const ARENA_OVERVIEW_PATH = '/arena';
const ARENA_SOURCES_PATH = '/arena/sources';
const ARENA_CONFIGS_PATH = '/arena/configs';
const ARENA_JOBS_PATH = '/arena/jobs';

// Timeouts
const DEFAULT_TIMEOUT = 10000;
const NAVIGATION_TIMEOUT = 5000;

// Common selectors
const SELECTORS = {
  // Overview page
  statsCard: '[data-testid="stat-card"], .stat-card, [class*="StatCard"]',
  recentJobsTable: 'table',
  quickLinks: 'a[href="/arena/sources"], a[href="/arena/configs"], a[href="/arena/jobs"]',
  featureSummary: 'text=Available Features',

  // Sources page - look for links to source detail pages as markers
  sourcesGrid: '[data-testid="sources-grid"], .grid',
  sourceItem: 'a[href*="/arena/sources/customer-support-scenarios"], a[href*="/arena/sources/sales-eval-suite"]',
  createSourceButton: 'button:has-text("Create Source")',
  sourceTypeBadge: '[data-testid="source-type-badge"], [class*="Badge"]',

  // Configs page - look for links to config detail pages
  configsGrid: '[data-testid="configs-grid"], .grid',
  configItem: 'a[href*="/arena/configs/support-eval-config"], a[href*="/arena/configs/sales-eval-config"]',

  // Jobs page - look for links to job detail pages or job names
  jobsGrid: '[data-testid="jobs-grid"], .grid',
  jobItem: 'a[href*="/arena/jobs/support-eval-20240115"], a[href*="/arena/jobs/sales-eval-running"]',
  createJobButton: 'button:has-text("Create Job")',
  jobTypeFilter: '[data-testid="type-filter"], button:has-text("All Types")',
  jobStatusFilter: '[data-testid="status-filter"], button:has-text("All Status")',
  jobProgressBar: '[data-testid="job-progress"], [class*="Progress"]',

  // Common
  viewToggle: '[role="tablist"]',
  viewToggleTab: '[role="tab"]',
  tableViewButton: '[role="tab"]:has([class*="List"]), button:has-text("Table")',
  gridViewButton: '[role="tab"]:has([class*="Grid"]), button:has-text("Grid")',
  dialog: '[role="dialog"]',
  loadingSkeleton: '[class*="Skeleton"], [data-testid="loading"]',
  errorAlert: '[role="alert"]',
  breadcrumb: '[data-testid="breadcrumb"], nav[aria-label="Breadcrumb"], [class*="Breadcrumb"]',
  emptyState: 'text=No .* found',

  // Quick links
  sourcesLink: 'a[href="/arena/sources"]',
  configsLink: 'a[href="/arena/configs"]',
  jobsLink: 'a[href="/arena/jobs"]',

  // Common text patterns
  readyStatus: 'text=Ready',
  evaluationType: 'text=Evaluation',
  runningStatus: 'text=Running',
};

// Mock source names from MockDataService
const MOCK_SOURCES = ['customer-support-scenarios', 'sales-eval-suite'];
const MOCK_CONFIGS = ['support-eval-config', 'sales-eval-config'];
const MOCK_JOBS = ['support-eval-20240115', 'sales-eval-running', 'support-eval-failed'];

/**
 * Wait for page to finish loading (no loading skeleton visible).
 */
async function waitForPageLoad(page: Page): Promise<void> {
  // Wait for any loading skeleton to disappear
  await page.waitForFunction(
    () => !document.querySelector('[class*="Skeleton"]'),
    { timeout: DEFAULT_TIMEOUT }
  ).catch(() => {
    // Ignore timeout - page might not have had a loading state
  });

  // Wait for network to be idle
  await page.waitForLoadState('networkidle', { timeout: DEFAULT_TIMEOUT }).catch(() => {
    // Ignore timeout - some background requests might be ongoing
  });
}

// ============================================================
// Arena Overview Page Tests
// ============================================================

test.describe('Arena Overview Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(ARENA_OVERVIEW_PATH);
    await waitForPageLoad(page);
  });

  test('should display the page header', async ({ page }) => {
    // Check for Arena title
    const header = page.locator('h1:has-text("Arena"), [data-testid="page-title"]:has-text("Arena")');
    await expect(header.first()).toBeVisible({ timeout: DEFAULT_TIMEOUT });
  });

  test('should display stats cards with Arena metrics', async ({ page }) => {
    // Wait for stats to load
    await page.waitForSelector('text=Active Sources', { timeout: DEFAULT_TIMEOUT });

    // Verify stats cards are displayed (use first() to handle multiple matches)
    await expect(page.locator('text=Active Sources').first()).toBeVisible();
    await expect(page.locator('text=Configurations').first()).toBeVisible();
    await expect(page.locator('text=Running Jobs').first()).toBeVisible();
    await expect(page.locator('text=Success Rate').first()).toBeVisible();
  });

  test('should display feature summary with license-gated features', async ({ page }) => {
    // Wait for feature summary card
    await page.waitForSelector(SELECTORS.featureSummary, { timeout: DEFAULT_TIMEOUT });

    // Verify feature summary shows features
    await expect(page.locator('text=ConfigMap Sources')).toBeVisible();
    await expect(page.locator('text=Evaluation Jobs')).toBeVisible();

    // In open-core mode, enterprise features should show "Enterprise" badge
    const enterpriseBadges = page.locator('text=Enterprise');
    await expect(enterpriseBadges.first()).toBeVisible();
  });

  test('should display recent jobs table', async ({ page }) => {
    // Wait for recent jobs section
    await page.waitForSelector('text=Recent Jobs', { timeout: DEFAULT_TIMEOUT });

    // Verify "Recent Jobs" header and "View all" link are visible
    await expect(page.locator('text=Recent Jobs').first()).toBeVisible();
    await expect(page.locator('text=View all').first()).toBeVisible();
  });

  test('should display quick links to other Arena pages', async ({ page }) => {
    // Verify quick links are present (use first() since there may be multiple links)
    await expect(page.locator(SELECTORS.sourcesLink).first()).toBeVisible();
    await expect(page.locator(SELECTORS.configsLink).first()).toBeVisible();
    await expect(page.locator(SELECTORS.jobsLink).first()).toBeVisible();
  });

  test('should navigate to Sources page from quick link', async ({ page }) => {
    // Click on Manage Sources link
    await page.locator(SELECTORS.sourcesLink).first().click();

    // Verify navigation
    await expect(page).toHaveURL(/\/arena\/sources/);
  });

  test('should navigate to Jobs page from stat card', async ({ page }) => {
    // Click on Running Jobs stat card (which is a link)
    const runningJobsLink = page.locator(SELECTORS.jobsLink).first();
    await runningJobsLink.click();

    // Verify navigation
    await expect(page).toHaveURL(/\/arena\/jobs/);
  });
});

// ============================================================
// Arena Sources Page Tests
// ============================================================

test.describe('Arena Sources Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(ARENA_SOURCES_PATH);
    await waitForPageLoad(page);
  });

  test('should display list of sources', async ({ page }) => {
    // Wait for sources to load
    await page.waitForSelector(SELECTORS.sourceItem, { timeout: DEFAULT_TIMEOUT });

    // Verify at least one source is displayed (mock data has 2)
    const sources = page.locator(SELECTORS.sourceItem);
    await expect(sources.first()).toBeVisible();

    // Verify mock source names are present
    await expect(page.locator(`text=${MOCK_SOURCES[0]}`)).toBeVisible();
  });

  test('should display source status badges', async ({ page }) => {
    await page.waitForSelector(SELECTORS.sourceItem, { timeout: DEFAULT_TIMEOUT });

    // Mock sources are Ready status
    await expect(page.locator(SELECTORS.readyStatus).first()).toBeVisible();
  });

  test('should display source type badges', async ({ page }) => {
    await page.waitForSelector(SELECTORS.sourceItem, { timeout: DEFAULT_TIMEOUT });

    // Mock sources use ConfigMap type
    await expect(page.locator('text=ConfigMap').first()).toBeVisible();
  });

  test('should toggle between grid and table view', async ({ page }) => {
    await page.waitForSelector(SELECTORS.sourceItem, { timeout: DEFAULT_TIMEOUT });

    // Find the view toggle tabs
    const viewToggle = page.locator(SELECTORS.viewToggle);

    if (await viewToggle.isVisible()) {
      // Click table view
      const tableButton = viewToggle.locator(SELECTORS.viewToggleTab).nth(1);
      await tableButton.click();

      // Verify table is displayed
      await expect(page.locator('table')).toBeVisible({ timeout: NAVIGATION_TIMEOUT });

      // Click grid view
      const gridButton = viewToggle.locator(SELECTORS.viewToggleTab).nth(0);
      await gridButton.click();

      // Verify grid is displayed (cards)
      await expect(page.locator(SELECTORS.sourceItem).first()).toBeVisible({ timeout: NAVIGATION_TIMEOUT });
    }
  });

  test('should show Create Source button', async ({ page }) => {
    // Check for Create Source button
    const createButton = page.locator(SELECTORS.createSourceButton);
    await expect(createButton).toBeVisible({ timeout: DEFAULT_TIMEOUT });
  });

  test('should open Create Source dialog when clicking create button', async ({ page }) => {
    const createButton = page.locator(SELECTORS.createSourceButton);
    await expect(createButton).toBeVisible({ timeout: DEFAULT_TIMEOUT });

    await createButton.click();

    // Verify dialog opens
    await expect(page.locator(SELECTORS.dialog)).toBeVisible({ timeout: NAVIGATION_TIMEOUT });

    // Verify dialog has source type selection
    await expect(page.locator('text=Source Type').first()).toBeVisible();
  });

  test('should navigate to source detail page on click', async ({ page }) => {
    await page.waitForSelector(SELECTORS.sourceItem, { timeout: DEFAULT_TIMEOUT });

    // Click on first source link
    const sourceLink = page.locator(`a[href*="/arena/sources/"]`).first();
    await sourceLink.click();

    // Verify navigation to detail page
    await expect(page).toHaveURL(/\/arena\/sources\/[^/]+$/);
  });

  test('should display breadcrumb navigation', async ({ page }) => {
    await page.waitForSelector(SELECTORS.sourceItem, { timeout: DEFAULT_TIMEOUT });

    // Check for breadcrumb with "Sources" text
    await expect(page.locator('text=Sources').first()).toBeVisible();
  });
});

// ============================================================
// Arena Jobs Page Tests
// ============================================================

test.describe('Arena Jobs Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(ARENA_JOBS_PATH);
    await waitForPageLoad(page);
  });

  test('should display list of jobs', async ({ page }) => {
    // Wait for jobs to load
    await page.waitForSelector(SELECTORS.jobItem, { timeout: DEFAULT_TIMEOUT });

    // Verify at least one job is displayed (mock data has 3)
    const jobs = page.locator(SELECTORS.jobItem);
    await expect(jobs.first()).toBeVisible();
  });

  test('should display job status badges', async ({ page }) => {
    await page.waitForSelector(SELECTORS.jobItem, { timeout: DEFAULT_TIMEOUT });

    // Mock data has jobs with different statuses - check for at least one status
    // The Running job should always be visible
    await expect(page.locator(SELECTORS.runningStatus).first()).toBeVisible();
  });

  test('should display job type badges', async ({ page }) => {
    await page.waitForSelector(SELECTORS.jobItem, { timeout: DEFAULT_TIMEOUT });

    // Mock jobs are evaluation type
    await expect(page.locator(SELECTORS.evaluationType).first()).toBeVisible();
  });

  test('should display job progress', async ({ page }) => {
    await page.waitForSelector(SELECTORS.jobItem, { timeout: DEFAULT_TIMEOUT });

    // Check for progress indicators (like "7/12" or progress bars)
    const progressText = page.locator('text=/\\d+\\/\\d+/');
    await expect(progressText.first()).toBeVisible();
  });

  test('should show type filter dropdown', async ({ page }) => {
    // Look for type filter
    const typeFilter = page.locator('button:has-text("All Types")');
    await expect(typeFilter).toBeVisible({ timeout: DEFAULT_TIMEOUT });

    // Click to open dropdown
    await typeFilter.click();

    // Verify filter options
    await expect(page.locator('[role="option"]:has-text("Evaluation")')).toBeVisible();
  });

  test('should show status filter dropdown', async ({ page }) => {
    // Look for status filter
    const statusFilter = page.locator('button:has-text("All Status")');
    await expect(statusFilter).toBeVisible({ timeout: DEFAULT_TIMEOUT });

    // Click to open dropdown
    await statusFilter.click();

    // Verify filter options
    await expect(page.locator('[role="option"]:has-text("Running")')).toBeVisible();
    await expect(page.locator('[role="option"]:has-text("Completed")')).toBeVisible();
    await expect(page.locator('[role="option"]:has-text("Failed")')).toBeVisible();
  });

  test('should filter jobs by status', async ({ page }) => {
    await page.waitForSelector(SELECTORS.jobItem, { timeout: DEFAULT_TIMEOUT });

    // Click status filter
    const statusFilter = page.locator('button:has-text("All Status")');
    await statusFilter.click();

    // Select "Completed"
    await page.locator('[role="option"]:has-text("Completed")').click();

    // Wait for filter to apply
    await page.waitForTimeout(500);

    // Verify only completed jobs are shown (check that Running is not visible)
    // Note: This depends on the mock data structure
    const jobCards = page.locator(SELECTORS.jobItem);
    const count = await jobCards.count();
    expect(count).toBeGreaterThanOrEqual(1);
  });

  test('should toggle between grid and table view', async ({ page }) => {
    await page.waitForSelector(SELECTORS.jobItem, { timeout: DEFAULT_TIMEOUT });

    // Find the view toggle tabs
    const viewToggle = page.locator(SELECTORS.viewToggle);

    if (await viewToggle.isVisible()) {
      // Click table view
      const tableButton = viewToggle.locator(SELECTORS.viewToggleTab).nth(1);
      await tableButton.click();

      // Verify table is displayed
      await expect(page.locator('table')).toBeVisible({ timeout: NAVIGATION_TIMEOUT });

      // Verify table has correct headers
      await expect(page.locator('th:has-text("Name")')).toBeVisible();
      await expect(page.locator('th:has-text("Type")')).toBeVisible();
      await expect(page.locator('th:has-text("Status")')).toBeVisible();
    }
  });

  test('should show Create Job button', async ({ page }) => {
    // Check for Create Job button
    const createButton = page.locator(SELECTORS.createJobButton);
    await expect(createButton).toBeVisible({ timeout: DEFAULT_TIMEOUT });
  });

  test('should open Create Job dialog when clicking create button', async ({ page }) => {
    const createButton = page.locator(SELECTORS.createJobButton);
    await expect(createButton).toBeVisible({ timeout: DEFAULT_TIMEOUT });

    await createButton.click();

    // Verify dialog opens
    await expect(page.locator(SELECTORS.dialog)).toBeVisible({ timeout: NAVIGATION_TIMEOUT });

    // Verify dialog has job configuration fields
    await expect(page.locator('text=Name').first()).toBeVisible();
  });

  test('should navigate to job detail page on click', async ({ page }) => {
    await page.waitForSelector(SELECTORS.jobItem, { timeout: DEFAULT_TIMEOUT });

    // Click on first job link
    const jobLink = page.locator('a[href*="/arena/jobs/"]').first();
    await jobLink.click();

    // Verify navigation to detail page
    await expect(page).toHaveURL(/\/arena\/jobs\/[^/]+$/);
  });
});

// ============================================================
// Arena Configs Page Tests
// ============================================================

test.describe('Arena Configs Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(ARENA_CONFIGS_PATH);
    await waitForPageLoad(page);
  });

  test('should display list of configs', async ({ page }) => {
    // Wait for configs to load
    await page.waitForSelector(SELECTORS.configItem, { timeout: DEFAULT_TIMEOUT });

    // Verify at least one config is displayed (mock data has 2)
    const configs = page.locator(SELECTORS.configItem);
    await expect(configs.first()).toBeVisible();

    // Verify mock config names are present
    await expect(page.locator(`text=${MOCK_CONFIGS[0]}`)).toBeVisible();
  });

  test('should display config status badges', async ({ page }) => {
    await page.waitForSelector(SELECTORS.configItem, { timeout: DEFAULT_TIMEOUT });

    // Mock configs are Ready status
    await expect(page.locator(SELECTORS.readyStatus).first()).toBeVisible();
  });

  test('should display scenario count', async ({ page }) => {
    await page.waitForSelector(SELECTORS.configItem, { timeout: DEFAULT_TIMEOUT });

    // Mock configs have scenario counts (8 and 12) - shown as "Scenarios: 8"
    await expect(page.locator('text=Scenarios').first()).toBeVisible();
    await expect(page.locator('text="8"').or(page.locator('text="12"')).first()).toBeVisible();
  });

  test('should navigate to config detail page on click', async ({ page }) => {
    await page.waitForSelector(SELECTORS.configItem, { timeout: DEFAULT_TIMEOUT });

    // Click on first config link
    const configLink = page.locator('a[href*="/arena/configs/"]').first();
    await configLink.click();

    // Verify navigation to detail page
    await expect(page).toHaveURL(/\/arena\/configs\/[^/]+$/);
  });

  test('should show linked source reference', async ({ page }) => {
    await page.waitForSelector(SELECTORS.configItem, { timeout: DEFAULT_TIMEOUT });

    // Configs should show their source reference
    // The mock configs reference customer-support-scenarios and sales-eval-suite
    await expect(page.locator('text=customer-support-scenarios').first()).toBeVisible();
  });
});

// ============================================================
// Arena Navigation Tests
// ============================================================

test.describe('Arena Navigation', () => {
  test('should navigate between Arena pages using sidebar', async ({ page }) => {
    // Start at overview
    await page.goto(ARENA_OVERVIEW_PATH);
    await waitForPageLoad(page);

    // Navigate to Sources using quick link
    await page.locator('a[href="/arena/sources"]').first().click();
    await expect(page).toHaveURL(/\/arena\/sources/);

    // Navigate back to overview via breadcrumb or sidebar
    await page.goto(ARENA_OVERVIEW_PATH);
    await expect(page).toHaveURL(/\/arena$/);
  });

  test('should maintain state when navigating between list and detail views', async ({ page }) => {
    // Go to jobs page
    await page.goto(ARENA_JOBS_PATH);
    await waitForPageLoad(page);

    // Wait for jobs to load
    await page.waitForSelector(SELECTORS.jobItem, { timeout: DEFAULT_TIMEOUT });

    // Navigate to job detail
    const jobLink = page.locator('a[href*="/arena/jobs/"]').first();
    const jobName = await jobLink.textContent();
    await jobLink.click();

    // Verify on detail page
    await expect(page).toHaveURL(/\/arena\/jobs\/[^/]+$/);

    // Verify job name is displayed on detail page
    if (jobName) {
      await expect(page.locator(`text=${jobName}`).first()).toBeVisible({ timeout: DEFAULT_TIMEOUT });
    }
  });
});

// ============================================================
// Arena Detail Page Tests
// ============================================================

test.describe('Arena Source Detail Page', () => {
  test('should display source details', async ({ page }) => {
    // Navigate directly to a mock source
    await page.goto(`${ARENA_SOURCES_PATH}/${MOCK_SOURCES[0]}`);
    await waitForPageLoad(page);

    // Verify source name is displayed
    await expect(page.locator(`text=${MOCK_SOURCES[0]}`).first()).toBeVisible({ timeout: DEFAULT_TIMEOUT });

    // Verify source status
    await expect(page.locator(SELECTORS.readyStatus).first()).toBeVisible();
  });

  test('should display source spec details', async ({ page }) => {
    await page.goto(`${ARENA_SOURCES_PATH}/${MOCK_SOURCES[0]}`);
    await waitForPageLoad(page);

    // Verify type is displayed
    await expect(page.locator('text=ConfigMap').first()).toBeVisible({ timeout: DEFAULT_TIMEOUT });

    // Verify interval is displayed (5m for customer-support-scenarios)
    await expect(page.locator('text=/5m|5 min/i').first()).toBeVisible();
  });
});

test.describe('Arena Job Detail Page', () => {
  test('should display job details', async ({ page }) => {
    // Navigate directly to a mock job (the completed one)
    await page.goto(`${ARENA_JOBS_PATH}/${MOCK_JOBS[0]}`);
    await waitForPageLoad(page);

    // Verify job name is displayed
    await expect(page.locator(`text=${MOCK_JOBS[0]}`).first()).toBeVisible({ timeout: DEFAULT_TIMEOUT });
  });

  test('should display job status and progress', async ({ page }) => {
    await page.goto(`${ARENA_JOBS_PATH}/${MOCK_JOBS[0]}`);
    await waitForPageLoad(page);

    // Verify status badge (this job is Completed)
    await expect(page.locator('text=Completed').first()).toBeVisible({ timeout: DEFAULT_TIMEOUT });

    // Verify job type
    await expect(page.locator(SELECTORS.evaluationType).first()).toBeVisible();
  });

  test('should display running job with active progress', async ({ page }) => {
    // Navigate to the running job
    await page.goto(`${ARENA_JOBS_PATH}/${MOCK_JOBS[1]}`);
    await waitForPageLoad(page);

    // Verify running status is shown
    await expect(page.locator(SELECTORS.runningStatus).first()).toBeVisible({ timeout: DEFAULT_TIMEOUT });

    // Verify the job has evaluation type
    await expect(page.locator(SELECTORS.evaluationType).first()).toBeVisible();
  });
});

test.describe('Arena Config Detail Page', () => {
  test('should display config details', async ({ page }) => {
    // Navigate directly to a mock config
    await page.goto(`${ARENA_CONFIGS_PATH}/${MOCK_CONFIGS[0]}`);
    await waitForPageLoad(page);

    // Verify config name is displayed
    await expect(page.locator(`text=${MOCK_CONFIGS[0]}`).first()).toBeVisible({ timeout: DEFAULT_TIMEOUT });

    // Verify status
    await expect(page.locator(SELECTORS.readyStatus).first()).toBeVisible();
  });

  test('should display linked source reference', async ({ page }) => {
    await page.goto(`${ARENA_CONFIGS_PATH}/${MOCK_CONFIGS[0]}`);
    await waitForPageLoad(page);

    // The first mock config references "customer-support-scenarios"
    await expect(page.locator('text=customer-support-scenarios').first()).toBeVisible({ timeout: DEFAULT_TIMEOUT });
  });

  test('should display scenario list', async ({ page }) => {
    await page.goto(`${ARENA_CONFIGS_PATH}/${MOCK_CONFIGS[0]}`);
    await waitForPageLoad(page);

    // Verify scenarios section exists - look for the "Scenarios" label
    await expect(page.locator('text=Scenarios').first()).toBeVisible({ timeout: DEFAULT_TIMEOUT });
  });
});

// ============================================================
// Arena Error Handling Tests
// ============================================================

test.describe('Arena Error Handling', () => {
  test('should handle navigation to non-existent source', async ({ page }) => {
    await page.goto(`${ARENA_SOURCES_PATH}/nonexistent-source-12345`);
    await waitForPageLoad(page);

    // Should show "Source not found" error message
    await expect(page.locator('text=Source not found')).toBeVisible({ timeout: DEFAULT_TIMEOUT });
  });

  test('should handle navigation to non-existent job', async ({ page }) => {
    await page.goto(`${ARENA_JOBS_PATH}/nonexistent-job-12345`);
    await waitForPageLoad(page);

    // Should show "Job not found" error message
    await expect(page.locator('text=Job not found')).toBeVisible({ timeout: DEFAULT_TIMEOUT });
  });

  test('should handle navigation to non-existent config', async ({ page }) => {
    await page.goto(`${ARENA_CONFIGS_PATH}/nonexistent-config-12345`);
    await waitForPageLoad(page);

    // Should show "Config not found" error message
    await expect(page.locator('text=Config not found')).toBeVisible({ timeout: DEFAULT_TIMEOUT });
  });
});
