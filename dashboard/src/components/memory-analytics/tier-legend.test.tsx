import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { TierLegend } from "./tier-legend";

describe("TierLegend", () => {
  it("renders all three tier names", () => {
    render(<TierLegend />);
    expect(screen.getByText("Institutional")).toBeInTheDocument();
    expect(screen.getByText("Agent")).toBeInTheDocument();
    expect(screen.getByText("User")).toBeInTheDocument();
  });

  it("renders each tier description", () => {
    render(<TierLegend />);
    expect(
      screen.getByText(/Knowledge shared across every agent/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Patterns this agent has learned/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/about a specific user/),
    ).toBeInTheDocument();
  });

  it("includes a header title", () => {
    render(<TierLegend />);
    expect(screen.getByText(/How memory is organized/)).toBeInTheDocument();
  });
});
