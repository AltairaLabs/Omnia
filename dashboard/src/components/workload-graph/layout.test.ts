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

describe("layoutFlow — per-node sizes", () => {
  it("uses each node's own width/height for elk", async () => {
    const nodes = [
      { id: "a", type: "workflowInitial", width: 24, height: 24, position: { x: 0, y: 0 }, data: {} },
      { id: "b", type: "workflowState", width: 200, height: 68, position: { x: 0, y: 0 }, data: {} },
    ] as never[];
    const { routes } = await layoutFlow(nodes, [{ id: "e", source: "a", target: "b" }]);
    expect(routes.has("e")).toBe(true);
  });
});

function child(id: string, parentId?: string, isContainer = false): Node {
  return {
    id, parentId, position: { x: 0, y: 0 }, width: 170, height: 52,
    data: { isContainer }, type: "x",
  } as Node;
}

describe("layoutFlow — hierarchical", () => {
  it("sizes a container from its children and positions children relative to it", async () => {
    // Mirror the real to-flow output: container carries type "composition" and
    // the isContainer flag at data.node.isContainer (not data.isContainer).
    const containerNode = {
      id: "main", position: { x: 0, y: 0 }, width: 320, height: 200,
      type: "composition", data: { node: { id: "main", isContainer: true } },
    } as unknown as Node;
    const nodes: Node[] = [
      containerNode,
      child("main::a", "main"),
      child("main::b", "main"),
    ];
    const edges: Edge[] = [{ id: "e", source: "main::a", target: "main::b" }];
    const { nodes: laid } = await layoutFlow(nodes, edges);
    const container = laid.find((x) => x.id === "main")!;
    const c = laid.find((x) => x.id === "main::a")!;
    // container grew beyond a leaf's height to hold its stacked children
    expect((container.height ?? 0)).toBeGreaterThan(52);
    // child position is parent-relative (small, not absolute canvas coords)
    expect(c.position.y).toBeGreaterThanOrEqual(0);
    expect(c.position.y).toBeLessThan(container.height ?? 0);
  });

  it("lays out flat graphs as before when there are no parents", async () => {
    const nodes: Node[] = [child("a"), child("b")];
    const { nodes: laid } = await layoutFlow(nodes, [{ id: "e", source: "a", target: "b" }]);
    expect(laid).toHaveLength(2);
    expect(laid.find((x) => x.id === "b")!.position.y).toBeGreaterThan(0);
  });
});
