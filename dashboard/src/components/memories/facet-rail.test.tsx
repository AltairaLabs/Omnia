import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FacetRail, type Facet } from "./facet-rail";

const facets: Facet[] = [
  { key: "institutional", label: "Institutional", color: "#111", count: 3 },
  { key: "agent", label: "Agent", color: "#222", count: 0 },
  { key: "user", label: "User", color: "#333", count: 5 },
];

describe("FacetRail", () => {
  it("renders every facet with its count, including zero", () => {
    render(<FacetRail facets={facets} hidden={new Set()} onToggle={() => {}} />);
    expect(screen.getByText("Institutional")).toBeInTheDocument();
    expect(screen.getByText("Agent")).toBeInTheDocument();
    expect(screen.getByText("User")).toBeInTheDocument();
    expect(screen.getByTestId("facet-count-agent")).toHaveTextContent("0");
  });

  it("calls onToggle with the facet key when a chip is clicked", async () => {
    const onToggle = vi.fn();
    render(<FacetRail facets={facets} hidden={new Set()} onToggle={onToggle} />);
    await userEvent.click(screen.getByTestId("facet-chip-user"));
    expect(onToggle).toHaveBeenCalledWith("user");
  });

  it("marks hidden facets with aria-pressed=false", () => {
    render(<FacetRail facets={facets} hidden={new Set(["agent"])} onToggle={() => {}} />);
    expect(screen.getByTestId("facet-chip-agent")).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByTestId("facet-chip-user")).toHaveAttribute("aria-pressed", "true");
  });
});
