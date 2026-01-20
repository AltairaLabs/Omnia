import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import {
  LicenseGate,
  RequireEnterprise,
  UpgradeBanner,
  FeatureBadge,
  LicenseInfo,
} from "./license-gate";
import { OPEN_CORE_LICENSE, type License } from "@/types/license";

// Mock the useLicense hook
const mockUseLicense = vi.fn();

vi.mock("@/hooks/use-license", () => ({
  useLicense: () => mockUseLicense(),
}));

describe("license-gate components", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("LicenseGate", () => {
    it("should render children when feature is enabled", () => {
      mockUseLicense.mockReturnValue({
        license: OPEN_CORE_LICENSE,
        canUseFeature: () => true,
        isEnterprise: false,
        isExpired: false,
      });

      render(
        <LicenseGate feature="gitSource">
          <div data-testid="child-content">Git Source Form</div>
        </LicenseGate>
      );

      expect(screen.getByTestId("child-content")).toBeInTheDocument();
      expect(screen.getByText("Git Source Form")).toBeInTheDocument();
    });

    it("should render default fallback when feature is disabled", () => {
      mockUseLicense.mockReturnValue({
        license: OPEN_CORE_LICENSE,
        canUseFeature: () => false,
        isEnterprise: false,
        isExpired: false,
      });

      render(
        <LicenseGate feature="gitSource">
          <div data-testid="child-content">Git Source Form</div>
        </LicenseGate>
      );

      expect(screen.queryByTestId("child-content")).not.toBeInTheDocument();
      expect(screen.getByText("Enterprise Feature")).toBeInTheDocument();
      expect(screen.getByText(/Git Sources/)).toBeInTheDocument();
    });

    it("should render custom fallback when provided", () => {
      mockUseLicense.mockReturnValue({
        license: OPEN_CORE_LICENSE,
        canUseFeature: () => false,
        isEnterprise: false,
        isExpired: false,
      });

      render(
        <LicenseGate
          feature="gitSource"
          fallback={<div data-testid="custom-fallback">Custom Fallback</div>}
        >
          <div data-testid="child-content">Git Source Form</div>
        </LicenseGate>
      );

      expect(screen.queryByTestId("child-content")).not.toBeInTheDocument();
      expect(screen.getByTestId("custom-fallback")).toBeInTheDocument();
      expect(screen.getByText("Custom Fallback")).toBeInTheDocument();
    });

    it("should check the correct feature", () => {
      const canUseFeature = vi.fn().mockReturnValue(true);
      mockUseLicense.mockReturnValue({
        license: OPEN_CORE_LICENSE,
        canUseFeature,
        isEnterprise: false,
        isExpired: false,
      });

      render(
        <LicenseGate feature="loadTesting">
          <div>Content</div>
        </LicenseGate>
      );

      expect(canUseFeature).toHaveBeenCalledWith("loadTesting");
    });
  });

  describe("RequireEnterprise", () => {
    it("should render children when enterprise license is active", () => {
      mockUseLicense.mockReturnValue({
        license: { ...OPEN_CORE_LICENSE, tier: "enterprise" },
        canUseFeature: () => true,
        isEnterprise: true,
        isExpired: false,
      });

      render(
        <RequireEnterprise>
          <div data-testid="enterprise-content">Enterprise Only Feature</div>
        </RequireEnterprise>
      );

      expect(screen.getByTestId("enterprise-content")).toBeInTheDocument();
    });

    it("should render default fallback for non-enterprise", () => {
      mockUseLicense.mockReturnValue({
        license: OPEN_CORE_LICENSE,
        canUseFeature: () => false,
        isEnterprise: false,
        isExpired: false,
      });

      render(
        <RequireEnterprise>
          <div data-testid="enterprise-content">Enterprise Only Feature</div>
        </RequireEnterprise>
      );

      expect(screen.queryByTestId("enterprise-content")).not.toBeInTheDocument();
      expect(screen.getByText("Enterprise Feature")).toBeInTheDocument();
    });

    it("should render custom fallback when provided", () => {
      mockUseLicense.mockReturnValue({
        license: OPEN_CORE_LICENSE,
        canUseFeature: () => false,
        isEnterprise: false,
        isExpired: false,
      });

      render(
        <RequireEnterprise
          fallback={<div data-testid="custom-fallback">Please upgrade</div>}
        >
          <div data-testid="enterprise-content">Enterprise Feature</div>
        </RequireEnterprise>
      );

      expect(screen.queryByTestId("enterprise-content")).not.toBeInTheDocument();
      expect(screen.getByTestId("custom-fallback")).toBeInTheDocument();
    });
  });

  describe("UpgradeBanner", () => {
    it("should render feature name in banner", () => {
      render(<UpgradeBanner feature="Git Sources" />);

      expect(screen.getByText(/Git Sources/)).toBeInTheDocument();
      expect(screen.getByText("Enterprise Feature")).toBeInTheDocument();
    });

    it("should render upgrade link", () => {
      render(<UpgradeBanner feature="Git Sources" />);

      const link = screen.getByRole("link", { name: /Upgrade to Enterprise/i });
      expect(link).toBeInTheDocument();
      expect(link).toHaveAttribute("href", "https://altairalabs.ai/enterprise");
      expect(link).toHaveAttribute("target", "_blank");
    });

    it("should render custom description when provided", () => {
      render(
        <UpgradeBanner
          feature="Git Sources"
          description="Custom description for the feature"
        />
      );

      expect(screen.getByText("Custom description for the feature")).toBeInTheDocument();
    });

    it("should render custom upgrade URL when provided", () => {
      render(
        <UpgradeBanner
          feature="Git Sources"
          upgradeUrl="https://custom.url/upgrade"
        />
      );

      const link = screen.getByRole("link", { name: /Upgrade to Enterprise/i });
      expect(link).toHaveAttribute("href", "https://custom.url/upgrade");
    });

    it("should render compact variant", () => {
      render(<UpgradeBanner feature="Git Sources" compact />);

      expect(screen.getByText(/Git Sources requires an Enterprise license/)).toBeInTheDocument();
      const link = screen.getByRole("link", { name: /Upgrade/i });
      expect(link).toBeInTheDocument();
    });
  });

  describe("FeatureBadge", () => {
    it("should render available badge when feature is enabled", () => {
      mockUseLicense.mockReturnValue({
        license: OPEN_CORE_LICENSE,
        canUseFeature: () => true,
        isEnterprise: true,
        isExpired: false,
      });

      render(<FeatureBadge feature="gitSource" />);

      expect(screen.getByText("Available")).toBeInTheDocument();
    });

    it("should render enterprise badge when feature is disabled", () => {
      mockUseLicense.mockReturnValue({
        license: OPEN_CORE_LICENSE,
        canUseFeature: () => false,
        isEnterprise: false,
        isExpired: false,
      });

      render(<FeatureBadge feature="gitSource" />);

      expect(screen.getByText("Enterprise")).toBeInTheDocument();
    });

    it("should render custom available text", () => {
      mockUseLicense.mockReturnValue({
        license: OPEN_CORE_LICENSE,
        canUseFeature: () => true,
        isEnterprise: true,
        isExpired: false,
      });

      render(<FeatureBadge feature="gitSource" availableText="Enabled" />);

      expect(screen.getByText("Enabled")).toBeInTheDocument();
    });

    it("should render custom enterprise text", () => {
      mockUseLicense.mockReturnValue({
        license: OPEN_CORE_LICENSE,
        canUseFeature: () => false,
        isEnterprise: false,
        isExpired: false,
      });

      render(<FeatureBadge feature="gitSource" enterpriseText="Pro" />);

      expect(screen.getByText("Pro")).toBeInTheDocument();
    });
  });

  describe("LicenseInfo", () => {
    it("should render open core badge for non-enterprise", () => {
      mockUseLicense.mockReturnValue({
        license: OPEN_CORE_LICENSE,
        canUseFeature: () => false,
        isEnterprise: false,
        isExpired: false,
      });

      render(<LicenseInfo />);

      expect(screen.getByText("Open Core")).toBeInTheDocument();
    });

    it("should render enterprise badge for enterprise license", () => {
      mockUseLicense.mockReturnValue({
        license: { ...OPEN_CORE_LICENSE, tier: "enterprise", customer: "Test Corp" },
        canUseFeature: () => true,
        isEnterprise: true,
        isExpired: false,
      });

      render(<LicenseInfo />);

      expect(screen.getByText("Enterprise")).toBeInTheDocument();
    });

    it("should render expired badge when license is expired", () => {
      mockUseLicense.mockReturnValue({
        license: OPEN_CORE_LICENSE,
        canUseFeature: () => false,
        isEnterprise: false,
        isExpired: true,
      });

      render(<LicenseInfo />);

      expect(screen.getByText("Expired")).toBeInTheDocument();
    });

    it("should render detailed info when detailed prop is true", () => {
      const enterpriseLicense: License = {
        ...OPEN_CORE_LICENSE,
        tier: "enterprise",
        customer: "Acme Corp",
        expiresAt: new Date("2025-12-31").toISOString(),
      };

      mockUseLicense.mockReturnValue({
        license: enterpriseLicense,
        canUseFeature: () => true,
        isEnterprise: true,
        isExpired: false,
      });

      render(<LicenseInfo detailed />);

      expect(screen.getByText("Customer")).toBeInTheDocument();
      expect(screen.getByText("Acme Corp")).toBeInTheDocument();
      expect(screen.getByText("Expires")).toBeInTheDocument();
    });

    it("should show upgrade button for non-enterprise in detailed view", () => {
      mockUseLicense.mockReturnValue({
        license: OPEN_CORE_LICENSE,
        canUseFeature: () => false,
        isEnterprise: false,
        isExpired: false,
      });

      render(<LicenseInfo detailed />);

      const link = screen.getByRole("link", { name: /Upgrade to Enterprise/i });
      expect(link).toBeInTheDocument();
    });

    it("should not show customer info for open-core in detailed view", () => {
      mockUseLicense.mockReturnValue({
        license: OPEN_CORE_LICENSE,
        canUseFeature: () => false,
        isEnterprise: false,
        isExpired: false,
      });

      render(<LicenseInfo detailed />);

      expect(screen.queryByText("Customer")).not.toBeInTheDocument();
      expect(screen.queryByText("Expires")).not.toBeInTheDocument();
    });
  });
});
