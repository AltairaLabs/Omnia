import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { GrowthChart } from "./growth-chart";

describe("GrowthChart", () => {
  it("renders title and the three range buttons", () => {
    render(
      <GrowthChart
        rows={[{ key: "2026-04-01", value: 5, count: 5 }]}
        rangeDays={7}
        onRangeChange={() => {}}
      />,
    );
    expect(screen.getByText(/Growth over time/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "7d" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "30d" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "90d" })).toBeInTheDocument();
  });

  it("calls onRangeChange when a different range is clicked", () => {
    const onRangeChange = vi.fn();
    render(
      <GrowthChart
        rows={[]}
        rangeDays={7}
        onRangeChange={onRangeChange}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "30d" }));
    expect(onRangeChange).toHaveBeenCalledWith(30);
    fireEvent.click(screen.getByRole("button", { name: "90d" }));
    expect(onRangeChange).toHaveBeenCalledWith(90);
  });

  it("renders an empty chart area when rows is empty", () => {
    const { container } = render(
      <GrowthChart rows={[]} rangeDays={7} onRangeChange={() => {}} />,
    );
    expect(container.querySelector(".recharts-responsive-container")).not.toBeNull();
  });
});
