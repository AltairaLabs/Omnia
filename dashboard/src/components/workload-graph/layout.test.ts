import { describe, it, expect } from "vitest";
import { layoutFlow } from "./layout";
import type { Node, Edge } from "@xyflow/react";

describe("layoutFlow", () => {
  it("assigns non-overlapping positions to all nodes", async () => {
    const nodes: Node[] = [
      { id: "a", type: "workloadState", position: { x: 0, y: 0 }, data: {} },
      { id: "b", type: "workloadState", position: { x: 0, y: 0 }, data: {} },
    ];
    const edges: Edge[] = [{ id: "e", source: "a", target: "b" }];
    const { nodes: out, routes } = await layoutFlow(nodes, edges);
    expect(out).toHaveLength(2);
    const [a, b] = out;
    expect(a.position).not.toEqual(b.position);
    // elk produces an orthogonal route for the edge
    expect(routes.has("e")).toBe(true);
  });

  it("returns nodes unchanged when there are none", async () => {
    const { nodes: out, routes } = await layoutFlow([], []);
    expect(out).toEqual([]);
    expect(routes.size).toBe(0);
  });
});
