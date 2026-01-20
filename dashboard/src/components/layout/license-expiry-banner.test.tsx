/**
 * Tests for LicenseExpiryBanner component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { LicenseExpiryBanner } from "./license-expiry-banner";
import { useLicense } from "@/hooks/use-license";
import type { License } from "@/types/license";

vi.mock("@/hooks/use-license");

const mockUseLicense = vi.mocked(useLicense);

function createMockLicense(overrides: Partial<License> = {}): License {
  return {
    id: "test-license",
    tier: "enterprise",
    customer: "Test Corp",
    issuedAt: new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString(),
    expiresAt: new Date(Date.now() + 90 * 24 * 60 * 60 * 1000).toISOString(),
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
      maxScenarios: 0,
      maxWorkerReplicas: 0,
    },
    ...overrides,
  };
}

describe("LicenseExpiryBanner", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should not render for open-core license", () => {
    mockUseLicense.mockReturnValue({
      license: createMockLicense({ tier: "open-core" }),
      isLoading: false,
      error: undefined,
      canUseFeature: () => true,
      canUseSourceType: () => true,
      canUseJobType: () => true,
      canUseScheduling: () => true,
      canUseWorkerReplicas: () => true,
      canUseScenarioCount: () => true,
      isExpired: false,
      isEnterprise: false,
      refresh: vi.fn(),
    });

    const { container } = render(<LicenseExpiryBanner />);
    expect(container).toBeEmptyDOMElement();
  });

  it("should show error banner when license is expired", () => {
    mockUseLicense.mockReturnValue({
      license: createMockLicense({
        expiresAt: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(),
      }),
      isLoading: false,
      error: undefined,
      canUseFeature: () => true,
      canUseSourceType: () => true,
      canUseJobType: () => true,
      canUseScheduling: () => true,
      canUseWorkerReplicas: () => true,
      canUseScenarioCount: () => true,
      isExpired: true,
      isEnterprise: true,
      refresh: vi.fn(),
    });

    render(<LicenseExpiryBanner />);
    expect(screen.getByText(/license has expired/i)).toBeInTheDocument();
    expect(screen.getByText(/features are now disabled/i)).toBeInTheDocument();
  });

  it("should show warning banner when license expires within 30 days", () => {
    mockUseLicense.mockReturnValue({
      license: createMockLicense({
        expiresAt: new Date(Date.now() + 15 * 24 * 60 * 60 * 1000).toISOString(),
      }),
      isLoading: false,
      error: undefined,
      canUseFeature: () => true,
      canUseSourceType: () => true,
      canUseJobType: () => true,
      canUseScheduling: () => true,
      canUseWorkerReplicas: () => true,
      canUseScenarioCount: () => true,
      isExpired: false,
      isEnterprise: true,
      refresh: vi.fn(),
    });

    render(<LicenseExpiryBanner />);
    expect(screen.getByText(/expires in \d+ days?/i)).toBeInTheDocument();
    expect(screen.getByText(/contact sales/i)).toBeInTheDocument();
  });

  it("should show singular 'day' when 1 day remaining", () => {
    mockUseLicense.mockReturnValue({
      license: createMockLicense({
        expiresAt: new Date(Date.now() + 1 * 24 * 60 * 60 * 1000).toISOString(),
      }),
      isLoading: false,
      error: undefined,
      canUseFeature: () => true,
      canUseSourceType: () => true,
      canUseJobType: () => true,
      canUseScheduling: () => true,
      canUseWorkerReplicas: () => true,
      canUseScenarioCount: () => true,
      isExpired: false,
      isEnterprise: true,
      refresh: vi.fn(),
    });

    render(<LicenseExpiryBanner />);
    expect(screen.getByText(/expires in 1 day\./i)).toBeInTheDocument();
  });

  it("should not render when license has more than 30 days", () => {
    mockUseLicense.mockReturnValue({
      license: createMockLicense({
        expiresAt: new Date(Date.now() + 90 * 24 * 60 * 60 * 1000).toISOString(),
      }),
      isLoading: false,
      error: undefined,
      canUseFeature: () => true,
      canUseSourceType: () => true,
      canUseJobType: () => true,
      canUseScheduling: () => true,
      canUseWorkerReplicas: () => true,
      canUseScenarioCount: () => true,
      isExpired: false,
      isEnterprise: true,
      refresh: vi.fn(),
    });

    const { container } = render(<LicenseExpiryBanner />);
    expect(container).toBeEmptyDOMElement();
  });
});
