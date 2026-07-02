/**
 * Tests for BrandedBoundary — the shared, token-styled boundary shell used by
 * the app-level not-found / error / global-error pages. Verifies it surfaces
 * the active brand product name and renders only design-token color classes
 * (never hardcoded Tailwind palette shades).
 */

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { BrandedBoundary } from "./branded-boundary";
import { BrandContext } from "@/components/branding/brand-provider";
import { OMNIA_BRAND, type BrandConfig } from "@/lib/branding/types";

function renderWithBrand(ui: React.ReactElement, brand: BrandConfig) {
  return render(
    <BrandContext.Provider value={{ brand, setBrandOverride: () => {} }}>
      {ui}
    </BrandContext.Provider>,
  );
}

describe("BrandedBoundary", () => {
  it("renders the default brand product name", () => {
    render(<BrandedBoundary title="Page not found" description="It's gone." />);
    expect(screen.getByText(OMNIA_BRAND.productName)).toBeInTheDocument();
  });

  it("renders a white-label brand product name from context", () => {
    renderWithBrand(
      <BrandedBoundary title="Page not found" description="It's gone." />,
      { ...OMNIA_BRAND, productName: "Acme Cloud" },
    );
    expect(screen.getByText("Acme Cloud")).toBeInTheDocument();
    expect(screen.queryByText("Omnia")).not.toBeInTheDocument();
  });

  it("renders title, description, and status code", () => {
    render(
      <BrandedBoundary code="404" title="Page not found" description="It's gone." />,
    );
    expect(screen.getByText("404")).toBeInTheDocument();
    expect(screen.getByText("Page not found")).toBeInTheDocument();
    expect(screen.getByText("It's gone.")).toBeInTheDocument();
  });

  it("renders the action slot", () => {
    render(
      <BrandedBoundary
        title="Whoops"
        description="Try again."
        action={<button type="button">Back home</button>}
      />,
    );
    expect(screen.getByRole("button", { name: "Back home" })).toBeInTheDocument();
  });

  it("uses only token color classes (no hardcoded palette shades)", () => {
    const { container } = render(
      <BrandedBoundary code="500" title="Error" description="Boom." />,
    );
    const html = container.innerHTML;
    // Design tokens only — must not leak raw Tailwind palette classes.
    expect(html).not.toMatch(/(?:text|bg|border)-(?:red|green|blue|amber|orange|gray|slate)-\d/);
    expect(html).toContain("text-primary");
  });
});
