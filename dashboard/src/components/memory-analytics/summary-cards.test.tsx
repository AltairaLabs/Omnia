import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { SummaryCards } from "./summary-cards";

describe("SummaryCards", () => {
  it("renders four cards with provided values", () => {
    render(
      <SummaryCards
        totalMemories={1234}
        activeUsers={56}
        memoriesToday={42}
        piiBlocked={3}
      />,
    );
    expect(screen.getByText("Total memories")).toBeInTheDocument();
    expect(screen.getByText("1,234")).toBeInTheDocument();
    expect(screen.getByText("Active users")).toBeInTheDocument();
    expect(screen.getByText("56")).toBeInTheDocument();
    expect(screen.getByText("Created today")).toBeInTheDocument();
    expect(screen.getByText("42")).toBeInTheDocument();
    expect(screen.getByText("PII blocked")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("forwards the loading flag to each StatCard", () => {
    const { container } = render(
      <SummaryCards
        totalMemories={0}
        activeUsers={0}
        memoriesToday={0}
        piiBlocked={0}
        loading
      />,
    );
    // Skeletons show as elements with role attributes from shadcn; assert
    // the four titles are NOT yet rendered while loading.
    expect(screen.queryByText("Total memories")).not.toBeInTheDocument();
    expect(container.querySelectorAll(".animate-pulse").length).toBeGreaterThan(0);
  });
});
