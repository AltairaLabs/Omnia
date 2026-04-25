import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { CategoryDonut } from "./category-donut";

describe("CategoryDonut", () => {
  it("renders chart title with rows present", () => {
    render(
      <CategoryDonut
        rows={[
          { key: "memory:context", value: 100, count: 100 },
          { key: "memory:identity", value: 50, count: 50 },
        ]}
      />,
    );
    expect(screen.getByText(/Memory by category/i)).toBeInTheDocument();
  });

  it("renders empty state when rows is empty", () => {
    render(<CategoryDonut rows={[]} />);
    expect(
      screen.getByText(/No memory data yet for this workspace/i),
    ).toBeInTheDocument();
  });

  it("falls back to the unknown color for unrecognised categories", () => {
    const { container } = render(
      <CategoryDonut
        rows={[{ key: "memory:novel-category", value: 5, count: 5 }]}
      />,
    );
    expect(container).toBeTruthy();
    expect(screen.getByText(/Memory by category/i)).toBeInTheDocument();
  });
});
