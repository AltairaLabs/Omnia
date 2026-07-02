import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ProviderTypeIcon } from "./provider-type-icon";

describe("ProviderTypeIcon", () => {
  it("keeps the vendor brand color for a known provider", () => {
    render(<ProviderTypeIcon type="openai" />);
    const el = screen.getByText("O");
    // Vendor identity swatch is intentionally NOT tokenized.
    expect(el.className).toContain("text-green-700");
  });

  it("uses the neutral token for the mock provider", () => {
    render(<ProviderTypeIcon type="mock" />);
    const el = screen.getByText("M");
    expect(el.className).toContain("bg-muted");
    expect(el.className).not.toMatch(/gray/);
  });

  it("falls back to the neutral token and initial for an unknown type", () => {
    render(<ProviderTypeIcon type="zephyr" />);
    const el = screen.getByText("Z");
    expect(el.className).toContain("bg-muted");
  });

  it("renders a neutral placeholder when no type is given", () => {
    render(<ProviderTypeIcon />);
    const el = screen.getByText("?");
    expect(el.className).toContain("bg-muted");
  });
});
