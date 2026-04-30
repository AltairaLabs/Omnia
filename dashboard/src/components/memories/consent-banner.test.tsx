/**
 * Tests for ConsentBanner.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const { mockUseConsent, mockUseUpdateConsent, mockUseEnterpriseConfig } =
  vi.hoisted(() => ({
    mockUseConsent: vi.fn(),
    mockUseUpdateConsent: vi.fn(),
    mockUseEnterpriseConfig: vi.fn(),
  }));

vi.mock("@/hooks/use-consent", () => ({
  useConsent: mockUseConsent,
  useUpdateConsent: mockUseUpdateConsent,
}));

vi.mock("@/hooks/core", () => ({
  useEnterpriseConfig: mockUseEnterpriseConfig,
}));

import { ConsentBanner } from "./consent-banner";

const mockMutate = vi.fn();

function setupMocks(
  grants: string[] = [],
  isLoading = false,
  enterprise: { enterpriseEnabled?: boolean; hideEnterprise?: boolean; loading?: boolean } = {},
) {
  mockUseConsent.mockReturnValue({
    data: { grants, defaults: [], denied: [] },
    isLoading,
  });
  mockUseUpdateConsent.mockReturnValue({
    mutate: mockMutate,
    isPending: false,
  });
  mockUseEnterpriseConfig.mockReturnValue({
    enterpriseEnabled: enterprise.enterpriseEnabled ?? true,
    hideEnterprise: enterprise.hideEnterprise ?? false,
    showUpgradePrompts: !(enterprise.enterpriseEnabled ?? true) && !(enterprise.hideEnterprise ?? false),
    loading: enterprise.loading ?? false,
  });
}

describe("ConsentBanner", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockMutate.mockReset();
    window.localStorage.clear();
  });

  it("renders loading skeleton while runtime config is loading", () => {
    setupMocks([], false, { loading: true });
    render(<ConsentBanner />);
    expect(screen.getByTestId("consent-banner")).toBeInTheDocument();
    expect(screen.queryByTestId("consent-toggle-memory:identity")).not.toBeInTheDocument();
  });

  it("renders loading skeleton when consent data is loading (EE on)", () => {
    setupMocks([], true);
    render(<ConsentBanner />);
    expect(screen.getByTestId("consent-banner")).toBeInTheDocument();
    expect(screen.queryByTestId("consent-toggle-memory:identity")).not.toBeInTheDocument();
  });

  it("renders all 6 category toggles when EE is enabled", () => {
    setupMocks([]);
    render(<ConsentBanner />);

    expect(screen.getByTestId("consent-toggle-memory:identity")).toBeInTheDocument();
    expect(screen.getByTestId("consent-toggle-memory:location")).toBeInTheDocument();
    expect(screen.getByTestId("consent-toggle-memory:health")).toBeInTheDocument();
    expect(screen.getByTestId("consent-toggle-memory:preferences")).toBeInTheDocument();
    expect(screen.getByTestId("consent-toggle-memory:context")).toBeInTheDocument();
    expect(screen.getByTestId("consent-toggle-memory:history")).toBeInTheDocument();
  });

  it("shows Privacy Consent title with shield icon (EE on)", () => {
    setupMocks([]);
    render(<ConsentBanner />);
    expect(screen.getByText("Privacy Consent")).toBeInTheDocument();
  });

  it("shows PII toggle as checked when category is in grants", () => {
    setupMocks(["memory:identity", "memory:health"]);
    render(<ConsentBanner />);

    expect(screen.getByTestId("consent-toggle-memory:identity")).toBeChecked();
    expect(screen.getByTestId("consent-toggle-memory:location")).not.toBeChecked();
    expect(screen.getByTestId("consent-toggle-memory:health")).toBeChecked();
  });

  it("shows PII toggle as unchecked when category is not in grants", () => {
    setupMocks([]);
    render(<ConsentBanner />);

    expect(screen.getByTestId("consent-toggle-memory:identity")).not.toBeChecked();
    expect(screen.getByTestId("consent-toggle-memory:location")).not.toBeChecked();
    expect(screen.getByTestId("consent-toggle-memory:health")).not.toBeChecked();
  });

  it("calls mutate with grants when PII toggle is turned on", async () => {
    const user = userEvent.setup();
    setupMocks([]);
    render(<ConsentBanner />);

    await user.click(screen.getByTestId("consent-toggle-memory:identity"));
    expect(mockMutate).toHaveBeenCalledWith({ grants: ["memory:identity"] });
  });

  it("calls mutate with revocations when PII toggle is turned off", async () => {
    const user = userEvent.setup();
    setupMocks(["memory:identity"]);
    render(<ConsentBanner />);

    await user.click(screen.getByTestId("consent-toggle-memory:identity"));
    expect(mockMutate).toHaveBeenCalledWith({ revocations: ["memory:identity"] });
  });

  it("default categories are always checked and disabled", () => {
    setupMocks([]);
    render(<ConsentBanner />);

    for (const cat of ["preferences", "context", "history"]) {
      const toggle = screen.getByTestId(`consent-toggle-memory:${cat}`);
      expect(toggle).toBeChecked();
      expect(toggle).toBeDisabled();
    }
  });

  it("shows (default) label for non-PII categories", () => {
    setupMocks([]);
    render(<ConsentBanner />);
    expect(screen.getAllByText("(default)")).toHaveLength(3);
  });

  it("disables PII toggles when mutation is pending", () => {
    mockUseConsent.mockReturnValue({ data: { grants: [], defaults: [], denied: [] }, isLoading: false });
    mockUseUpdateConsent.mockReturnValue({ mutate: mockMutate, isPending: true });
    mockUseEnterpriseConfig.mockReturnValue({
      enterpriseEnabled: true,
      hideEnterprise: false,
      showUpgradePrompts: false,
      loading: false,
    });

    render(<ConsentBanner />);

    expect(screen.getByTestId("consent-toggle-memory:identity")).toBeDisabled();
    expect(screen.getByTestId("consent-toggle-memory:location")).toBeDisabled();
    expect(screen.getByTestId("consent-toggle-memory:health")).toBeDisabled();
  });

  describe("OSS mode (enterpriseEnabled=false)", () => {
    it("renders the dismissable Enterprise upgrade CTA, no toggles", () => {
      setupMocks([], false, { enterpriseEnabled: false });
      render(<ConsentBanner />);

      expect(screen.getByTestId("upgrade-banner-compact")).toBeInTheDocument();
      expect(screen.getByTestId("upgrade-banner-dismiss")).toBeInTheDocument();
      expect(screen.queryByTestId("consent-toggle-memory:identity")).not.toBeInTheDocument();
      expect(screen.queryByText("Privacy Consent")).not.toBeInTheDocument();
    });

    it("renders nothing when hideEnterprise is true", () => {
      setupMocks([], false, { enterpriseEnabled: false, hideEnterprise: true });
      const { container } = render(<ConsentBanner />);
      expect(container).toBeEmptyDOMElement();
    });

    it("hides the CTA after dismissal and remembers across re-renders", async () => {
      const user = userEvent.setup();
      setupMocks([], false, { enterpriseEnabled: false });
      const { rerender } = render(<ConsentBanner />);

      await user.click(screen.getByTestId("upgrade-banner-dismiss"));
      expect(screen.queryByTestId("upgrade-banner-compact")).not.toBeInTheDocument();

      rerender(<ConsentBanner />);
      expect(screen.queryByTestId("upgrade-banner-compact")).not.toBeInTheDocument();
      expect(window.localStorage.getItem("omnia.upgradeBanner.dismissed.memory-consent-banner")).toBe("1");
    });
  });
});
