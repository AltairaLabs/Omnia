import type { ReactNode } from "react";
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("@xyflow/react", () => ({
  BaseEdge: ({ path }: { path: string }) => <div data-testid="base-edge" data-path={path} />,
  EdgeLabelRenderer: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  getSmoothStepPath: () => ["M 0,0 L 1,1", 5, 5],
}));

import { WorkloadEdge } from "./workload-edge";

function renderEdge(extra: Record<string, unknown>) {
  const props = {
    id: "e", source: "a", target: "b",
    sourceX: 0, sourceY: 0, targetX: 10, targetY: 0,
    ...extra,
  } as unknown as Parameters<typeof WorkloadEdge>[0];
  return render(<WorkloadEdge {...props} />);
}

describe("WorkloadEdge", () => {
  it("draws elk's routed path and renders the label when points are provided", () => {
    renderEdge({
      data: { points: [{ x: 0, y: 0 }, { x: 20, y: 0 }, { x: 20, y: 20 }] },
      label: "need_more",
    });
    expect(screen.getByTestId("base-edge").getAttribute("data-path")).toContain("Q 20,0");
    expect(screen.getByText("need_more")).toBeInTheDocument();
  });

  it("falls back to a smoothstep path when there is no route", () => {
    renderEdge({ data: {} });
    expect(screen.getByTestId("base-edge").getAttribute("data-path")).toBe("M 0,0 L 1,1");
  });
});
