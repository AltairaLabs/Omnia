import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ProviderStatusBadge } from "./provider-status-badge";

describe("ProviderStatusBadge", () => {
  it("renders Unknown when no phase is provided", () => {
    render(<ProviderStatusBadge />);
    expect(screen.getByText("Unknown")).toBeInTheDocument();
  });

  it("renders a success-token badge for Ready", () => {
    render(<ProviderStatusBadge phase="Ready" />);
    const badge = screen.getByText("Ready");
    expect(badge.className).toContain("text-success");
    expect(badge.className).not.toMatch(/green/);
  });

  it("renders a destructive-token badge for Error", () => {
    render(<ProviderStatusBadge phase="Error" />);
    const badge = screen.getByText("Error");
    expect(badge.className).toContain("text-destructive");
    expect(badge.className).not.toMatch(/red/);
  });

  it("renders an outline badge with the raw phase for other states", () => {
    render(<ProviderStatusBadge phase="Pending" />);
    expect(screen.getByText("Pending")).toBeInTheDocument();
  });
});
