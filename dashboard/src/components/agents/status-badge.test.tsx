import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { StatusBadge } from "./status-badge";

describe("StatusBadge", () => {
  it("uses the success token for Running, not a palette class", () => {
    render(<StatusBadge phase="Running" />);
    const badge = screen.getByTestId("status-badge");
    expect(badge.className).toContain("text-success");
    expect(badge.className).not.toMatch(/-(green|red|yellow|orange|gray)-\d/);
  });

  it("uses the destructive token for Failed", () => {
    render(<StatusBadge phase="Failed" />);
    expect(screen.getByTestId("status-badge").className).toContain("text-destructive");
  });

  it("renders Unknown when phase is missing", () => {
    render(<StatusBadge />);
    expect(screen.getByText("Unknown")).toBeInTheDocument();
  });
});
