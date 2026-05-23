import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { TierQuadCard } from "./tier-quad-card";

describe("TierQuadCard", () => {
  it("renders a card per tier with count and share", () => {
    render(
      <TierQuadCard
        rows={[
          { key: "institutional", value: 100, count: 100 },
          { key: "agent", value: 200, count: 200 },
          { key: "user", value: 300, count: 300 },
          { key: "user_for_agent", value: 400, count: 400 },
        ]}
      />,
    );
    expect(screen.getByText("Institutional")).toBeInTheDocument();
    expect(screen.getByText("Agent")).toBeInTheDocument();
    expect(screen.getByText("User")).toBeInTheDocument();
    expect(screen.getByText("User-for-agent")).toBeInTheDocument();
    expect(screen.getByText("100")).toBeInTheDocument();
    expect(screen.getByText("200")).toBeInTheDocument();
    expect(screen.getByText("300")).toBeInTheDocument();
    expect(screen.getByText("400")).toBeInTheDocument();
    expect(screen.getByText("10.0%")).toBeInTheDocument();
    expect(screen.getByText("20.0%")).toBeInTheDocument();
    expect(screen.getByText("30.0%")).toBeInTheDocument();
    expect(screen.getByText("40.0%")).toBeInTheDocument();
  });

  it("renders zero state when total is 0", () => {
    render(<TierQuadCard rows={[]} />);
    expect(screen.getAllByText("0")).toHaveLength(4);
    expect(screen.getAllByText("0.0%")).toHaveLength(4);
  });

  it("ignores rows whose key is not a recognised tier", () => {
    render(
      <TierQuadCard
        rows={[
          { key: "institutional", value: 5, count: 5 },
          { key: "memory:context", value: 999, count: 999 },
        ]}
      />,
    );
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getAllByText("0")).toHaveLength(3); // agent + user + user_for_agent
  });

  it("renders skeletons when loading", () => {
    render(<TierQuadCard rows={[]} loading />);
    expect(screen.getAllByTestId("tier-skeleton").length).toBeGreaterThan(0);
  });
});
