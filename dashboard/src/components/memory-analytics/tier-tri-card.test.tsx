import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { TierTriCard } from "./tier-tri-card";

describe("TierTriCard", () => {
  it("renders a card per tier with count and share", () => {
    render(
      <TierTriCard
        rows={[
          { key: "institutional", value: 100, count: 100 },
          { key: "agent", value: 200, count: 200 },
          { key: "user", value: 700, count: 700 },
        ]}
      />,
    );
    expect(screen.getByText("Institutional")).toBeInTheDocument();
    expect(screen.getByText("Agent")).toBeInTheDocument();
    expect(screen.getByText("User")).toBeInTheDocument();
    expect(screen.getByText("100")).toBeInTheDocument();
    expect(screen.getByText("200")).toBeInTheDocument();
    expect(screen.getByText("700")).toBeInTheDocument();
    expect(screen.getByText("10.0%")).toBeInTheDocument();
    expect(screen.getByText("20.0%")).toBeInTheDocument();
    expect(screen.getByText("70.0%")).toBeInTheDocument();
  });

  it("renders zero state when total is 0", () => {
    render(<TierTriCard rows={[]} />);
    expect(screen.getAllByText("0")).toHaveLength(3);
    expect(screen.getAllByText("0.0%")).toHaveLength(3);
  });

  it("ignores rows whose key is not a recognised tier", () => {
    render(
      <TierTriCard
        rows={[
          { key: "institutional", value: 5, count: 5 },
          { key: "memory:context", value: 999, count: 999 },
        ]}
      />,
    );
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getAllByText("0")).toHaveLength(2); // agent + user fallthrough
  });

  it("renders skeletons when loading", () => {
    render(<TierTriCard rows={[]} loading />);
    expect(screen.getAllByTestId("tier-skeleton").length).toBeGreaterThan(0);
  });
});
