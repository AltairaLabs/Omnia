import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { AgentChart } from "./agent-chart";

describe("AgentChart", () => {
  it("renders title with rows present", () => {
    render(
      <AgentChart
        rows={[{ key: "support-agent", value: 100, count: 100 }]}
      />,
    );
    expect(screen.getByText(/Memory by agent/i)).toBeInTheDocument();
  });

  it("shows empty state when no rows", () => {
    render(<AgentChart rows={[]} />);
    expect(
      screen.getByText(/No agent data yet for this workspace/i),
    ).toBeInTheDocument();
  });

  it("limits the chart to the top 20 agents by value", () => {
    const many: { key: string; value: number; count: number }[] = [];
    for (let i = 0; i < 30; i++) {
      many.push({ key: `agent-${i}`, value: 100 - i, count: 100 - i });
    }
    const { container } = render(<AgentChart rows={many} />);
    expect(container).toBeTruthy();
    expect(screen.getByText(/Memory by agent/i)).toBeInTheDocument();
  });
});
