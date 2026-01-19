/**
 * Tests for SharedBadge component.
 */

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { SharedBadge } from "./shared-badge";

describe("SharedBadge", () => {
  it("renders the shared badge", () => {
    render(<SharedBadge />);

    expect(screen.getByTestId("shared-badge")).toBeInTheDocument();
    expect(screen.getByText("Shared")).toBeInTheDocument();
  });

  it("applies custom className", () => {
    render(<SharedBadge className="custom-class" />);

    const badge = screen.getByTestId("shared-badge");
    expect(badge).toHaveClass("custom-class");
  });

  it("has correct styling", () => {
    render(<SharedBadge />);

    const badge = screen.getByTestId("shared-badge");
    expect(badge).toHaveClass("bg-blue-500/15");
    expect(badge).toHaveClass("text-blue-700");
  });
});
