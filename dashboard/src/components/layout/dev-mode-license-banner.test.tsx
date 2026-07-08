/**
 * Tests for DevModeLicenseBanner — verifies it is brand-aware (no hardcoded
 * altairalabs.ai licensing link/copy) and uses status tokens, not raw palette.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { DevModeLicenseBanner } from "./dev-mode-license-banner";
import { BrandContext } from "@/components/branding/brand-provider";
import { OMNIA_BRAND, type BrandConfig } from "@/lib/branding/types";

const mockUseDevMode = vi.fn();
vi.mock("@/hooks/core", () => ({
  useDevMode: () => mockUseDevMode(),
}));

const mockUseLicense = vi.fn();
vi.mock("@/hooks/auth", () => ({
  useLicense: () => mockUseLicense(),
}));

const BRAND: BrandConfig = {
  ...OMNIA_BRAND,
  productName: "Acme Cloud",
  links: { upgradeUrl: "https://acme.example/licensing" },
};

function renderBanner(brand: BrandConfig) {
  return render(
    <BrandContext.Provider value={{ brand, setBrandOverride: () => {} }}>
      <DevModeLicenseBanner />
    </BrandContext.Provider>,
  );
}

describe("DevModeLicenseBanner", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Default: actually running the synthetic dev license.
    mockUseLicense.mockReturnValue({ license: { id: "dev-mode" } });
  });

  it("renders nothing when not in dev mode", () => {
    mockUseDevMode.mockReturnValue({ isDevMode: false, loading: false });
    const { container } = renderBanner(BRAND);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when a real license is installed, even in dev mode", () => {
    mockUseDevMode.mockReturnValue({ isDevMode: true, loading: false });
    mockUseLicense.mockReturnValue({ license: { id: "ent_real_customer_123" } });
    const { container } = renderBanner(BRAND);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing while loading", () => {
    mockUseDevMode.mockReturnValue({ isDevMode: true, loading: true });
    const { container } = renderBanner(BRAND);
    expect(container).toBeEmptyDOMElement();
  });

  it("uses the brand licensing link and name, not a hardcoded vendor URL", () => {
    mockUseDevMode.mockReturnValue({ isDevMode: true, loading: false });
    renderBanner(BRAND);
    const link = screen.getByRole("link", { name: "Acme Cloud" });
    expect(link).toHaveAttribute("href", "https://acme.example/licensing");
    expect(screen.queryByText(/altairalabs\.ai\/licensing/)).not.toBeInTheDocument();
  });

  it("uses status tokens rather than raw palette shades", () => {
    mockUseDevMode.mockReturnValue({ isDevMode: true, loading: false });
    const { container } = renderBanner(BRAND);
    expect(container.innerHTML).not.toMatch(/orange-\d/);
    expect(container.innerHTML).toContain("warning");
  });
});
