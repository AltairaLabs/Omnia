import { test, expect } from '../fixtures/coverage';
import type { Page } from '@playwright/test';

/**
 * E2E tests for Arena Fleet license gating.
 *
 * These tests verify that license-gated features in the Arena UI work correctly.
 * The Arena pages use the DataService abstraction which provides mock data in
 * demo mode, allowing these tests to run without k8s access.
 *
 * Tests mock the license API to simulate both Open Core and Enterprise licenses,
 * then verify the UI correctly enables/disables features based on license tier.
 *
 * The license gating functionality is also thoroughly tested via unit tests
 * in src/components/arena/job-dialog.test.tsx
 */

// License mock data
const OPEN_CORE_LICENSE = {
  id: "open-core",
  tier: "open-core",
  customer: "Open Core User",
  features: {
    gitSource: false,
    ociSource: false,
    s3Source: false,
    loadTesting: false,
    dataGeneration: false,
    scheduling: false,
    distributedWorkers: false,
  },
  limits: {
    maxScenarios: 10,
    maxWorkerReplicas: 1,
  },
  issuedAt: new Date().toISOString(),
  expiresAt: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString(),
};

const ENTERPRISE_LICENSE = {
  id: "enterprise-test",
  tier: "enterprise",
  customer: "Test Enterprise",
  features: {
    gitSource: true,
    ociSource: true,
    s3Source: true,
    loadTesting: true,
    dataGeneration: true,
    scheduling: true,
    distributedWorkers: true,
  },
  limits: {
    maxScenarios: 0, // unlimited
    maxWorkerReplicas: 0, // unlimited
  },
  issuedAt: new Date().toISOString(),
  expiresAt: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString(),
};

// Common constants
const API_VERSION = "omnia.altairalabs.ai/v1alpha1";
const TEST_CONFIG_NAME = "test-config";
const TEST_SOURCE_NAME = "test-source";

// Page paths
const ARENA_JOBS_PATH = '/arena/jobs';
const ARENA_SOURCES_PATH = '/arena/sources';

// Selectors
const SELECTORS = {
  createJobButton: 'button:has-text("Create Job")',
  createSourceButton: 'button:has-text("Create Source")',
  evaluationButton: 'button:has-text("Evaluation")',
  configMapButton: 'button:has-text("ConfigMap")',
  roleDialog: 'role=dialog',
  roleListbox: '[role="listbox"]',
  roleOption: '[role="option"]',
  loadTestOption: '[role="option"]:has-text("Load Test")',
  dataGenOption: '[role="option"]:has-text("Data Generation")',
  evaluationOption: '[role="option"]:has-text("Evaluation")',
  gitOption: '[role="option"]:has-text("Git")',
  ociOption: '[role="option"]:has-text("OCI")',
  s3Option: '[role="option"]:has-text("S3")',
  workersInput: 'input[id="workers"]',
  enterpriseText: 'text=Enterprise',
  workerLimitText: 'text=/Limited to 1 worker/',
  workerLimitPattern: 'text=/Limited to.*worker/',
  loadTestOptionsText: 'text=Load Test Options',
} as const;

// Timeouts
const DEFAULT_TIMEOUT = 10000;
const DIALOG_TIMEOUT = 5000;

// Attributes
const DATA_DISABLED_ATTR = 'data-disabled';

// Mock Arena data
const MOCK_ARENA_STATS = {
  sources: { active: 2, failed: 0 },
  configs: { total: 3, scenarios: 15 },
  jobs: { running: 1, queued: 0, completed: 5, successRate: 0.8 },
};

const MOCK_ARENA_JOBS = [
  {
    apiVersion: API_VERSION,
    kind: "ArenaJob",
    metadata: { name: "test-job-1", creationTimestamp: new Date().toISOString() },
    spec: { type: "evaluation", configRef: { name: TEST_CONFIG_NAME }, workers: { replicas: 1 } },
    status: { phase: "Completed", completedTasks: 10, totalTasks: 10 },
  },
];

const MOCK_ARENA_CONFIGS = [
  {
    apiVersion: API_VERSION,
    kind: "ArenaConfig",
    metadata: { name: TEST_CONFIG_NAME },
    spec: { sourceRef: { name: TEST_SOURCE_NAME } },
    status: { phase: "Ready", scenarioCount: 5 },
  },
];

const MOCK_ARENA_SOURCES = [
  {
    apiVersion: API_VERSION,
    kind: "ArenaSource",
    metadata: { name: TEST_SOURCE_NAME },
    spec: { type: "configmap", configMapRef: { name: "prompts" } },
    status: { phase: "Ready" },
  },
];

/**
 * Setup all necessary API mocks for Arena pages.
 */
async function setupArenaMocks(page: Page, license: typeof OPEN_CORE_LICENSE | typeof ENTERPRISE_LICENSE) {
  // Mock license API
  await page.route('**/api/license', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(license),
    });
  });

  // Mock Arena stats API
  await page.route('**/api/workspaces/*/arena/stats', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(MOCK_ARENA_STATS),
    });
  });

  // Mock Arena jobs API
  await page.route('**/api/workspaces/*/arena/jobs', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ items: MOCK_ARENA_JOBS }),
      });
    } else {
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_ARENA_JOBS[0]),
      });
    }
  });

  // Mock Arena configs API
  await page.route('**/api/workspaces/*/arena/configs', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ items: MOCK_ARENA_CONFIGS }),
    });
  });

  // Mock Arena sources API
  await page.route('**/api/workspaces/*/arena/sources', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ items: MOCK_ARENA_SOURCES }),
      });
    } else {
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_ARENA_SOURCES[0]),
      });
    }
  });
}

test.describe('Arena License API', () => {
  test('should return Open Core license by default in demo mode', async ({ page }) => {
    // Navigate to a simple page and check the license API response
    await page.goto('/');

    // Make a fetch request to the license API
    const response = await page.evaluate(async () => {
      const res = await fetch('/api/license');
      return res.json();
    });

    // In demo mode without DEMO_ENTERPRISE_LICENSE, should return open-core
    expect(response.tier).toBe('open-core');
    expect(response.features.loadTesting).toBe(false);
    expect(response.features.dataGeneration).toBe(false);
    expect(response.limits.maxWorkerReplicas).toBe(1);
  });

  test('should respect mocked Enterprise license', async ({ page }) => {
    // Mock the license API to return enterprise
    await page.route('**/api/license', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(ENTERPRISE_LICENSE),
      });
    });

    await page.goto('/');

    // Make a fetch request to the license API
    const response = await page.evaluate(async () => {
      const res = await fetch('/api/license');
      return res.json();
    });

    expect(response.tier).toBe('enterprise');
    expect(response.features.loadTesting).toBe(true);
    expect(response.features.dataGeneration).toBe(true);
    expect(response.limits.maxWorkerReplicas).toBe(0); // unlimited
  });
});

test.describe('Arena License Gating - Open Core', () => {
  test.beforeEach(async ({ page }) => {
    await setupArenaMocks(page, OPEN_CORE_LICENSE);
  });

  test.describe('Job Dialog License Gating', () => {
    test('should disable Load Test and Data Generation job types', async ({ page }) => {
      await page.goto(ARENA_JOBS_PATH);
      await page.waitForLoadState('networkidle');

      const createButton = page.locator(SELECTORS.createJobButton);
      await expect(createButton).toBeVisible({ timeout: DEFAULT_TIMEOUT });
      await createButton.click();

      await page.waitForSelector(SELECTORS.roleDialog, { timeout: DIALOG_TIMEOUT });

      const jobTypeSelect = page.locator(SELECTORS.evaluationButton).first();
      await jobTypeSelect.click();

      const loadTestOption = page.locator(SELECTORS.loadTestOption);
      await expect(loadTestOption).toHaveAttribute(DATA_DISABLED_ATTR);

      const dataGenOption = page.locator(SELECTORS.dataGenOption);
      await expect(dataGenOption).toHaveAttribute(DATA_DISABLED_ATTR);

      const enterpriseBadges = page.locator(`${SELECTORS.roleListbox} >> ${SELECTORS.enterpriseText}`);
      const badgeCount = await enterpriseBadges.count();
      expect(badgeCount).toBe(2);
    });

    test('should show worker limit message', async ({ page }) => {
      await page.goto(ARENA_JOBS_PATH);
      await page.waitForLoadState('networkidle');

      const createButton = page.locator(SELECTORS.createJobButton);
      await expect(createButton).toBeVisible({ timeout: DEFAULT_TIMEOUT });
      await createButton.click();

      await page.waitForSelector(SELECTORS.roleDialog, { timeout: DIALOG_TIMEOUT });

      const workerLimitText = page.locator(SELECTORS.workerLimitText);
      await expect(workerLimitText).toBeVisible();
    });

    test('should enforce max workers input limit', async ({ page }) => {
      await page.goto(ARENA_JOBS_PATH);
      await page.waitForLoadState('networkidle');

      const createButton = page.locator(SELECTORS.createJobButton);
      await expect(createButton).toBeVisible({ timeout: DEFAULT_TIMEOUT });
      await createButton.click();

      await page.waitForSelector(SELECTORS.roleDialog, { timeout: DIALOG_TIMEOUT });

      const workersInput = page.locator(SELECTORS.workersInput);
      await expect(workersInput).toBeVisible();

      const maxValue = await workersInput.getAttribute('max');
      expect(maxValue).toBe('1');
    });
  });

  test.describe('Source Dialog License Gating', () => {
    test('should disable Git, OCI, and S3 source types', async ({ page }) => {
      await page.goto(ARENA_SOURCES_PATH);
      await page.waitForLoadState('networkidle');

      const createButton = page.locator(SELECTORS.createSourceButton);
      await expect(createButton).toBeVisible({ timeout: DEFAULT_TIMEOUT });
      await createButton.click();

      await page.waitForSelector(SELECTORS.roleDialog, { timeout: DIALOG_TIMEOUT });

      const sourceTypeSelect = page.locator(SELECTORS.configMapButton).first();
      await sourceTypeSelect.click();

      const gitOption = page.locator(SELECTORS.gitOption);
      await expect(gitOption).toHaveAttribute(DATA_DISABLED_ATTR);

      const ociOption = page.locator(SELECTORS.ociOption);
      await expect(ociOption).toHaveAttribute(DATA_DISABLED_ATTR);

      const s3Option = page.locator(SELECTORS.s3Option);
      await expect(s3Option).toHaveAttribute(DATA_DISABLED_ATTR);
    });
  });
});

test.describe('Arena License Gating - Enterprise', () => {
  test.beforeEach(async ({ page }) => {
    await setupArenaMocks(page, ENTERPRISE_LICENSE);
  });

  test.describe('Job Dialog - All Features Enabled', () => {
    test('should enable all job types', async ({ page }) => {
      await page.goto(ARENA_JOBS_PATH);
      await page.waitForLoadState('networkidle');

      const createButton = page.locator(SELECTORS.createJobButton);
      await expect(createButton).toBeVisible({ timeout: DEFAULT_TIMEOUT });
      await createButton.click();

      await page.waitForSelector(SELECTORS.roleDialog, { timeout: DIALOG_TIMEOUT });

      const jobTypeSelect = page.locator(SELECTORS.evaluationButton).first();
      await jobTypeSelect.click();

      const evaluationOption = page.locator(SELECTORS.evaluationOption);
      await expect(evaluationOption).not.toHaveAttribute(DATA_DISABLED_ATTR);

      const loadTestOption = page.locator(SELECTORS.loadTestOption);
      await expect(loadTestOption).not.toHaveAttribute(DATA_DISABLED_ATTR);

      const dataGenOption = page.locator(SELECTORS.dataGenOption);
      await expect(dataGenOption).not.toHaveAttribute(DATA_DISABLED_ATTR);

      const enterpriseBadges = page.locator(`${SELECTORS.roleListbox} >> ${SELECTORS.enterpriseText}`);
      const badgeCount = await enterpriseBadges.count();
      expect(badgeCount).toBe(0);
    });

    test('should not show worker limit message', async ({ page }) => {
      await page.goto(ARENA_JOBS_PATH);
      await page.waitForLoadState('networkidle');

      const createButton = page.locator(SELECTORS.createJobButton);
      await expect(createButton).toBeVisible({ timeout: DEFAULT_TIMEOUT });
      await createButton.click();

      await page.waitForSelector(SELECTORS.roleDialog, { timeout: DIALOG_TIMEOUT });

      const workerLimitText = page.locator(SELECTORS.workerLimitPattern);
      await expect(workerLimitText).not.toBeVisible();
    });

    test('should allow selecting Load Test job type', async ({ page }) => {
      await page.goto(ARENA_JOBS_PATH);
      await page.waitForLoadState('networkidle');

      const createButton = page.locator(SELECTORS.createJobButton);
      await expect(createButton).toBeVisible({ timeout: DEFAULT_TIMEOUT });
      await createButton.click();

      await page.waitForSelector(SELECTORS.roleDialog, { timeout: DIALOG_TIMEOUT });

      const jobTypeSelect = page.locator(SELECTORS.evaluationButton).first();
      await jobTypeSelect.click();

      const loadTestOption = page.locator(SELECTORS.loadTestOption);
      await loadTestOption.click();

      const loadTestOptions = page.locator(SELECTORS.loadTestOptionsText);
      await expect(loadTestOptions).toBeVisible({ timeout: DIALOG_TIMEOUT });
    });
  });

  test.describe('Source Dialog - All Features Enabled', () => {
    test('should enable all source types', async ({ page }) => {
      await page.goto(ARENA_SOURCES_PATH);
      await page.waitForLoadState('networkidle');

      const createButton = page.locator(SELECTORS.createSourceButton);
      await expect(createButton).toBeVisible({ timeout: DEFAULT_TIMEOUT });
      await createButton.click();

      await page.waitForSelector(SELECTORS.roleDialog, { timeout: DIALOG_TIMEOUT });

      const sourceTypeSelect = page.locator(SELECTORS.configMapButton).first();
      await sourceTypeSelect.click();

      const gitOption = page.locator(SELECTORS.gitOption);
      await expect(gitOption).not.toHaveAttribute(DATA_DISABLED_ATTR);

      const ociOption = page.locator(SELECTORS.ociOption);
      await expect(ociOption).not.toHaveAttribute(DATA_DISABLED_ATTR);

      const s3Option = page.locator(SELECTORS.s3Option);
      await expect(s3Option).not.toHaveAttribute(DATA_DISABLED_ATTR);
    });
  });
});
