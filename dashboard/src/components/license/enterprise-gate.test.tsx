import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { EnterpriseGate, EnterpriseUpgradePage, useShowEnterpriseNav } from "./license-gate";

// Mock the useEnterpriseConfig hook
vi.mock("@/hooks/use-runtime-config", () => ({
  useEnterpriseConfig: vi.fn(),
}));

import { useEnterpriseConfig } from "@/hooks/use-runtime-config";

const mockUseEnterpriseConfig = vi.mocked(useEnterpriseConfig);

describe("EnterpriseGate", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders children when enterpriseEnabled is true", () => {
    mockUseEnterpriseConfig.mockReturnValue({
      enterpriseEnabled: true,
      hideEnterprise: false,
      showUpgradePrompts: false,
      loading: false,
    });

    render(
      <EnterpriseGate featureName="Test Feature">
        <div data-testid="children">Enterprise Content</div>
      </EnterpriseGate>
    );

    expect(screen.getByTestId("children")).toBeInTheDocument();
    expect(screen.getByText("Enterprise Content")).toBeInTheDocument();
  });

  it("renders upgrade prompt when enterprise is not enabled and not hidden", () => {
    mockUseEnterpriseConfig.mockReturnValue({
      enterpriseEnabled: false,
      hideEnterprise: false,
      showUpgradePrompts: true,
      loading: false,
    });

    render(
      <EnterpriseGate featureName="Arena Fleet">
        <div data-testid="children">Enterprise Content</div>
      </EnterpriseGate>
    );

    // Should not show children
    expect(screen.queryByTestId("children")).not.toBeInTheDocument();

    // Should show upgrade prompt
    expect(screen.getByText("Enterprise Feature")).toBeInTheDocument();
    expect(screen.getByText(/Arena Fleet is an enterprise feature/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Upgrade to Enterprise/i })).toBeInTheDocument();
  });

  it("renders nothing when hideEnterprise is true", () => {
    mockUseEnterpriseConfig.mockReturnValue({
      enterpriseEnabled: false,
      hideEnterprise: true,
      showUpgradePrompts: false,
      loading: false,
    });

    const { container } = render(
      <EnterpriseGate featureName="Test Feature">
        <div data-testid="children">Enterprise Content</div>
      </EnterpriseGate>
    );

    // Should not show children
    expect(screen.queryByTestId("children")).not.toBeInTheDocument();

    // Should not show upgrade prompt either
    expect(screen.queryByText("Enterprise Feature")).not.toBeInTheDocument();

    // Container should be empty
    expect(container.innerHTML).toBe("");
  });

  it("renders nothing while loading", () => {
    mockUseEnterpriseConfig.mockReturnValue({
      enterpriseEnabled: false,
      hideEnterprise: false,
      showUpgradePrompts: true,
      loading: true,
    });

    const { container } = render(
      <EnterpriseGate featureName="Test Feature">
        <div data-testid="children">Enterprise Content</div>
      </EnterpriseGate>
    );

    // Should not show anything while loading
    expect(container.innerHTML).toBe("");
  });

  it("renders custom fallback when provided", () => {
    mockUseEnterpriseConfig.mockReturnValue({
      enterpriseEnabled: false,
      hideEnterprise: false,
      showUpgradePrompts: true,
      loading: false,
    });

    render(
      <EnterpriseGate
        featureName="Test Feature"
        fallback={<div data-testid="custom-fallback">Custom Fallback</div>}
      >
        <div data-testid="children">Enterprise Content</div>
      </EnterpriseGate>
    );

    expect(screen.queryByTestId("children")).not.toBeInTheDocument();
    expect(screen.getByTestId("custom-fallback")).toBeInTheDocument();
    expect(screen.getByText("Custom Fallback")).toBeInTheDocument();
  });
});

describe("EnterpriseUpgradePage", () => {
  it("renders upgrade page with feature name", () => {
    render(<EnterpriseUpgradePage featureName="Arena Fleet" />);

    expect(screen.getByText("Enterprise Feature")).toBeInTheDocument();
    expect(screen.getByText(/Arena Fleet is an enterprise feature/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Upgrade to Enterprise/i })).toHaveAttribute(
      "href",
      "https://altairalabs.ai/enterprise"
    );
    expect(screen.getByText(/sales@altairalabs.ai/)).toBeInTheDocument();
  });
});

describe("useShowEnterpriseNav", () => {
  // This is tested implicitly through the sidebar tests, but we can add a unit test
  it("returns showEnterpriseNav=true when hideEnterprise is false", () => {
    mockUseEnterpriseConfig.mockReturnValue({
      enterpriseEnabled: false,
      hideEnterprise: false,
      showUpgradePrompts: true,
      loading: false,
    });

    // We need to test the hook through a component
    const TestComponent = () => {
      const { showEnterpriseNav } = useShowEnterpriseNav();
      return <div data-testid="result">{showEnterpriseNav ? "show" : "hide"}</div>;
    };

    render(<TestComponent />);
    expect(screen.getByTestId("result")).toHaveTextContent("show");
  });

  it("returns showEnterpriseNav=false when hideEnterprise is true", () => {
    mockUseEnterpriseConfig.mockReturnValue({
      enterpriseEnabled: false,
      hideEnterprise: true,
      showUpgradePrompts: false,
      loading: false,
    });

    const TestComponent = () => {
      const { showEnterpriseNav } = useShowEnterpriseNav();
      return <div data-testid="result">{showEnterpriseNav ? "show" : "hide"}</div>;
    };

    render(<TestComponent />);
    expect(screen.getByTestId("result")).toHaveTextContent("hide");
  });
});
