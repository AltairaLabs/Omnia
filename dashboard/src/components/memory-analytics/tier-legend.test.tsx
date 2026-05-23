import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { TierLegend } from "./tier-legend";

describe("TierLegend", () => {
  it("renders all four tier names", () => {
    render(<TierLegend />);
    expect(screen.getByText("Institutional")).toBeInTheDocument();
    expect(screen.getByText("Agent")).toBeInTheDocument();
    expect(screen.getByText("User")).toBeInTheDocument();
    expect(screen.getByText("User-for-agent")).toBeInTheDocument();
  });

  it("renders each tier description", () => {
    render(<TierLegend />);
    expect(
      screen.getByText(/Knowledge shared across every agent/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Patterns this agent has learned/),
    ).toBeInTheDocument();
    // Both the user and user_for_agent tier descriptions mention
    // "specific user" — use getAllByText to accept either match.
    expect(
      screen.getAllByText(/about a specific user/).length,
    ).toBeGreaterThanOrEqual(2);
  });

  it("includes a header title", () => {
    render(<TierLegend />);
    expect(screen.getByText(/How memory is organized/)).toBeInTheDocument();
  });
});
