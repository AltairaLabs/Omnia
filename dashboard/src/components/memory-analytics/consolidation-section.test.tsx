import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ConsolidationSection } from "./consolidation-section";

describe("ConsolidationSection", () => {
  it("renders headline stats and by-type breakdown", () => {
    render(
      <ConsolidationSection
        stats={{
          passesTotal: 7,
          actionsTotal: 47,
          actionsByType: {
            create_summary: 30,
            supersede: 12,
            rescope: 5,
          },
        }}
      />,
    );
    expect(screen.getByText("7")).toBeInTheDocument();
    expect(screen.getByText("47")).toBeInTheDocument();
    expect(screen.getByText("30")).toBeInTheDocument();
    expect(screen.getByText("Create summary")).toBeInTheDocument();
    expect(screen.getByText("Supersede")).toBeInTheDocument();
    expect(screen.getByText("Rescope")).toBeInTheDocument();
  });

  it("renders empty state when no passes recorded", () => {
    render(
      <ConsolidationSection
        stats={{ passesTotal: 0, actionsTotal: 0, actionsByType: {} }}
      />,
    );
    expect(
      screen.getByText(/No consolidation runs/i),
    ).toBeInTheDocument();
  });

  it("renders empty state when stats undefined", () => {
    render(<ConsolidationSection stats={undefined} />);
    expect(
      screen.getByText(/No consolidation runs/i),
    ).toBeInTheDocument();
  });

  it("renders skeleton when loading", () => {
    render(<ConsolidationSection stats={undefined} loading />);
    expect(screen.getByTestId("consolidation-skeleton")).toBeInTheDocument();
  });

  it("falls back to action key when no friendly label exists", () => {
    render(
      <ConsolidationSection
        stats={{
          passesTotal: 1,
          actionsTotal: 1,
          actionsByType: { unknown_action: 1 },
        }}
      />,
    );
    expect(screen.getByText("unknown_action")).toBeInTheDocument();
  });
});
