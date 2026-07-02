/**
 * Tests for BrandPresetSwitcher — the dev/demo-only header control that flips
 * the in-memory brand override between presets. It must never render in a real
 * (non-dev, non-demo) deployment, where there is exactly one brand.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BrandPresetSwitcher } from "./brand-preset-switcher";
import { getBrandPreset } from "@/lib/branding/presets";

const mockSetOverride = vi.fn();
vi.mock("@/hooks/use-brand", () => ({
  useBrand: () => ({ brand: { productName: "Omnia" }, setBrandOverride: mockSetOverride }),
}));

const mockDev = vi.fn();
const mockDemo = vi.fn();
vi.mock("@/hooks/core", () => ({
  useDevMode: () => mockDev(),
  useDemoMode: () => mockDemo(),
}));

describe("BrandPresetSwitcher", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockDev.mockReturnValue({ isDevMode: true, loading: false });
    mockDemo.mockReturnValue({ isDemoMode: false, loading: false });
  });

  it("renders nothing in a real deployment (not dev, not demo)", () => {
    mockDev.mockReturnValue({ isDevMode: false, loading: false });
    mockDemo.mockReturnValue({ isDemoMode: false, loading: false });
    const { container } = render(<BrandPresetSwitcher />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing while runtime config is loading", () => {
    mockDev.mockReturnValue({ isDevMode: false, loading: true });
    mockDemo.mockReturnValue({ isDemoMode: false, loading: true });
    const { container } = render(<BrandPresetSwitcher />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders the switcher trigger in dev mode", () => {
    render(<BrandPresetSwitcher />);
    expect(screen.getByTestId("brand-preset-switcher")).toBeInTheDocument();
  });

  it("renders in demo mode even when dev mode is off", () => {
    mockDev.mockReturnValue({ isDevMode: false, loading: false });
    mockDemo.mockReturnValue({ isDemoMode: true, loading: false });
    render(<BrandPresetSwitcher />);
    expect(screen.getByTestId("brand-preset-switcher")).toBeInTheDocument();
  });

  it("applies the selected preset as an in-memory override", async () => {
    const user = userEvent.setup();
    render(<BrandPresetSwitcher />);
    await user.click(screen.getByTestId("brand-preset-switcher"));
    await user.click(await screen.findByRole("menuitem", { name: /Acme Cloud/i }));
    expect(mockSetOverride).toHaveBeenCalledWith(getBrandPreset("acme"));
  });
});
